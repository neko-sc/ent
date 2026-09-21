// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// trackingLevel is a cache level owned by the tests. It reports what the driver
// stored in it, so backfill behavior is observable without touching the driver.
type trackingLevel struct {
	name    string
	mutex   sync.Mutex
	entries map[Key]trackedEntry
}

type trackedEntry struct {
	entry *Entry
	ttl   time.Duration
}

func newTrackingLevel(name string) *trackingLevel {
	return &trackingLevel{name: name, entries: make(map[Key]trackedEntry)}
}

func (l *trackingLevel) Name() string {
	return l.name
}

func (l *trackingLevel) Get(ctx context.Context, key Key) (*Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.entries[key].entry, nil
}

func (l *trackingLevel) Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if entry == nil || ttl <= 0 {
		delete(l.entries, key)
		return nil
	}
	l.entries[key] = trackedEntry{entry: entry, ttl: ttl}
	return nil
}

func (l *trackingLevel) Delete(ctx context.Context, keys ...Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
	return nil
}

func (l *trackingLevel) clear() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	clear(l.entries)
}

func (l *trackingLevel) stored() []trackedEntry {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	stored := make([]trackedEntry, 0, len(l.entries))
	for _, tracked := range l.entries {
		stored = append(stored, tracked)
	}
	return stored
}

func newMemoryLevel(t *testing.T) Level {
	t.Helper()
	level := NewMemory(100, 0)
	t.Cleanup(func() { level.(*memoryLevel).cache.StopAllGoroutines() })
	return level
}

func newTestDriver(t *testing.T, options ...Option) (*Driver, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	cached, err := New(entsql.OpenDB(dialect.SQLite, database), append([]Option{Levels(newMemoryLevel(t))}, options...)...)
	require.NoError(t, err)
	require.Same(t, database, cached.DB())
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, cached.Close())
	})
	return cached, mock
}

func readContext(ctx context.Context, ids ...any) context.Context {
	return dialect.WithStatement(ctx, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, IDColumn: "id", PointRead: len(ids) > 0, RequestedIDs: ids})
}

func writeContext(ctx context.Context, ids ...any) context.Context {
	statement := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}}
	for _, id := range ids {
		statement.Rows = append(statement.Rows, dialect.RowRef{Table: "users", ID: id})
	}
	if len(ids) == 0 {
		statement.Bulk = []string{"users"}
	}
	return dialect.WithStatement(ctx, statement)
}

func expectRead(mock sqlmock.Sqlmock, query string, ids ...int64) {
	rows := sqlmock.NewRows([]string{"id"})
	for _, id := range ids {
		rows.AddRow(id)
	}
	mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()
}

func readIDs(t *testing.T, executor dialect.ExecQuerier, ctx context.Context, query string, arguments ...any) []int64 {
	t.Helper()
	rows, err := executor.Query(ctx, query, arguments)
	require.NoError(t, err)
	var ids []int64
	for rows.Next() {
		var id int64
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	return ids
}

func executeWrite(t *testing.T, executor dialect.ExecQuerier, mock sqlmock.Sqlmock, ctx context.Context) {
	t.Helper()
	mock.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 1))
	_, err := executor.Exec(ctx, "UPDATE users", nil)
	require.NoError(t, err)
}

func TestDriverReadReplay(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id FROM users", 1, 2)
	require.Equal(t, []int64{1, 2}, readIDs(t, cached, ctx, "SELECT id FROM users"))
	require.False(t, info.Hit)
	require.Equal(t, []string{"users"}, info.Tables)
	require.Equal(t, []int64{1, 2}, readIDs(t, cached, ctx, "SELECT id FROM users"))
	require.True(t, info.Hit)
	require.Equal(t, "memory", info.Level)
	snapshot := cached.Stats().Snapshot()
	require.Equal(t, uint64(1), snapshot.Fills)
	require.Equal(t, uint64(1), snapshot.Misses)
	require.Equal(t, uint64(1), snapshot.Hits["memory"])
}

