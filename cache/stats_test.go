// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatsCountersAndSnapshot(t *testing.T) {
	var stats Stats
	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			for range 100 {
				stats.RecordHit("memory")
				stats.RecordBypass(BypassTx)
				stats.RecordBump("users")
				stats.Misses.Add(1)
				stats.Fills.Add(1)
				stats.DroppedFills.Add(1)
				stats.SingleflightShared.Add(1)
				stats.Snapshot()
			}
		})
	}
	workers.Wait()
	require.Equal(t, StatsSnapshot{Hits: map[string]uint64{"memory": 1000}, Misses: 1000, Fills: 1000, DroppedFills: 1000, Bypasses: map[string]uint64{BypassTx: 1000}, SingleflightShared: 1000, Bumps: map[string]uint64{"users": 1000}}, stats.Snapshot())
	snapshot := stats.Snapshot()
	snapshot.Hits["memory"] = 0
	require.Equal(t, uint64(1000), stats.Snapshot().Hits["memory"])
}
