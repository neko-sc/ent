// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMemoryLevelConformance(t *testing.T) {
	for _, limits := range [][2]int{{0, 0}, {10, 0}, {0, 100}, {10, 100}} {
		t.Run(fmt.Sprintf("entries=%d bytes=%d", limits[0], limits[1]), func(t *testing.T) {
			testLevel(t, t.Context(), NewMemory(limits[0], int64(limits[1])))
		})
	}
	t.Run("negative limits panic", func(t *testing.T) {
		require.Panics(t, func() { NewMemory(-1, 0) })
		require.Panics(t, func() { NewMemory(0, -1) })
	})
}

// testLevel asserts the contract every Level must satisfy.
func testLevel(t *testing.T, ctx context.Context, level Level) {
	t.Helper()
	key := Key{1}
	entry := recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64))

	t.Run("miss on an unknown key", func(t *testing.T) {
		cached, err := level.Get(ctx, key)
		require.NoError(t, err)
		require.Nil(t, cached)
	})

	t.Run("set then get returns the same entry", func(t *testing.T) {
		require.NoError(t, level.Set(ctx, key, entry, time.Minute))
		cached, err := level.Get(ctx, key)
		require.NoError(t, err)
		require.Same(t, entry, cached)
	})

	t.Run("del removes the entry and tolerates unknown keys", func(t *testing.T) {
		require.NoError(t, level.Set(ctx, key, entry, time.Minute))
		require.NoError(t, level.Delete(ctx, key, Key{2}))
		cached, err := level.Get(ctx, key)
		require.NoError(t, err)
		require.Nil(t, cached)
	})

	t.Run("entries expire", func(t *testing.T) {
		require.NoError(t, level.Set(ctx, key, entry, 10*time.Millisecond))
		require.Eventually(t, func() bool {
			cached, err := level.Get(ctx, key)
			return err == nil && cached == nil
		}, time.Second, time.Millisecond)
	})

	t.Run("nonpositive ttl removes the entry", func(t *testing.T) {
		require.NoError(t, level.Set(ctx, key, entry, time.Minute))
		require.NoError(t, level.Set(ctx, key, entry, 0))
		cached, err := level.Get(ctx, key)
		require.NoError(t, err)
		require.Nil(t, cached)
	})

	t.Run("nil entry removes the entry", func(t *testing.T) {
		require.NoError(t, level.Set(ctx, key, entry, time.Minute))
		require.NoError(t, level.Set(ctx, key, nil, time.Minute))
		cached, err := level.Get(ctx, key)
		require.NoError(t, err)
		require.Nil(t, cached)
	})

	t.Run("canceled context fails every operation", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := level.Get(canceled, key)
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, level.Set(canceled, key, entry, time.Minute), context.Canceled)
		require.ErrorIs(t, level.Delete(canceled, key), context.Canceled)
	})
}

// present counts how many of the first count keys the level still serves.
func present(t *testing.T, level Level, count int) int {
	t.Helper()
	var found int
	for i := range count {
		entry, err := level.Get(t.Context(), Key{byte(i)})
		require.NoError(t, err)
		if entry != nil {
			found++
		}
	}
	return found
}

func TestMemoryLimits(t *testing.T) {
	// Each entry weighs ten bytes: a two-byte column name and one integer cell.
	for _, limits := range [][2]int{{2, 1000}, {100, 20}, {0, 20}, {2, 0}} {
		t.Run(fmt.Sprintf("entries=%d bytes=%d", limits[0], limits[1]), func(t *testing.T) {
			level := NewMemory(limits[0], int64(limits[1]))
			admitted := limits[0]
			if limits[1] > 0 && (admitted == 0 || limits[1]/10 < admitted) {
				admitted = limits[1] / 10
			}
			for i := range 20 {
				require.NoError(t, level.Set(t.Context(), Key{byte(i)}, recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64)), time.Minute))
			}
			require.Eventually(t, func() bool {
				return present(t, level, 20) <= admitted
			}, time.Second, 10*time.Millisecond, "the level must evict down to its bound")

			if limits[1] > 0 {
				oversized := recordEntry(t, "id", []string{"id"}, [][]any{{make([]byte, limits[1])}}, new([]byte))
				require.NoError(t, level.Set(t.Context(), Key{99}, oversized, time.Minute))
				entry, err := level.Get(t.Context(), Key{99})
				require.NoError(t, err)
				require.Nil(t, entry, "an entry larger than the byte bound must not be admitted")
			}
		})
	}
}

func TestMemoryConcurrent(t *testing.T) {
	level := NewMemory(5, 1000)
	entry := recordEntry(t, "id", []string{"id"}, [][]any{{int64(1)}}, new(int64))
	var workers sync.WaitGroup
	for i := range 20 {
		workers.Go(func() {
			for range 20 {
				if err := level.Set(t.Context(), Key{byte(i)}, entry, time.Minute); err != nil {
					t.Error(err)
				}
				if _, err := level.Get(t.Context(), Key{byte(i)}); err != nil {
					t.Error(err)
				}
				if err := level.Delete(t.Context(), Key{byte(i)}); err != nil {
					t.Error(err)
				}
			}
		})
	}
	workers.Wait()
	require.Eventually(t, func() bool {
		return present(t, level, 20) <= 5
	}, time.Second, 10*time.Millisecond)
}