func TestDriverScannerReplay(t *testing.T) {
	for _, level := range []struct {
		name string
		new  func(*testing.T) Level
	}{
		{name: "memory", new: newMemoryLevel},
		{name: "encoding", new: func(*testing.T) Level { return newEncodingLevel() }},
	} {
		t.Run(level.name, func(t *testing.T) {
			t.Run("JSON projection", func(t *testing.T) {
				cached, mock := newTestDriver(t, Levels(level.new(t)), OnError(func(_ context.Context, err error) {
					t.Errorf("cache error: %v", err)
				}))

				mock.ExpectQuery("SELECT config FROM users").WillReturnRows(sqlmock.NewRows([]string{"config"}).
					AddRow([]byte(`{"enabled":true}`)).AddRow(nil).AddRow([]byte(`{"enabled":false}`))).RowsWillBeClosed()
				type projection struct {
					Config struct {
						Enabled bool `json:"enabled"`
					} `sql:"config"`
				}
				for attempt := range 2 {
					ctx, info := WithInfo(Cache(readContext(t.Context())))
					rows, err := cached.Query(ctx, "SELECT config FROM users", nil)

					require.NoError(t, err)
					var result []projection

					require.NoError(t, entsql.ScanSlice(rows, &result))
					require.NoError(t, rows.Close())
					require.Len(t, result, 3)
					require.True(t, result[0].Config.Enabled)
					require.False(t, result[1].Config.Enabled)
					require.False(t, result[2].Config.Enabled)
					require.Equal(t, attempt > 0, info.Hit)
					if info.Hit {
						require.Equal(t, level.name, info.Level)
					}
				}
			})

			t.Run("nullable UUID and plain NULL", func(t *testing.T) {
				cached, mock := newTestDriver(t, Levels(level.new(t)), OnError(func(_ context.Context, err error) {
					t.Errorf("cache error: %v", err)
				}))

				identifier := uuid.MustParse("01234567-89ab-cdef-0123-456789abcdef")
				mock.ExpectQuery("SELECT uploader_user_id, empty FROM users").WillReturnRows(sqlmock.NewRows([]string{"uploader_user_id", "empty"}).
					AddRow(identifier.String(), nil).AddRow(nil, nil)).RowsWillBeClosed()
				for attempt := range 2 {
					ctx, info := WithInfo(Cache(readContext(t.Context())))
					rows, err := cached.Query(ctx, "SELECT uploader_user_id, empty FROM users", nil)

					require.NoError(t, err)
					var result []*uuid.UUID

					for rows.Next() {
						value := new(uuid.UUID)
						nullable := &entsql.NullScanner{S: value}
						var empty any = "stale"

						require.NoError(t, rows.Scan(nullable, &empty))
						require.Nil(t, empty)
						if nullable.Valid {
							result = append(result, value)
						} else {
							result = append(result, nil)
						}
					}
					require.NoError(t, rows.Err())
					require.NoError(t, rows.Close())
					require.Equal(t, []*uuid.UUID{&identifier, nil}, result)
					require.Equal(t, attempt > 0, info.Hit)
					if info.Hit {
						require.Equal(t, level.name, info.Level)
					}
				}
			})
		})
	}
}

