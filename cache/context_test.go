// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestContextLevel(t *testing.T) {
	level := ContextLevel()

	t.Run("level contract", func(t *testing.T) {
		testLevel(t, NewContext(t.Context()), level)
	})

	t.Run("a context without a cache stores nothing", func(t *testing.T) {
		require.NoError(t, level.Set(t.Context(), Key{1}, recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64)), time.Minute))
		entry, err := level.Get(t.Context(), Key{1})
		require.NoError(t, err)
		require.Nil(t, entry)
		require.NoError(t, level.Delete(t.Context(), Key{1}))
	})

	t.Run("entries are isolated per context", func(t *testing.T) {
		first, second := NewContext(t.Context()), NewContext(t.Context())
		require.NoError(t, level.Set(first, Key{1}, recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64)), time.Minute))
		entry, err := level.Get(second, Key{1})
		require.NoError(t, err)
		require.Nil(t, entry)
		entry, err = level.Get(Cache(first), Key{1})
		require.NoError(t, err)
		require.NotNil(t, entry, "derived contexts share the cache of their parent")
	})
}

func TestContextOptions(t *testing.T) {
	t.Run("a bare context carries nothing", func(t *testing.T) {
		require.Equal(t, queryOptions{}, queryOptionsFrom(t.Context()))
		require.Nil(t, infoFrom(t.Context()))
		require.Nil(t, touchesFrom(t.Context()))
		require.False(t, skipFrom(t.Context()))
	})

	t.Run("cache records every option", func(t *testing.T) {
		ctx := Cache(t.Context(), WithTTL(time.Minute), Tags("feed"), TTLOnly(), DependsOn("users"), NoFill())
		require.Equal(t, queryOptions{enabled: true, ttl: time.Minute, tags: []string{"feed"}, ttlOnly: true, dependsOn: []string{"users"}, noFill: true}, queryOptionsFrom(ctx))
	})

	t.Run("a derived context extends its parent without changing it", func(t *testing.T) {
		ctx := Cache(t.Context(), WithTTL(time.Minute), Tags("feed"), DependsOn("users"))
		child := Cache(ctx, WithTTL(time.Second), Tags("child"), DependsOn("groups"))
		require.Equal(t, time.Second, queryOptionsFrom(child).ttl)
		require.Equal(t, []string{"feed", "child"}, queryOptionsFrom(child).tags)
		require.Equal(t, []string{"users", "groups"}, queryOptionsFrom(child).dependsOn)
		require.Equal(t, []string{"feed"}, queryOptionsFrom(ctx).tags)
	})

	t.Run("options own their slices", func(t *testing.T) {
		ctx := Cache(t.Context(), Tags("feed"))
		settings := queryOptionsFrom(ctx)
		settings.tags[0] = "mutated"
		require.Equal(t, []string{"feed"}, queryOptionsFrom(ctx).tags)
		tags := []string{"original"}
		option := Tags(tags...)
		tags[0] = "mutated"
		require.Equal(t, []string{"original"}, queryOptionsFrom(Cache(t.Context(), option)).tags)
	})

	t.Run("skip is recorded", func(t *testing.T) {
		require.True(t, skipFrom(Skip(t.Context())))
	})

	t.Run("touches own their tables", func(t *testing.T) {
		ctx := Touches(t.Context(), "users")
		require.Equal(t, []string{"users"}, touchesFrom(ctx))
		tables := touchesFrom(ctx)
		tables[0] = "mutated"
		require.Equal(t, []string{"users"}, touchesFrom(ctx))
	})

	t.Run("info is shared with the caller", func(t *testing.T) {
		ctx, info := WithInfo(t.Context())
		require.Same(t, info, infoFrom(ctx))
		info.Hit = true
		require.True(t, infoFrom(ctx).Hit)
	})
}

func TestContextConcurrent(t *testing.T) {
	ctx := NewContext(t.Context())
	level := ContextLevel()
	entry := recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64))
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Go(func() {
			for range 20 {
				if err := level.Set(ctx, Key{byte(i)}, entry, time.Minute); err != nil {
					t.Error(err)
				}
				if _, err := level.Get(ctx, Key{byte(i)}); err != nil {
					t.Error(err)
				}
				if err := level.Delete(ctx, Key{byte(i)}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	workers.Wait()
}
