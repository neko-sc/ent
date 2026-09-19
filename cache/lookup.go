// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"fmt"
	"time"
)

func (d *Driver) valid(ctx context.Context, entry *Entry) (bool, error) {
	if len(entry.validationTags) == 0 {
		return true, nil
	}
	tokens, err := d.generations.Load(ctx, entry.validationTags)
	if err != nil {
		return false, err
	}
	margin := d.skewMargin
	if _, local := d.generations.(*memoryGenerations); local {
		// The in-process store orders fills and writes on one monotonic clock.
		margin = 0
	}
	if entry.fillStart <= margin {
		return false, nil
	}
	for _, token := range tokens {
		if token >= entry.fillStart-margin {
			return false, nil
		}
	}
	return true, nil
}

// remaining separates freshness from storage lifetime, which includes the stale window.
func (d *Driver) remaining(ctx context.Context, entry *Entry) (freshness, storage time.Duration, err error) {
	now, err := d.generations.Now(ctx)
	if err != nil {
		return 0, 0, err
	}
	if entry.expiresAt > now {
		//nolint:gosec // Clamp before converting unsigned microseconds.
		freshness = time.Duration(min(entry.expiresAt-now, uint64((1<<63-1)/time.Microsecond))) * time.Microsecond
	}
	if entry.staleUntil > now {
		//nolint:gosec // Clamp before converting unsigned microseconds.
		storage = time.Duration(min(entry.staleUntil-now, uint64((1<<63-1)/time.Microsecond))) * time.Microsecond
	}
	return freshness, max(freshness, storage), nil
}

func levelName(level Level) string {
	if named, ok := level.(interface{ Name() string }); ok {
		return named.Name()
	}
	return fmt.Sprintf("%T", level)
}

func (d *Driver) hit(ctx context.Context, name string) {
	d.stats.RecordHit(name)
	if info := infoFrom(ctx); info != nil {
		info.Hit = true
		info.Level = name
	}
}