func TestDriverConcurrentReadWrite(t *testing.T) {
	const (
		workerCount = 20
		iterations  = 20
		pointQuery  = "SELECT id FROM users WHERE id = ?"
		listQuery   = "SELECT id FROM users WHERE id >= ?"
	)
	database, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)
	database.SetMaxOpenConns(workerCount)
	database.SetMaxIdleConns(workerCount)
	cached, err := New(entsql.OpenDB(dialect.SQLite, database),
		Levels(ContextLevel(), newMemoryLevel(t)),
		Generations(NewMemoryGenerations()), Global(true), TTL(time.Hour),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		for range database.Stats().OpenConnections {
			mock.ExpectClose()
		}
		require.NoError(t, cached.Close())
	})
	// Cache hits and singleflight make the database read count nondeterministic.
	// Provision the maximum; unused read expectations are intentionally allowed.
	for range workerCount * iterations {
		mock.ExpectQuery(listQuery).WithArgs(sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)).AddRow(int64(2)))
		mock.ExpectExec("UPDATE users SET name = ? WHERE id = ?").WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for id := int64(1); id <= 2; id++ {
		for range workerCount * iterations {
			mock.ExpectQuery(pointQuery).WithArgs(id).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
		}
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := range workerCount {
		workers.Go(func() {
			ctx := NewContext(t.Context())
			<-start
			for iteration := range iterations {
				id := int64((worker+iteration)%2 + 1)
				if _, err := cached.Exec(writeContext(ctx, id), "UPDATE users SET name = ? WHERE id = ?", []any{"active", id}); !assert.NoError(t, err) {
					return
				}
				for read := range 3 {
					query := pointQuery
					arguments := []any{id}
					want := []int64{id}
					readCtx := readContext(ctx, id)
					switch read {
					case 1:
						query = listQuery
						arguments = []any{int64(1)}
						want = []int64{1, 2}
						readCtx = Cache(readContext(ctx))
					case 2:
						readCtx = Skip(readCtx)
					}
					readCtx, info := WithInfo(readCtx)
					rows, err := cached.Query(readCtx, query, arguments)
					if !assert.NoError(t, err) {
						return
					}
					var ids []int64
					for rows.Next() {
						var returnedID int64
						if !assert.NoError(t, rows.Scan(&returnedID)) {
							assert.NoError(t, rows.Close())
							return
						}
						ids = append(ids, returnedID)
					}
					assert.NoError(t, rows.Err())
					assert.NoError(t, rows.Close())
					assert.Equal(t, want, ids)
					assert.Equal(t, []string{"users"}, info.Tables)
					assert.Equal(t, read != 1, info.PointRead)
					if read == 2 {
						assert.Equal(t, BypassSkip, info.Bypass)
					} else {
						assert.Empty(t, info.Bypass)
					}
				}
			}
		})
	}
	close(start)
	workers.Wait()

	snapshot := cached.Stats().Snapshot()
	total := snapshot.Misses
	for _, hits := range snapshot.Hits {
		total += hits
	}
	for _, bypasses := range snapshot.Bypasses {
		total += bypasses
	}
	// Shared singleflight reads record both a miss and a hit.
	require.Equal(t, uint64(workerCount*iterations*3)+snapshot.SingleflightShared, total)
	require.Equal(t, snapshot.SingleflightShared, snapshot.Hits["singleflight"])
	require.Equal(t, uint64(workerCount*iterations), snapshot.Bypasses[BypassSkip])
	require.Equal(t, uint64(workerCount*iterations), snapshot.Bumps["users"])
}

func TestDriverSessionVariables(t *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	cached, err := New(entsql.OpenDB(dialect.Postgres, database), Levels(newMemoryLevel(t)))
	require.NoError(t, err)
	require.Same(t, database, cached.DB())
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, cached.Close())
	})
	for index, tenant := range []string{"first", "second"} {
		ctx, info := WithInfo(Cache(entsql.WithVar(readContext(t.Context()), "app.tenant", tenant)))
		mock.ExpectExec("SET app.tenant = '" + tenant + "'").WillReturnResult(sqlmock.NewResult(0, 0))
		expectRead(mock, "SELECT id", int64(index+1))
		mock.ExpectExec("RESET app.tenant").WillReturnResult(sqlmock.NewResult(0, 0))
		require.Equal(t, []int64{int64(index + 1)}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
		require.Equal(t, []int64{int64(index + 1)}, readIDs(t, cached, ctx, "SELECT id"))
		require.True(t, info.Hit)
	}
	require.Equal(t, []int64{1}, readIDs(t, cached, Cache(entsql.WithVar(readContext(t.Context()), "app.tenant", "first")), "SELECT id"))
}

