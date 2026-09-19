// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
)

type generationLevel struct {
	Level
	GenerationStore
}

func TestDriverOptions(t *testing.T) {
	t.Run("invalid options are rejected", func(t *testing.T) {
		for _, option := range []Option{TTL(0), TTL(-time.Second), SWR(-time.Second), HotTable(-time.Second, 1), HotTable(time.Second, -1), SingleflightWait(0), SingleflightWait(-time.Second), Levels(), Levels(nil), Generations(nil), OnError(nil), SkewMargin(-1), TxReads(99), Schema(nil)} {
			_, err := New(&gatedDriver{}, Levels(ContextLevel()), option)
			require.Error(t, err)
		}
	})

	t.Run("a driver needs a target and a level", func(t *testing.T) {
		_, err := New(nil, Levels(ContextLevel()))
		require.Error(t, err)
		_, err = New(&gatedDriver{}, Levels())
		require.Error(t, err)
	})

	t.Run("a driver without a database reports none", func(t *testing.T) {
		cached, err := New(&gatedDriver{}, Levels(ContextLevel()))
		require.NoError(t, err)
		require.Nil(t, cached.DB())
	})

	t.Run("the last level providing generations wins", func(t *testing.T) {
		first := &generationLevel{Level: NewMemory(100, 0), GenerationStore: NewMemoryGenerations()}
		last := &generationLevel{Level: NewMemory(100, 0), GenerationStore: NewMemoryGenerations()}
		cached, mock := newTestDriver(t, Levels(first, last))
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))

		require.NoError(t, NewInvalidator(first).Invalidate(t.Context(), Tables("users")))
		readIDs(t, cached, ctx, "SELECT id")
		require.True(t, info.Hit, "an unused generation store must not invalidate entries")

		require.NoError(t, NewInvalidator(last).Invalidate(t.Context(), Tables("users")))
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})

	t.Run("an explicit generation store overrides the levels", func(t *testing.T) {
		level := &generationLevel{Level: NewMemory(100, 0), GenerationStore: NewMemoryGenerations()}
		store := NewMemoryGenerations()
		cached, mock := newTestDriver(t, Levels(level), Generations(store))
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))

		require.NoError(t, NewInvalidator(level).Invalidate(t.Context(), Tables("users")))
		readIDs(t, cached, ctx, "SELECT id")
		require.True(t, info.Hit)

		require.NoError(t, NewInvalidator(store).Invalidate(t.Context(), Tables("users")))
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})

	t.Run("entries expire after the configured ttl", func(t *testing.T) {
		cached, mock := newTestDriver(t, TTL(50*time.Millisecond))
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		time.Sleep(80 * time.Millisecond)
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})

	t.Run("a per-query ttl overrides the driver ttl", func(t *testing.T) {
		cached, mock := newTestDriver(t, TTL(time.Hour))
		ctx, info := WithInfo(Cache(readContext(t.Context()), WithTTL(50*time.Millisecond)))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		time.Sleep(80 * time.Millisecond)
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})
}

func TestSchemaPrefix(t *testing.T) {
	table := schema.NewTable("users").SetSchema("public").
		AddColumn(&schema.Column{Name: "id", Type: field.TypeInt64}).
		AddColumn(&schema.Column{Name: "email", Type: field.TypeString, Nullable: true}).
		AddIndex("email_unique", true, []string{"email"})
	other := schema.NewTable("groups").AddColumn(&schema.Column{Name: "id", Type: field.TypeInt64})

	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, database.Close())
	})
	// One shared level and one shared generation store, so only the schema can
	// separate the drivers' entries.
	level, store := NewMemory(100, 0), NewMemoryGenerations()
	newDriver := func(t *testing.T, tables ...*schema.Table) *Driver {
		t.Helper()
		cached, err := New(entsql.OpenDB(dialect.SQLite, database), Levels(level), Generations(store), Schema(tables...))
		require.NoError(t, err)
		return cached
	}
	ctx, info := WithInfo(Cache(readContext(t.Context())))

	t.Run("an equal schema shares entries", func(t *testing.T) {
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, newDriver(t, table, other), ctx, "SELECT id"))
		require.False(t, info.Hit)
		require.Equal(t, []int64{1}, readIDs(t, newDriver(t, other, table), ctx, "SELECT id"))
		require.True(t, info.Hit, "table order must not change the schema prefix")
	})

	for name, mutate := range map[string]func(){
		"column nullability": func() { table.Columns[1].Nullable = false },
		"column type":        func() { table.Columns[1].Type = field.TypeBytes },
		"column name":        func() { table.Columns[1].Name = "address" },
		"unique index":       func() { table.Indexes[0].Columns = []*schema.Column{table.Columns[0]} },
		"table name":         func() { table.Name = "customers" },
	} {
		t.Run("a changed "+name+" isolates entries", func(t *testing.T) {
			mutate()
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, newDriver(t, table, other), ctx, "SELECT id"))
			require.False(t, info.Hit)
		})
	}
}

