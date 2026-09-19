// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"sync/atomic"
)

const (
	BypassTx          = "tx"
	BypassLock        = "lock"
	BypassNoStatement = "no-statement"
	BypassGeneration  = "gen-failure"
	BypassDisabled    = "disabled"
	BypassSkip        = "skip"
	BypassKey         = "key"
)

// Stats is safe for concurrent updates. It must not be copied after first use.
type Stats struct {
	Misses             atomic.Uint64
	Fills              atomic.Uint64
	StaleHits          atomic.Uint64
	Refreshes          atomic.Uint64
	DroppedFills       atomic.Uint64
	SingleflightShared atomic.Uint64
	hits               sync.Map
	bypasses           sync.Map
	bumps              sync.Map
	hotTables          *hotTableTracker
}

type StatsSnapshot struct {
	Hits               map[string]uint64
	Misses             uint64
	Fills              uint64
	StaleHits          uint64
	Refreshes          uint64
	DroppedFills       uint64
	Bypasses           map[string]uint64
	SingleflightShared uint64
	Bumps              map[string]uint64
	// DemotedTables lists automatically demoted tables, excluding manual policies.
	DemotedTables []string
}

func (s *Stats) RecordHit(level string) {
	increment(&s.hits, level)
}

func (s *Stats) RecordBypass(reason string) {
	increment(&s.bypasses, reason)
}

func (s *Stats) RecordBump(table string) {
	increment(&s.bumps, table)
}

func increment(counters *sync.Map, key string) {
	counter, found := counters.Load(key)
	if !found {
		counter, _ = counters.LoadOrStore(key, new(atomic.Uint64))
	}
	counter.(*atomic.Uint64).Add(1)
}

func counterValues(counters *sync.Map) map[string]uint64 {
	values := make(map[string]uint64)
	counters.Range(func(key, counter any) bool {
		values[key.(string)] = counter.(*atomic.Uint64).Load()
		return true
	})
	return values
}

// Snapshot reads counters independently and reports current demotion state.
func (s *Stats) Snapshot() StatsSnapshot {
	return StatsSnapshot{
		Hits:               counterValues(&s.hits),
		Misses:             s.Misses.Load(),
		Fills:              s.Fills.Load(),
		StaleHits:          s.StaleHits.Load(),
		Refreshes:          s.Refreshes.Load(),
		DroppedFills:       s.DroppedFills.Load(),
		Bypasses:           counterValues(&s.bypasses),
		SingleflightShared: s.SingleflightShared.Load(),
		Bumps:              counterValues(&s.bumps),
		DemotedTables:      s.hotTables.snapshot(),
	}
}