func TestDriverRawCTEs(t *testing.T) {
	for _, test := range []struct {
		query string
		write bool
	}{
		{query: "WITH selected AS (SELECT id FROM users) SELECT id FROM selected"},
		{query: "WITH updated AS (SELECT update_count, deleted_at FROM users) SELECT id FROM updated"},
		{query: "WITH changed AS (iNsErT INTO users DEFAULT VALUES RETURNING id) SELECT id FROM changed", write: true},
		{query: "WITH changed AS (uPdAtE users SET name = 'new' RETURNING id) SELECT id FROM changed", write: true},
		{query: "WITH changed AS (dElEtE FROM users RETURNING id) SELECT id FROM changed", write: true},
		{query: "WITH source AS (SELECT 1) mErGe INTO users USING source RETURNING id", write: true},
	} {
		for _, touches := range []bool{false, true} {
			t.Run(test.query+map[bool]string{false: "/global", true: "/touches"}[touches], func(t *testing.T) {
				cached, mock := newTestDriver(t)
				ctx, info := WithInfo(Cache(readContext(t.Context(), 1)))
				expectRead(mock, "SELECT id", 1)
				readIDs(t, cached, ctx, "SELECT id")
				raw := Cache(t.Context())
				if touches {
					raw = Touches(raw, "users")
				}
				expectRead(mock, regexp.QuoteMeta(test.query), 1)
				readIDs(t, cached, raw, test.query)
				if test.write {
					expectRead(mock, "SELECT id", 2)
				}
				readIDs(t, cached, ctx, "SELECT id")
				require.Equal(t, !test.write, info.Hit)
				snapshot := cached.Stats().Snapshot()
				require.Equal(t, uint64(0), snapshot.Bumps["groups"])
				require.Equal(t, test.write && touches, snapshot.Bumps["users"] > 0)
			})
		}
	}
}

func TestDriverPolicies(t *testing.T) {
	for _, test := range []struct {
		name    string
		options []Option
		context func(context.Context) context.Context
		bypass  string
	}{
		{name: "explicit disabled", bypass: BypassDisabled},
		{name: "explicit enabled", context: func(ctx context.Context) context.Context { return Cache(ctx) }},
		{name: "global", options: []Option{Global(true)}},
		{name: "global skip", options: []Option{Global(true)}, context: Skip, bypass: BypassSkip},
		{name: "explicit overrides skip", options: []Option{Global(true)}, context: func(ctx context.Context) context.Context { return Cache(Skip(ctx)) }},
		{name: "allow list", options: []Option{Global(true), CacheTables("groups")}, bypass: BypassDisabled},
		{name: "deny list", options: []Option{Global(true), SkipTables("users")}, bypass: BypassSkip},
		{name: "allow node", options: []Option{Global(true), CacheTables("users")}},
		{name: "lock", options: []Option{Global(true)}, context: func(ctx context.Context) context.Context {
			dialect.StatementFrom(ctx).Lock = true
			return ctx
		}, bypass: BypassLock},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t, test.options...)
			ctx := readContext(t.Context(), 1)
			if test.context != nil {
				ctx = test.context(ctx)
			}
			ctx, info := WithInfo(ctx)
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			if test.bypass != "" {
				expectRead(mock, "SELECT id", 1)
			}
			readIDs(t, cached, ctx, "SELECT id")
			require.Equal(t, test.bypass, info.Bypass)
			require.Equal(t, test.bypass == "", info.Hit)
			require.True(t, info.PointRead)
		})
	}
}

func TestDriverRawQueries(t *testing.T) {
	t.Run("reads without a statement bypass", func(t *testing.T) {
		cached, mock := newTestDriver(t)
		for _, query := range []string{" SELECT id", " select id", "WITH selected AS SELECT id"} {
			expectRead(mock, query, 1)
			ctx, info := WithInfo(Cache(t.Context()))
			readIDs(t, cached, ctx, query)
			require.Equal(t, BypassNoStatement, info.Bypass)
		}
		// Caching was never requested here, so the read is not counted as a bypass.
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, t.Context(), "SELECT id")
		require.Equal(t, uint64(3), cached.Stats().Snapshot().Bypasses[BypassNoStatement])
	})

	t.Run("returning writes without a statement invalidate", func(t *testing.T) {
		cached, mock := newTestDriver(t)
		ctx := Cache(readContext(t.Context()))
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
		readIDs(t, cached, ctx, "SELECT id")
		expectRead(mock, "INSERT INTO users RETURNING id", 2)
		readIDs(t, cached, t.Context(), "INSERT INTO users RETURNING id")
		expectRead(mock, "SELECT id", 1, 2)
		require.Equal(t, []int64{1, 2}, readIDs(t, cached, ctx, "SELECT id"))
	})

	t.Run("execs without a statement invalidate", func(t *testing.T) {
		cached, mock := newTestDriver(t)
		ctx := Cache(readContext(t.Context()))
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
		executeWrite(t, cached, mock, t.Context())
		expectRead(mock, "SELECT id", 3)
		require.Equal(t, []int64{3}, readIDs(t, cached, ctx, "SELECT id"))
	})
}

