// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

// refreshGateDriver keeps the second database query pending until the test releases it.
type refreshGateDriver struct {
	dialect.Driver
	calls   atomic.Uint64
	entered chan struct{}
	release chan struct{}
}

func (d *refreshGateDriver) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	if d.calls.Add(1) == 2 {
		close(d.entered)
		select {
		case <-d.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return d.Driver.Query(ctx, query, arguments)
}

func newRefreshTestDriver(t *testing.T, options ...Option) (*Driver, sqlmock.Sqlmock, <-chan struct{}, func()) {
	t.Helper()
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	underlying := &refreshGateDriver{
		Driver:  entsql.OpenDB(dialect.SQLite, database),
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	cached, err := New(underlying, append([]Option{Levels(newMemoryLevel(t)), TTL(time.Second), SWR(5 * time.Second)}, options...)...)
	require.NoError(t, err)
	release := sync.OnceFunc(func() { close(underlying.release) })
	t.Cleanup(func() {
		release()
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, cached.Close())
	})
	return cached, mock, underlying.entered, release
}

func TestSWRServesStaleAndRefreshes(t *testing.T) {
	for _, name := range []string{"memory", "wire", "context"} {
		t.Run(name, func(t *testing.T) {
			store := newClockGenerations()
			var level Level
			switch name {
			case "memory":
				level = newMemoryLevel(t)
			case "wire":
				level = newWireLevel()
			case "context":
				level = ContextLevel()
			}
			cached, mock, entered, release := newRefreshTestDriver(t, Generations(store), Levels(level))
			ctx, info := WithInfo(Cache(readContext(NewContext(t.Context()))))
			expectRead(mock, "SELECT id", 1)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.False(t, info.Hit)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.True(t, info.Hit)
			require.False(t, info.Stale)
			store.advance()
			expectRead(mock, "SELECT id", 2, 3)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.True(t, info.Hit)
			require.True(t, info.Stale)
			require.Equal(t, name, info.Level)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not start")
			}
			release()
			require.Eventually(t, func() bool { return cached.Stats().Fills.Load() == 2 }, 5*time.Second, time.Millisecond)
			require.Equal(t, []int64{2, 3}, readIDs(t, cached, ctx, "SELECT id"))
			require.True(t, info.Hit)
			require.False(t, info.Stale)
			require.Equal(t, uint64(1), cached.Stats().Snapshot().StaleHits)
			require.Equal(t, uint64(1), cached.Stats().Snapshot().Refreshes)
			require.Equal(t, uint64(1), cached.Stats().Misses.Load())
		})
	}
}

func TestSWRInvalidatedEntryNotServed(t *testing.T) {
	for _, name := range []string{"key generation", "validation generation"} {
		t.Run(name, func(t *testing.T) {
			store := newClockGenerations()
			cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(5*time.Second))
			ctx := Cache(readContext(t.Context(), int64(1)))
			if name == "validation generation" {
				ctx = Cache(pointReadContext(t.Context()))
			}
			ctx, info := WithInfo(ctx)
			// Initialize row generations before filling a point read by a unique column.
			require.NoError(t, cached.Invalidate(t.Context(), Rows("users", int64(1))))
			store.advance()
			expectRead(mock, "SELECT id", 1)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.True(t, info.Hit)
			require.NoError(t, cached.Invalidate(t.Context(), Rows("users", int64(1))))
			store.advance()
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
			require.False(t, info.Hit)
			require.False(t, info.Stale)
			require.Zero(t, cached.Stats().StaleHits.Load())
			require.Zero(t, cached.Stats().Refreshes.Load())
		})
	}
}

func TestSWRBeyondWindow(t *testing.T) {
	for _, seconds := range []int{3, 4} {
		t.Run(map[int]string{3: "at deadline", 4: "past deadline"}[seconds], func(t *testing.T) {
			store := newClockGenerations()
			cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(2*time.Second))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			for range seconds {
				store.advance()
			}
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
			require.False(t, info.Hit)
			require.False(t, info.Stale)
			require.Zero(t, cached.Stats().StaleHits.Load())
			require.Zero(t, cached.Stats().Refreshes.Load())
		})
	}
}

