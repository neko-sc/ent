// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/maypok86/otter/v2"
)

type memoryValue struct {
	entry *Entry
	ttl   time.Duration
}

type memoryLevel struct {
	cache      *otter.Cache[Key, memoryValue]
	mutex      sync.Mutex
	maxEntries int
	maxBytes   int64
}

// NewMemory bounds entries and estimated bytes independently; zero disables a bound.
// Negative bounds panic. Nonpositive Set TTLs remove an existing entry.
func NewMemory(maxEntries int, maxBytes int64) Level {
	if maxEntries < 0 || maxBytes < 0 {
		panic("cache: memory limits must be nonnegative")
	}

	options := &otter.Options[Key, memoryValue]{
		ExpiryCalculator: otter.ExpiryWritingFunc(func(entry otter.Entry[Key, memoryValue]) time.Duration {
			return entry.Value.ttl
		}),
	}
	if maxBytes > 0 {
		options.MaximumWeight = uint64(maxBytes)
		options.Weigher = func(_ Key, value memoryValue) uint32 {
			return uint32(min(max(value.entry.estimatedBytes, 1), math.MaxUint32))
		}
	} else {
		options.MaximumSize = maxEntries
	}

	return &memoryLevel{cache: otter.Must(options), maxEntries: maxEntries, maxBytes: maxBytes}
}

func (*memoryLevel) Name() string {
	return "memory"
}

func (m *memoryLevel) Get(ctx context.Context, key Key) (*Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, found := m.cache.GetIfPresent(key)
	if !found {
		return nil, nil
	}
	return value.entry, nil
}

func (m *memoryLevel) Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if entry == nil || ttl <= 0 || (m.maxBytes > 0 && entry.estimatedBytes > min(m.maxBytes, math.MaxUint32)) {
		m.cache.Invalidate(key)
		return nil
	}
	if m.maxBytes > 0 && m.maxEntries > 0 {
		// Otter makes MaximumSize and MaximumWeight mutually exclusive. Serialize
		// admission and trim the coldest entries to enforce the second bound.
		m.mutex.Lock()
		defer m.mutex.Unlock()
		m.cache.Set(key, memoryValue{entry: entry, ttl: ttl})
		m.cache.CleanUp()
		for m.cache.EstimatedSize() > m.maxEntries {
			var coldest Key
			found := false
			for candidate := range m.cache.Coldest() {
				coldest, found = candidate.Key, true
				break
			}
			if !found {
				break
			}
			m.cache.Invalidate(coldest)
		}
		return nil
	}

	m.cache.Set(key, memoryValue{entry: entry, ttl: ttl})

	return nil
}

func (m *memoryLevel) Delete(ctx context.Context, keys ...Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	for _, key := range keys {
		m.cache.Invalidate(key)
	}

	return nil
}
