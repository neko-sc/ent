// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryGenerations(t *testing.T) {
	t.Run("now is strictly monotonic", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		previous, err := store.Now(t.Context())
		require.NoError(t, err)
		for range 100 {
			current, err := store.Now(t.Context())
			require.NoError(t, err)
			require.Greater(t, current, previous)
			previous = current
		}
	})

	t.Run("unwritten tags predate the current time", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		now, err := store.Now(t.Context())
		require.NoError(t, err)
		tokens, err := store.Load(t.Context(), []string{"users", "*", "users"})
		require.NoError(t, err)
		require.Len(t, tokens, 3)
		require.NotZero(t, tokens[0])
		require.Less(t, tokens[0], now, "an unwritten tag must be older than a fill starting now")
		require.Equal(t, tokens[0], tokens[2], "one tag has one token per load")
		require.Equal(t, tokens[0], tokens[1], "every unwritten tag shares the origin")
	})

	t.Run("load is stable without writes", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		tags := []string{"users", "*", "users"}
		tokens, err := store.Load(t.Context(), tags)
		require.NoError(t, err)
		again, err := store.Load(t.Context(), tags)
		require.NoError(t, err)
		require.Equal(t, tokens, again)
	})

	t.Run("bump raises only the named tags", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		before, err := store.Load(t.Context(), []string{"users", "groups"})
		require.NoError(t, err)
		require.NoError(t, store.Bump(t.Context(), []string{"users"}))
		after, err := store.Load(t.Context(), []string{"users", "groups"})
		require.NoError(t, err)
		require.Greater(t, after[0], before[0])
		require.Equal(t, before[1], after[1])
	})

	t.Run("repeated bumps of one tag keep advancing", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		tokens, err := store.Load(t.Context(), []string{"users"})
		require.NoError(t, err)
		previous := tokens[0]
		for range 10 {
			// A tag named twice in one call must advance twice, so no fill
			// starting between the two writes can be considered valid.
			require.NoError(t, store.Bump(t.Context(), []string{"users", "users"}))
			tokens, err := store.Load(t.Context(), []string{"users"})
			require.NoError(t, err)
			require.Greater(t, tokens[0], previous)
			previous = tokens[0]
		}
	})

	t.Run("now orders after every bump", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		require.NoError(t, store.Bump(t.Context(), []string{"users", "users", "users"}))
		tokens, err := store.Load(t.Context(), []string{"users"})
		require.NoError(t, err)
		now, err := store.Now(t.Context())
		require.NoError(t, err)
		require.Greater(t, now, tokens[0], "a fill starting after a write must outrank it")
		require.NoError(t, store.Bump(t.Context(), []string{"users"}))
		tokens, err = store.Load(t.Context(), []string{"users"})
		require.NoError(t, err)
		require.Greater(t, tokens[0], now, "a write after a fill must invalidate it")
	})

	t.Run("canceled context fails every operation", func(t *testing.T) {
		store := NewMemoryGenerations(100)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := store.Load(ctx, []string{"users"})
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, store.Bump(ctx, []string{"users"}), context.Canceled)
		_, err = store.Now(ctx)
		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestMemoryGenerationsExpiry(t *testing.T) {
	t.Run("expired tags never move backward", func(t *testing.T) {
		store := NewMemoryGenerations(4)
		last := make(map[string]uint64)
		// Every tag recurs only after more than 2*4 other tags, so each
		// revisit after the first round reads an expired tag.
		for round := range 300 {
			tag := fmt.Sprintf("users#%02x", round%23)
			if round%2 == 0 {
				tag = fmt.Sprintf("~feed%d", round%19)
			}
			before, err := store.Load(t.Context(), []string{tag})
			require.NoError(t, err)
			require.GreaterOrEqual(t, before[0], last[tag])
			require.NoError(t, store.Bump(t.Context(), []string{tag}))
			after, err := store.Load(t.Context(), []string{tag})
			require.NoError(t, err)
			require.Greater(t, after[0], before[0])
			last[tag] = after[0]
		}
	})

	t.Run("tags expired before any write keep their token", func(t *testing.T) {
		store := NewMemoryGenerations(1)
		first, err := store.Load(t.Context(), []string{"users#01"})
		require.NoError(t, err)
		_, err = store.Load(t.Context(), []string{"users#02"})
		require.NoError(t, err)
		again, err := store.Load(t.Context(), []string{"users#01"})
		require.NoError(t, err)
		require.GreaterOrEqual(t, again[0], first[0])
	})

	t.Run("bounded and recently used tags survive rotations", func(t *testing.T) {
		store := NewMemoryGenerations(4)
		tags := []string{"*", "users", "users!bulk", "users#01", "~feed"}
		require.NoError(t, store.Bump(t.Context(), tags))
		before, err := store.Load(t.Context(), tags)
		require.NoError(t, err)
		for round := range 100 {
			// Bumping other tags advances the clock, so a dropped tag would
			// come back with a newer token.
			require.NoError(t, store.Bump(t.Context(), []string{fmt.Sprintf("groups#%02x", round)}))
			_, err := store.Load(t.Context(), []string{"users#01", "~feed"})
			require.NoError(t, err)
		}
		after, err := store.Load(t.Context(), tags)
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("retained tags stay bounded", func(t *testing.T) {
		store := NewMemoryGenerations(4)
		for round := range 100 {
			tag := fmt.Sprintf("groups#%02x", round)
			_, err := store.Load(t.Context(), []string{tag})
			require.NoError(t, err)
			require.NoError(t, store.Bump(t.Context(), []string{tag, "~" + tag}))
		}
		memory := store.(*memoryGenerations)
		require.LessOrEqual(t, len(memory.bounded)+len(memory.current)+len(memory.previous), 2*4)
	})
}

func TestMemoryGenerationsConcurrent(t *testing.T) {
	store := NewMemoryGenerations(100)
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			for range 100 {
				tokens, err := store.Load(t.Context(), []string{"users", "*", "users"})
				if err != nil || len(tokens) != 3 || tokens[0] == 0 || tokens[0] != tokens[2] {
					t.Error("invalid generation load", tokens, err)
				}
				if err := store.Bump(t.Context(), []string{"users"}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	workers.Wait()
}
