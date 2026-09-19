// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"slices"
	"sync"
	"time"
)

type hotTableTracker struct {
	mutex      sync.Mutex
	window     time.Duration
	bumps      int
	timestamps map[string][]time.Time
}

func (tracker *hotTableTracker) record(table string) {
	if tracker == nil {
		return
	}
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	now := time.Now()
	timestamps := tracker.recent(table, now)
	// Only the latest threshold+1 bumps are needed to decide demotion.
	if len(timestamps) > tracker.bumps {
		timestamps = timestamps[len(timestamps)-tracker.bumps:]
	}
	tracker.timestamps[table] = append(timestamps, now)
}

func (tracker *hotTableTracker) demoted(table string) bool {
	if tracker == nil {
		return false
	}
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	return len(tracker.recent(table, time.Now())) > tracker.bumps
}

func (tracker *hotTableTracker) snapshot() []string {
	if tracker == nil {
		return nil
	}
	tracker.mutex.Lock()
	defer tracker.mutex.Unlock()
	now := time.Now()
	var tables []string
	for table := range tracker.timestamps {
		if len(tracker.recent(table, now)) > tracker.bumps {
			tables = append(tables, table)
		}
	}
	slices.Sort(tables)
	return tables
}

// recent removes expired bumps while the caller holds the mutex.
func (tracker *hotTableTracker) recent(table string, now time.Time) []time.Time {
	timestamps := tracker.timestamps[table]
	cutoff := now.Add(-tracker.window)
	for len(timestamps) > 0 && !timestamps[0].After(cutoff) {
		timestamps = timestamps[1:]
	}
	if len(timestamps) == 0 {
		delete(tracker.timestamps, table)
	} else {
		tracker.timestamps[table] = timestamps
	}
	return timestamps
}