func TestDriverWriteInvalidation(t *testing.T) {
	cached, mock := newTestDriver(t)
	first := Cache(readContext(t.Context(), 1))
	second := Cache(readContext(t.Context(), 2))
	list := Cache(readContext(t.Context()))
	for _, ctx := range []context.Context{first, second, list} {
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
	}
	executeWrite(t, cached, mock, writeContext(t.Context(), 1))
	readIDs(t, cached, second, "SELECT id")
	for _, ctx := range []context.Context{first, list} {
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
	}
	executeWrite(t, cached, mock, writeContext(t.Context()))
	for _, ctx := range []context.Context{first, second, list} {
		expectRead(mock, "SELECT id", 3)
		require.Equal(t, []int64{3}, readIDs(t, cached, ctx, "SELECT id"))
	}
	require.Equal(t, uint64(2), cached.Stats().Snapshot().Bumps["users"])
}

func TestDriverViews(t *testing.T) {
	for _, test := range []struct {
		name       string
		views      map[string][]string
		tables     []string
		writeTable string
		hit        bool
	}{
		{
			name:       "base table write invalidates view",
			views:      map[string][]string{"user_view": {"users"}},
			tables:     []string{"user_view", "users"},
			writeTable: "users",
		},
		{
			name:       "unrelated write preserves view cache",
			views:      map[string][]string{"user_view": {"users"}},
			tables:     []string{"user_view", "users"},
			writeTable: "groups",
			hit:        true,
		},
		{
			name:       "base table write invalidates transitive view",
			views:      map[string][]string{"view_a": {"view_b"}, "view_b": {"users"}},
			tables:     []string{"view_a", "view_b", "users"},
			writeTable: "users",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t, Views(test.views))
			ctx, info := WithInfo(Cache(dialect.WithStatement(t.Context(), &dialect.Statement{
				Kind:     dialect.StatementRead,
				Tables:   []string{test.tables[0]},
				IDColumn: "id",
			})))
			query := "SELECT id FROM " + test.tables[0]
			expectRead(mock, query, 1)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, query))
			require.Equal(t, Info{Tables: test.tables}, *info)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, query))
			require.Equal(t, Info{Hit: true, Level: "memory", Tables: test.tables}, *info)
			snapshot := cached.Stats().Snapshot()
			require.Equal(t, uint64(1), snapshot.Misses)
			require.Equal(t, uint64(1), snapshot.Fills)
			require.Equal(t, uint64(1), snapshot.Hits["memory"])

			write := "UPDATE " + test.writeTable + " SET name = 'updated'"
			mock.ExpectExec(write).WillReturnResult(sqlmock.NewResult(0, 1))
			_, err := cached.Exec(dialect.WithStatement(t.Context(), &dialect.Statement{
				Kind:   dialect.StatementWrite,
				Tables: []string{test.writeTable},
				Bulk:   []string{test.writeTable},
			}), write, nil)
			require.NoError(t, err)

			expectedID := int64(1)
			expectedMisses, expectedHits := uint64(1), uint64(2)
			expectedInfo := Info{Hit: true, Level: "memory", Tables: test.tables}
			if !test.hit {
				expectRead(mock, query, 2)
				expectedID = 2
				expectedMisses, expectedHits = 2, 1
				expectedInfo = Info{Tables: test.tables}
			}
			require.Equal(t, []int64{expectedID}, readIDs(t, cached, ctx, query))
			require.Equal(t, expectedInfo, *info)
			snapshot = cached.Stats().Snapshot()
			require.Equal(t, expectedMisses, snapshot.Misses)
			require.Equal(t, expectedMisses, snapshot.Fills)
			require.Equal(t, expectedHits, snapshot.Hits["memory"])
			require.Equal(t, map[string]uint64{test.writeTable: 1}, snapshot.Bumps)
			require.Empty(t, snapshot.Bypasses)
		})
	}
}

