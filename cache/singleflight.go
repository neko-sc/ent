// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import "time"

type flight struct {
	done       chan struct{}
	entry      *Entry
	started    time.Time
	refreshing bool
}

func (d *Driver) resolve(key Key, pending *flight, entry *Entry) {
	if pending == nil {
		return
	}
	d.mutex.Lock()
	defer d.mutex.Unlock()
	pending.entry = entry
	// An expired leader must not remove its replacement's registration.
	if d.inflight[key] == pending {
		delete(d.inflight, key)
	}
	close(pending.done)
}
