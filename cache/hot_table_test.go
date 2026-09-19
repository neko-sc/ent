// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

func TestHotTableDemotionAndRecovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cached, mock := newTestDriver(t, HotTable(10*time.Second, 2))
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")

		for index := range 3 {
			if index > 0 {
				time.Sleep(4 * time.Second)
			}
			executeWrite(t, cached, mock, writeContext(t.Context(), 1, 2))
			if index < 2 {
				require.Empty(t, cached.Stats().Snapshot().DemotedTables)
			} else {
				require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
			}
			expectRead(mock, "SELECT id", int64(index+2))
			readIDs(t, cached, ctx, "SELECT id")
			require.False(t, info.Hit)
		}
		executeWrite(t, cached, mock, writeContext(t.Context(), 1))
		require.Equal(t, []int64{4}, readIDs(t, cached, ctx, "SELECT id"))
		require.True(t, info.Hit)

		snapshot := cached.Stats().Snapshot()
		require.Equal(t, uint64(4), snapshot.Bumps["users"])
		snapshot.DemotedTables[0] = "changed"
		require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
		time.Sleep(2 * time.Second)
		require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
		time.Sleep(4 * time.Second)
		// Recovery requires neither a new bump nor a stats read, even while two
		// bumps remain in the sliding window.
		expectRead(mock, "SELECT id", 5)
		require.Equal(t, []int64{5}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
		require.Empty(t, cached.Stats().Snapshot().DemotedTables)
		time.Sleep(10 * time.Second)
		executeWrite(t, cached, mock, writeContext(t.Context(), 1))
		expectRead(mock, "SELECT id", 6)
		readIDs(t, cached, ctx, "SELECT id")
		require.False(t, info.Hit)
		require.Empty(t, cached.Stats().Snapshot().DemotedTables)
	})
}

func TestHotTablePointReads(t *testing.T) {
	for _, test := range []struct {
		name    string
		context func(context.Context) context.Context
		ids     []int64
	}{
		{name: "primary key", context: func(ctx context.Context) context.Context { return readContext(ctx, 1) }, ids: []int64{1}},
		{name: "unique", context: pointReadContext, ids: []int64{1}},
		{name: "missing unique", context: pointReadContext},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t, HotTable(time.Hour, 1), SkewMargin(0))
			ctx, info := WithInfo(Cache(test.context(t.Context())))
			expectRead(mock, "SELECT id", test.ids...)
			readIDs(t, cached, ctx, "SELECT id")
			for range 2 {
				executeWrite(t, cached, mock, writeContext(t.Context(), 2))
			}
			require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
			if len(test.ids) == 0 {
				expectRead(mock, "SELECT id")
			}
			readIDs(t, cached, ctx, "SELECT id")
			require.Equal(t, len(test.ids) > 0, info.Hit)
			require.True(t, info.PointRead)
			for _, ids := range [][]any{{1}, nil} {
				executeWrite(t, cached, mock, writeContext(t.Context(), ids...))
				expectRead(mock, "SELECT id", 1)
				readIDs(t, cached, ctx, "SELECT id")
				require.False(t, info.Hit)
			}
		})
	}
}

func TestHotTableDependencies(t *testing.T) {
	for _, pointRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "list", true: "point"}[pointRead], func(t *testing.T) {
			cached, mock := newTestDriver(t, HotTable(time.Hour, 1), Views(map[string][]string{"user_view": {"users"}}))
			ctx, info := WithInfo(Cache(dialect.WithStatement(t.Context(), &dialect.Statement{
				Kind: dialect.StatementRead, Tables: []string{"user_view", "groups"}, PointRead: pointRead, RequestedIDs: []any{1},
			}), DependsOn("users")))
			for range 2 {
				require.NoError(t, cached.Invalidate(t.Context(), Rows("users", 2)))
			}
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			require.NoError(t, cached.Invalidate(t.Context(), Rows("users", 2)))
			if pointRead {
				expectRead(mock, "SELECT id", 1)
			}
			readIDs(t, cached, ctx, "SELECT id")
			require.Equal(t, !pointRead, info.Hit)
			require.NoError(t, cached.Invalidate(t.Context(), Tables("groups")))
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
			require.False(t, info.Hit, "a cold dependency must still invalidate")
			require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
		})
	}
}

func TestHotTableTTLAndExplicitInvalidation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cached, mock := newTestDriver(t, HotTable(time.Hour, 1), TTL(time.Second))
		ctx, info := WithInfo(Cache(readContext(t.Context()), Tags("feed")))
		for range 2 {
			require.NoError(t, cached.Invalidate(t.Context(), Tables("users")))
		}
		expectRead(mock, "SELECT id", 1)
		readIDs(t, cached, ctx, "SELECT id")
		require.NoError(t, cached.Invalidate(t.Context(), Tables("users")))
		readIDs(t, cached, ctx, "SELECT id")
		require.True(t, info.Hit)
		time.Sleep(time.Second)
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
		for _, option := range []InvalidateOption{Tags("feed"), All()} {
			require.NoError(t, cached.Invalidate(t.Context(), option))
			expectRead(mock, "SELECT id", 3)
			readIDs(t, cached, ctx, "SELECT id")
			require.False(t, info.Hit)
			require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
		}
	})
}

