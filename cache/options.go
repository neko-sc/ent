// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type Option func(*Driver) error

func TTL(duration time.Duration) Option {
	return func(driver *Driver) error {
		if duration <= 0 {
			return fmt.Errorf("cache: TTL must be positive")
		}
		driver.ttl = duration
		return nil
	}
}

// SWR permits serving expired entries while refreshing them in the background.
// A zero window disables stale-while-revalidate, which is the default.
func SWR(duration time.Duration) Option {
	return func(driver *Driver) error {
		if duration < 0 {
			return fmt.Errorf("cache: SWR must be nonnegative")
		}
		driver.swr = duration
		return nil
	}
}

func Levels(levels ...Level) Option {
	levels = slices.Clone(levels)
	return func(driver *Driver) error {
		for _, level := range levels {
			if level == nil {
				return fmt.Errorf("cache: nil cache level")
			}
		}
		driver.levels = levels
		return nil
	}
}

func Generations(store GenerationStore) Option {
	return func(driver *Driver) error {
		if store == nil {
			return fmt.Errorf("cache: nil generation store")
		}
		driver.generations = store
		return nil
	}
}

func Global(enabled bool) Option {
	return func(driver *Driver) error {
		driver.global = enabled
		return nil
	}
}

func CacheTables(tables ...string) Option {
	selected := newSet(tables)
	return func(driver *Driver) error {
		driver.cacheTables = selected
		return nil
	}
}

func SkipTables(tables ...string) Option {
	selected := newSet(tables)
	return func(driver *Driver) error {
		driver.skipTables = selected
		return nil
	}
}

func TTLOnlyTables(tables ...string) Option {
	selected := newSet(tables)
	return func(driver *Driver) error {
		driver.ttlOnlyTables = selected
		return nil
	}
}

// HotTable demotes list queries to TTL-only invalidation when a table exceeds
// bumps within window. Either value set to zero disables automatic demotion.
// The default is 50 bumps within 10 seconds. Point reads are not demoted.
func HotTable(window time.Duration, bumps int) Option {
	return func(driver *Driver) error {
		if window < 0 || bumps < 0 {
			return fmt.Errorf("cache: hot table window and bumps must be nonnegative")
		}
		driver.hotTables = nil
		if window > 0 && bumps > 0 {
			driver.hotTables = &hotTableTracker{
				window:     window,
				bumps:      bumps,
				timestamps: make(map[string][]time.Time),
			}
		}
		return nil
	}
}

type TxReadPolicy uint8

const (
	TxReadsBypass TxReadPolicy = iota
	// TxReadsUntilWrite is only safe at read committed isolation. Stronger
	// isolation may observe cached results newer than the transaction snapshot.
	TxReadsUntilWrite
)

func TxReads(policy TxReadPolicy) Option {
	return func(driver *Driver) error {
		if policy != TxReadsBypass && policy != TxReadsUntilWrite {
			return fmt.Errorf("cache: invalid transaction read policy %d", policy)
		}
		driver.txReads = policy
		return nil
	}
}

func WithLimits(limits Limits) Option {
	return func(driver *Driver) error {
		driver.limits = limits
		return nil
	}
}

func Singleflight(enabled bool) Option {
	return func(driver *Driver) error {
		driver.singleflight = enabled
		return nil
	}
}

// SingleflightWait bounds waiting for a leader whose caller may never close rows.
func SingleflightWait(duration time.Duration) Option {
	return func(driver *Driver) error {
		if duration <= 0 {
			return fmt.Errorf("cache: singleflight wait must be positive")
		}
		driver.singleflightWait = duration
		return nil
	}
}

func Views(views map[string][]string) Option {
	copied := make(map[string][]string, len(views))
	for name, tables := range views {
		copied[name] = slices.Clone(tables)
	}
	return func(driver *Driver) error {
		driver.views = copied
		return nil
	}
}

func OnError(handler func(context.Context, error)) Option {
	return func(driver *Driver) error {
		if handler == nil {
			return fmt.Errorf("cache: nil error handler")
		}
		driver.onError = handler
		return nil
	}
}

func SkewMargin(margin time.Duration) Option {
	return func(driver *Driver) error {
		if margin < 0 {
			return fmt.Errorf("cache: skew margin must be nonnegative")
		}
		//nolint:gosec // The duration is nonnegative and division cannot overflow.
		driver.skewMargin = uint64(margin.Microseconds())
		return nil
	}
}