func TestSWRRefreshDedup(t *testing.T) {
	for _, singleflight := range []bool{true, false} {
		t.Run(map[bool]string{true: "singleflight enabled", false: "singleflight disabled"}[singleflight], func(t *testing.T) {
			store := newClockGenerations()
			cached, mock, entered, release := newRefreshTestDriver(t, Generations(store), Singleflight(singleflight))
			ctx := Cache(readContext(t.Context()))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			store.advance()
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("refresh did not start")
			}
			var readers sync.WaitGroup
			for range 20 {
				readers.Go(func() {
					ctx, info := WithInfo(ctx)
					require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
					require.True(t, info.Stale)
				})
			}
			readers.Wait()
			require.Equal(t, uint64(1), cached.Stats().Refreshes.Load())
			require.Equal(t, uint64(21), cached.Stats().StaleHits.Load())
			release()
			require.Eventually(t, func() bool { return cached.Stats().Fills.Load() == 2 }, 5*time.Second, time.Millisecond)
			require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		})
	}
}

func TestSWRNoFillSuppressesRefresh(t *testing.T) {
	store := newClockGenerations()
	cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(5*time.Second))
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	store.advance()
	for range 3 {
		require.Equal(t, []int64{1}, readIDs(t, cached, Cache(ctx, NoFill()), "SELECT id"))
		require.True(t, info.Hit)
		require.True(t, info.Stale)
	}
	require.Equal(t, uint64(3), cached.Stats().StaleHits.Load())
	require.Zero(t, cached.Stats().Refreshes.Load())
	require.Equal(t, uint64(1), cached.Stats().Fills.Load())
}

func TestSWRDisabledByDefault(t *testing.T) {
	for _, name := range []string{"default", "explicit zero"} {
		t.Run(name, func(t *testing.T) {
			store := newClockGenerations()
			options := []Option{Generations(store), TTL(time.Second)}
			if name == "explicit zero" {
				options = append(options, SWR(0))
			}
			cached, mock := newTestDriver(t, options...)
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			store.advance()
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
			require.False(t, info.Hit)
			require.False(t, info.Stale)
			require.Zero(t, cached.Stats().StaleHits.Load())
			require.Zero(t, cached.Stats().Refreshes.Load())
		})
	}
}

func TestSWRPerQueryOverride(t *testing.T) {
	for _, test := range []struct {
		name     string
		window   time.Duration
		override time.Duration
		seconds  int
		stale    bool
	}{
		{name: "enable", override: 5 * time.Second, seconds: 1, stale: true},
		{name: "disable", window: 5 * time.Second, seconds: 1},
		{name: "shorten", window: 5 * time.Second, override: time.Second, seconds: 2},
		{name: "extend", window: time.Second, override: 5 * time.Second, seconds: 3, stale: true},
		{name: "negative disables", window: 5 * time.Second, override: -time.Second, seconds: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newClockGenerations()
			cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(test.window))
			ctx, info := WithInfo(Cache(readContext(t.Context()), WithSWR(test.override)))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			for range test.seconds {
				store.advance()
			}
			if test.stale {
				require.Equal(t, []int64{1}, readIDs(t, cached, Cache(ctx, NoFill()), "SELECT id"))
			} else {
				expectRead(mock, "SELECT id", 2)
				require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
			}
			require.Equal(t, test.stale, info.Hit)
			require.Equal(t, test.stale, info.Stale)
			require.Zero(t, cached.Stats().Refreshes.Load())
		})
	}
	t.Run("disable a previously enabled entry", func(t *testing.T) {
		store := newClockGenerations()
		cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(5*time.Second))
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
		store.advance()
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, Cache(ctx, WithSWR(0)), "SELECT id"))
		require.False(t, info.Stale)
		require.False(t, info.Hit)
		require.Zero(t, cached.Stats().Refreshes.Load())
	})
}

func TestSWRBackfillLifetime(t *testing.T) {
	for _, stale := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh backfill", true: "stale backfill"}[stale], func(t *testing.T) {
			store := newClockGenerations()
			front, back := newTrackingLevel("front"), newWireLevel()
			cached, mock := newTestDriver(t, Generations(store), Levels(front, back), TTL(2*time.Second), SWR(5*time.Second), TxReads(TxReadsUntilWrite))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			require.Equal(t, 7*time.Second, front.stored()[0].ttl)
			front.clear()
			store.advance()
			if stale {
				store.advance()
			}
			// A read-only transaction permits backfill but suppresses refresh.
			mock.ExpectBegin()
			transaction, err := cached.Tx(ctx)
			require.NoError(t, err)
			require.Equal(t, []int64{1}, readIDs(t, transaction, ctx, "SELECT id"))
			require.True(t, info.Hit)
			require.Equal(t, stale, info.Stale)
			require.Equal(t, "wire", info.Level)
			if stale {
				require.Equal(t, 5*time.Second, front.stored()[0].ttl)
			} else {
				require.Equal(t, 6*time.Second, front.stored()[0].ttl)
			}
			require.Equal(t, []int64{1}, readIDs(t, transaction, ctx, "SELECT id"))
			require.Equal(t, "front", info.Level)
			require.Zero(t, cached.Stats().Refreshes.Load())
			mock.ExpectRollback()
			require.NoError(t, transaction.Rollback())
		})
	}
}