func TestDriverReturningInvalidation(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx := Cache(readContext(t.Context(), 1))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	statement := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}}
	expectRead(mock, "UPDATE users RETURNING id", 1)
	rows, err := cached.Query(dialect.WithStatement(t.Context(), statement), "UPDATE users RETURNING id", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	statement.Rows = []dialect.RowRef{{Table: "users", ID: id}}
	readIDs(t, cached, ctx, "SELECT id")
	require.NoError(t, rows.Close())
	require.NoError(t, rows.Close())
	expectRead(mock, "SELECT id", 2)
	readIDs(t, cached, ctx, "SELECT id")
	require.Equal(t, uint64(1), cached.Stats().Snapshot().Bumps["users"])
}

func TestDriverTTLOnly(t *testing.T) {
	for _, test := range []struct {
		name         string
		options      []Option
		queryOptions []QueryOption
	}{
		{name: "table policy", options: []Option{TTLOnlyTables("users")}},
		{name: "query option", queryOptions: []QueryOption{TTLOnly()}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t, test.options...)
			ctx := Cache(readContext(t.Context(), 1), test.queryOptions...)
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			executeWrite(t, cached, mock, writeContext(t.Context()))
			readIDs(t, cached, ctx, "SELECT id")
			unique := dialect.WithStatement(ctx, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, PointRead: true, IDColumn: "id"})
			expectRead(mock, "SELECT unique", 1)
			readIDs(t, cached, unique, "SELECT unique")
			executeWrite(t, cached, mock, writeContext(t.Context(), 1))
			readIDs(t, cached, unique, "SELECT unique")
		})
	}
}

func TestDriverContextBackfill(t *testing.T) {
	shared := newTrackingLevel("shared")
	cached, mock := newTestDriver(t, Levels(ContextLevel(), shared))
	ctx, info := WithInfo(Cache(NewContext(readContext(t.Context()))))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	readIDs(t, cached, ctx, "SELECT id")
	require.Equal(t, "context", info.Level)
	ctx, info = WithInfo(NewContext(ctx))
	readIDs(t, cached, ctx, "SELECT id")
	require.Equal(t, "shared", info.Level)
	shared.clear()
	readIDs(t, cached, ctx, "SELECT id")
	require.Equal(t, "context", info.Level)
}

func TestBackfillUsesOriginalExpiry(t *testing.T) {
	for _, noFill := range []bool{false, true} {
		t.Run(map[bool]string{false: "backfill", true: "no-fill"}[noFill], func(t *testing.T) {
			store := newClockGenerations()
			shared := newTrackingLevel("shared")
			cached, mock := newTestDriver(t, Generations(store), TTL(2*time.Second), Levels(ContextLevel(), shared))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			store.advance()
			ctx = NewContext(ctx)
			served := ctx
			if noFill {
				served = Cache(ctx, NoFill())
			}
			require.Equal(t, []int64{1}, readIDs(t, cached, served, "SELECT id"))
			require.Equal(t, "shared", info.Level)
			// A backfill copies the entry into the context level, so the next read
			// in the same context is served locally instead of from the shared level.
			require.Equal(t, []int64{1}, readIDs(t, cached, Cache(ctx, NoFill()), "SELECT id"))
			require.Equal(t, map[bool]string{false: "context", true: "shared"}[noFill], info.Level)
			// The backfilled copy inherits the remaining lifetime, not a fresh TTL.
			store.advance()
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, cached, NewContext(Cache(ctx, NoFill())), "SELECT id"))
			require.False(t, info.Hit)
		})
	}
}

func TestBackfillAcrossLevelsUsesOriginalExpiry(t *testing.T) {
	store := newClockGenerations()
	shared := newTrackingLevel("shared")
	cached, mock := newTestDriver(t, Generations(store), TTL(2*time.Second), Levels(shared, ContextLevel()))
	ctx, info := WithInfo(NewContext(Cache(readContext(t.Context()))))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	shared.clear()
	store.advance()
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.Equal(t, "context", info.Level)
	stored := shared.stored()
	require.Len(t, stored, 1)
	require.Equal(t, time.Second, stored[0].ttl)
}

