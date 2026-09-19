// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"time"
)

type Level interface {
	// Get returns (nil, nil) on a miss.
	Get(ctx context.Context, key Key) (*Entry, error)
	Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error
	Delete(ctx context.Context, keys ...Key) error
}