func TestHotTableOptions(t *testing.T) {
	for _, test := range []struct {
		name   string
		option Option
	}{
		{name: "zero window and bumps", option: HotTable(0, 0)},
		{name: "zero window", option: HotTable(0, 1)},
		{name: "zero bumps", option: HotTable(time.Second, 0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t, HotTable(time.Hour, 1), test.option)
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			for range 3 {
				expectRead(mock, "SELECT id", 1)
				readIDs(t, cached, ctx, "SELECT id")
				require.False(t, info.Hit)
				executeWrite(t, cached, mock, writeContext(t.Context(), 1))
			}
			require.Empty(t, cached.Stats().Snapshot().DemotedTables)
		})
	}

	t.Run("default threshold", func(t *testing.T) {
		cached, _ := newTestDriver(t)
		for range 50 {
			require.NoError(t, cached.Invalidate(t.Context(), Tables("users")))
		}
		require.Empty(t, cached.Stats().Snapshot().DemotedTables)
		require.NoError(t, cached.Invalidate(t.Context(), Tables("users")))
		require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
	})

	t.Run("manual policy survives recovery", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cached, mock := newTestDriver(t, HotTable(time.Second, 1), TTLOnlyTables("users"))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			for range 2 {
				executeWrite(t, cached, mock, writeContext(t.Context(), 1))
			}
			require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)
			time.Sleep(time.Second)
			require.Empty(t, cached.Stats().Snapshot().DemotedTables)
			readIDs(t, cached, ctx, "SELECT id")
			require.True(t, info.Hit)
		})
	})
}

func TestHotTableOnlyCountsSuccessfulBumps(t *testing.T) {
	cached, mock := newTestDriver(t, HotTable(time.Hour, 1))
	for range 2 {
		mock.ExpectExec("UPDATE users").WillReturnError(errors.New("write failed"))
		_, err := cached.Exec(writeContext(t.Context()), "UPDATE users", nil)
		require.Error(t, err)
		require.NoError(t, cached.Invalidate(t.Context(), Tags("users"), All()))
		mock.ExpectBegin()
		transaction, err := cached.Tx(t.Context())
		require.NoError(t, err)
		executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
		mock.ExpectRollback()
		require.NoError(t, transaction.Rollback())
	}
	require.Empty(t, cached.Stats().Snapshot().Bumps)
	require.Empty(t, cached.Stats().Snapshot().DemotedTables)
	for index := range 2 {
		mock.ExpectBegin()
		transaction, err := cached.Tx(t.Context())
		require.NoError(t, err)
		for range 2 {
			executeWrite(t, transaction, mock, writeContext(t.Context(), 1, 2))
		}
		require.Empty(t, cached.Stats().Snapshot().DemotedTables)
		mock.ExpectCommit()
		require.NoError(t, transaction.Commit())
		require.Equal(t, uint64(index+1), cached.Stats().Snapshot().Bumps["users"])
	}
	require.Equal(t, []string{"users"}, cached.Stats().Snapshot().DemotedTables)

	failed, _ := newTestDriver(t, HotTable(time.Hour, 1), Generations(&failingGenerations{bumpError: errors.New("bump failed")}))
	for range 2 {
		require.Error(t, failed.Invalidate(t.Context(), Tables("users")))
	}
	require.Empty(t, failed.Stats().Snapshot().DemotedTables)
}

func TestHotTableConcurrentReadsAndBumps(t *testing.T) {
	cached, mock := newTestDriver(t, HotTable(time.Hour, 2), Singleflight(false))
	cached.DB().SetMaxOpenConns(1)
	mock.MatchExpectationsInOrder(false)
	for range 100 {
		expectRead(mock, "SELECT id", 1)
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			ctx := Cache(readContext(t.Context()), NoFill())
			for range 25 {
				if err := cached.Invalidate(t.Context(), Rows("users", 1, 2), Tables("groups")); err != nil {
					t.Error(err)
				}
				rows, err := cached.Query(ctx, "SELECT id", nil)
				if err != nil {
					t.Error(err)
					return
				}
				if err := rows.Close(); err != nil {
					t.Error(err)
				}
				// Reading stats concurrently exercises demotion state under the race detector.
				cached.Stats().Snapshot()
			}
		})
	}
	workers.Wait()
	require.Equal(t, []string{"groups", "users"}, cached.Stats().Snapshot().DemotedTables)
	require.Equal(t, uint64(100), cached.Stats().Snapshot().Bumps["users"])
}