func TestViewsAndDependencies(t *testing.T) {
	views := map[string][]string{"user_view": {"users", "group_view"}, "group_view": {"groups", "user_view"}}
	option := Views(views)
	views["user_view"][0] = "changed"
	cached, mock := newTestDriver(t, option)
	statement := &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"user_view"}, PointRead: true, RequestedIDs: []any{1}}
	ctx, info := WithInfo(Cache(dialect.WithStatement(t.Context(), statement), DependsOn("subscriptions")))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	require.Equal(t, []string{"user_view", "subscriptions", "users", "group_view", "groups"}, info.Tables)
	require.Equal(t, []string{"user_view"}, statement.Tables, "expansion must not change the caller's statement")
	readIDs(t, cached, ctx, "SELECT id")
	for _, table := range []string{"users", "groups", "subscriptions"} {
		t.Run("a write to "+table+" invalidates the view", func(t *testing.T) {
			require.NoError(t, cached.Invalidate(t.Context(), Tables(table)))
			expectRead(mock, "SELECT id", 2)
			readIDs(t, cached, ctx, "SELECT id")
			require.False(t, info.Hit)
		})
	}
}

func TestDriverLimits(t *testing.T) {
	cached, mock := newTestDriver(t, WithLimits(Limits{MaxRows: 1}))
	ctx := Cache(readContext(t.Context()))
	for range 2 {
		expectRead(mock, "SELECT id", 1, 2)
		require.Equal(t, []int64{1, 2}, readIDs(t, cached, ctx, "SELECT id"))
	}
	require.Zero(t, cached.Stats().Fills.Load())
	require.Equal(t, uint64(2), cached.Stats().DroppedFills.Load())
}

func TestTableOptionsOwnTheirInput(t *testing.T) {
	// Every option is built from one slice that is mutated before the driver sees it.
	tables := []string{"users"}
	allowed, skipped, ttlOnly := CacheTables(tables...), SkipTables(tables...), TTLOnlyTables(tables...)
	tables[0] = "changed"

	t.Run("cache tables still admits the table", func(t *testing.T) {
		cached, mock := newTestDriver(t, allowed)
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.Empty(t, info.Bypass)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.True(t, info.Hit)
	})

	t.Run("skip tables still bypasses the table", func(t *testing.T) {
		cached, mock := newTestDriver(t, skipped)
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		for range 2 {
			expectRead(mock, "SELECT id", 1)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.Equal(t, BypassSkip, info.Bypass)
		}
	})

	t.Run("ttl-only tables still ignore invalidation", func(t *testing.T) {
		cached, mock := newTestDriver(t, ttlOnly)
		ctx, info := WithInfo(Cache(readContext(t.Context())))
		expectRead(mock, "SELECT id", 1)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.NoError(t, cached.Invalidate(t.Context(), Tables("users")))
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.True(t, info.Hit)
	})
}

func TestTagsOptionAppliesToBothSides(t *testing.T) {
	var invalidation InvalidateOption = Tags("feed")
	var query QueryOption = Tags("feed")
	require.NoError(t, NewInvalidator(NewMemoryGenerations()).Invalidate(context.Background(), invalidation))
	require.Equal(t, []string{"feed"}, queryOptionsFrom(Cache(t.Context(), query)).tags)
}