func TestDriverFillSurvivesCancellation(t *testing.T) {
	shared := newTrackingLevel("shared")
	cached, mock := newTestDriver(t, Levels(ContextLevel(), shared))
	ctx, info := WithInfo(Cache(NewContext(readContext(t.Context()))))
	cancellable, cancel := context.WithCancel(ctx)
	defer cancel()
	expectRead(mock, "SELECT id", 1)
	rows, err := cached.Query(cancellable, "SELECT id", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	cancel()
	require.NoError(t, rows.Close())
	require.Equal(t, uint64(1), cached.Stats().Fills.Load())
	require.Zero(t, cached.Stats().DroppedFills.Load())
	require.Len(t, shared.stored(), 1)
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.True(t, info.Hit)
	require.Equal(t, "context", info.Level)
}

type cancelingLevel struct {
	Level
	cancel context.CancelFunc
}

func (l *cancelingLevel) Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error {
	l.cancel()
	return l.Level.Set(ctx, key, entry, ttl)
}

func TestDriverBackfillSurvivesCancellation(t *testing.T) {
	front, back := newTrackingLevel("front"), newTrackingLevel("back")
	cached, mock := newTestDriver(t, Levels(front, back))
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	front.clear()
	cancellable, cancel := context.WithCancel(ctx)
	defer cancel()
	cached.levels[0] = &cancelingLevel{Level: front, cancel: cancel}
	require.Equal(t, []int64{1}, readIDs(t, cached, cancellable, "SELECT id"))
	require.ErrorIs(t, cancellable.Err(), context.Canceled)
	require.True(t, info.Hit)
	require.Equal(t, "back", info.Level)
	require.Len(t, front.stored(), 1)
}

func TestDriverFillLifetimeStartsBeforeRead(t *testing.T) {
	store := newClockGenerations()
	cached, mock := newTestDriver(t, Generations(store), TTL(time.Second))
	ctx := Cache(readContext(t.Context()))
	expectRead(mock, "SELECT id", 1)
	rows, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	store.advance()
	require.NoError(t, rows.Close())
	snapshot := cached.Stats().Snapshot()
	require.Zero(t, snapshot.Fills)
	require.Equal(t, uint64(1), snapshot.DroppedFills)
	expectRead(mock, "SELECT id", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
}

func TestDriverNoFill(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx := Cache(readContext(t.Context()), NoFill())
	for range 2 {
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
	}
	require.Zero(t, cached.Stats().Snapshot().Fills)
}

func TestDriverIncompleteReadIsNotCached(t *testing.T) {
	// A long wait turns a leaked in-flight registration into a blocked follower.
	cached, mock := newTestDriver(t, SingleflightWait(time.Minute))
	ctx := Cache(readContext(t.Context()))
	expectRead(mock, "SELECT id", 1)
	rows, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	require.NoError(t, rows.Close())
	require.Equal(t, uint64(1), cached.Stats().Snapshot().DroppedFills)
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	expectRead(mock, "SELECT id", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, bounded, "SELECT id"))
}

func TestDriverFailedReadIsNotCached(t *testing.T) {
	cached, mock := newTestDriver(t, SingleflightWait(time.Minute))
	ctx := Cache(readContext(t.Context()))
	mock.ExpectQuery("SELECT id").WillReturnError(errors.New("database failed"))
	_, err := cached.Query(ctx, "SELECT id", nil)
	require.ErrorContains(t, err, "database failed")
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	expectRead(mock, "SELECT id", 1)
	require.Equal(t, []int64{1}, readIDs(t, cached, bounded, "SELECT id"))
}

func TestQueryTTLOverridesDriverTTL(t *testing.T) {
	cached, mock := newTestDriver(t, TTL(time.Hour))
	ctx, info := WithInfo(Cache(readContext(t.Context()), WithTTL(20*time.Millisecond)))
	expectRead(mock, "SELECT id", 1)
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.True(t, info.Hit)
	// The query TTL replaces the driver TTL, so the entry expires within it.
	time.Sleep(50 * time.Millisecond)
	expectRead(mock, "SELECT id", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
	require.False(t, info.Hit)
}