func TestSWRRefreshFailure(t *testing.T) {
	for _, name := range []string{"query", "rows", "limit"} {
		t.Run(name, func(t *testing.T) {
			store := newClockGenerations()
			reported := make(chan error, 4)
			cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(5*time.Second), WithLimits(Limits{MaxRows: 1}), OnError(func(_ context.Context, err error) { reported <- err }))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			store.advance()
			switch name {
			case "query":
				mock.ExpectQuery("SELECT id").WillReturnError(errors.New("refresh failed"))
			case "rows":
				mock.ExpectQuery("SELECT id").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)).RowError(0, errors.New("refresh failed"))).RowsWillBeClosed()
			case "limit":
				expectRead(mock, "SELECT id", 2, 3)
			}
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.True(t, info.Stale)
			if name != "limit" {
				select {
				case err := <-reported:
					require.ErrorContains(t, err, "refresh failed")
				case <-time.After(5 * time.Second):
					t.Fatal("refresh error was not reported")
				}
			} else {
				require.Eventually(t, func() bool { return cached.Stats().DroppedFills.Load() == 1 }, 5*time.Second, time.Millisecond)
			}
			require.Equal(t, []int64{1}, readIDs(t, cached, Cache(ctx, NoFill()), "SELECT id"))
			require.True(t, info.Stale)
			expectRead(mock, "SELECT id", 4)
			require.Eventually(t, func() bool {
				rows, err := cached.Query(ctx, "SELECT id", nil)
				return err == nil && rows.Close() == nil && !info.Stale
			}, 5*time.Second, time.Millisecond)
			require.Equal(t, []int64{4}, readIDs(t, cached, ctx, "SELECT id"))
			require.Equal(t, uint64(2), cached.Stats().Refreshes.Load())
			require.Equal(t, uint64(2), cached.Stats().Fills.Load())
		})
	}
}

func TestSWRRefreshOwnsCallerData(t *testing.T) {
	store := newClockGenerations()
	cached, mock, entered, release := newRefreshTestDriver(t, Generations(store))
	statement := &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, IDColumn: "id"}
	ctx, cancel := context.WithCancel(Cache(dialect.WithStatement(t.Context(), statement)))
	defer cancel()
	arguments := []any{[]byte("original")}
	mock.ExpectQuery("SELECT id").WithArgs([]byte("original")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1))).RowsWillBeClosed()
	readIDs(t, cached, ctx, "SELECT id", arguments...)
	store.advance()
	mock.ExpectQuery("SELECT id").WithArgs([]byte("original")).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2))).RowsWillBeClosed()
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id", arguments...))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	arguments[0].([]byte)[0] = 'X'
	arguments[0] = "replaced"
	statement.Tables[0] = "changed"
	statement.IDColumn = "changed"
	cancel()
	release()
	require.Eventually(t, func() bool { return cached.Stats().Fills.Load() == 2 }, 5*time.Second, time.Millisecond)
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id", []byte("original")))
	require.True(t, info.Hit)
	require.False(t, info.Stale)
}

func TestSWRRefreshTypedDestinations(t *testing.T) {
	store := newClockGenerations()
	cached, mock, entered, release := newRefreshTestDriver(t, Generations(store))
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	store.advance()
	expectRead(mock, "SELECT id", 2)
	readIDs(t, cached, ctx, "SELECT id")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	release()
	require.Eventually(t, func() bool { return cached.Stats().Fills.Load() == 2 }, 5*time.Second, time.Millisecond)
	rows, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var identifier int
	require.NoError(t, rows.Scan(&identifier))
	require.Equal(t, 2, identifier)
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.True(t, info.Hit)
	require.False(t, info.Stale)
}

func TestSWRRefreshSharesMissFlight(t *testing.T) {
	store := newClockGenerations()
	cached, mock := newTestDriver(t, Generations(store), TTL(time.Second), SWR(5*time.Second))
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	store.advance()
	expectRead(mock, "SELECT id", 2)
	leader, err := cached.Query(Cache(ctx, WithSWR(0)), "SELECT id", nil)
	require.NoError(t, err)
	// The open miss owns this key's flight even though an eligible stale entry remains.
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.True(t, info.Stale)
	require.Zero(t, cached.Stats().Refreshes.Load())
	require.True(t, leader.Next())
	var identifier int64
	require.NoError(t, leader.Scan(&identifier))
	require.False(t, leader.Next())
	require.NoError(t, leader.Close())
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
	require.True(t, info.Hit)
	require.False(t, info.Stale)
}
