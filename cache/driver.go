// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/neko-sc/ent/dialect"
)

// As with database/sql, callers must close Rows to release database resources.
type Driver struct {
	dialect.Driver
	levels           []Level
	generations      GenerationStore
	ttl              time.Duration
	swr              time.Duration
	limits           Limits
	global           bool
	cacheTables      set
	skipTables       set
	ttlOnlyTables    set
	hotTables        *hotTableTracker
	views            map[string][]string
	txReads          TxReadPolicy
	singleflight     bool
	singleflightWait time.Duration
	skewMargin       uint64
	schemaPrefix     [16]byte
	stats            Stats
	onError          func(context.Context, error)
	builders         sync.Pool
	mutex            sync.Mutex
	inflight         map[Key]*flight
}

func New(driver dialect.Driver, options ...Option) (*Driver, error) {
	if driver == nil {
		return nil, fmt.Errorf("cache: nil driver")
	}

	cached := &Driver{
		Driver: driver,
		ttl:    time.Minute,
		hotTables: &hotTableTracker{
			window:     10 * time.Second,
			bumps:      50,
			timestamps: make(map[string][]time.Time),
		},
		singleflight:     true,
		singleflightWait: 2 * time.Second,
		skewMargin:       100_000,
		inflight:         make(map[Key]*flight),
		onError:          func(_ context.Context, err error) { log.Printf("cache: %v", err) },
	}

	for _, option := range options {
		if err := option(cached); err != nil {
			return nil, err
		}
	}

	if cached.ttl <= 0 || len(cached.levels) == 0 {
		return nil, fmt.Errorf("cache: positive TTL and at least one cache level required")
	}
	if cached.generations == nil {
		for _, level := range cached.levels {
			if store, ok := level.(GenerationStore); ok {
				cached.generations = store
			}
		}
		if cached.generations == nil {
			cached.generations = NewMemoryGenerations()
		}
	}

	cached.stats.hotTables = cached.hotTables
	cached.builders.New = func() any { return &keyBuilder{prefix: cached.schemaPrefix} }
	return cached, nil
}

func (d *Driver) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	return (&executor{ExecQuerier: d.Driver, driver: d}).Query(ctx, query, arguments)
}

func (d *Driver) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	return (&executor{ExecQuerier: d.Driver, driver: d}).Exec(ctx, query, arguments)
}

func (d *Driver) DB() *sql.DB {
	if driver, ok := d.Driver.(interface{ DB() *sql.DB }); ok {
		return driver.DB()
	}
	return nil
}

func (d *Driver) Stats() *Stats {
	return &d.stats
}

type executor struct {
	dialect.ExecQuerier
	driver      *Driver
	transaction *Tx
}

func (d *Driver) expand(statement *dialect.Statement, options queryOptions) *dialect.Statement {
	expanded := *statement
	expanded.Tables = make([]string, 0, len(statement.Tables)+len(options.dependsOn))
	seen := make(set)

	for _, tables := range [2][]string{statement.Tables, options.dependsOn} {
		for _, table := range tables {
			if _, found := seen[table]; !found {
				seen[table] = struct{}{}
				expanded.Tables = append(expanded.Tables, table)
			}
		}
	}

	// Appending while walking also expands nested views without looping on cycles.
	for index := 0; index < len(expanded.Tables); index++ {
		for _, table := range d.views[expanded.Tables[index]] {
			if _, found := seen[table]; !found {
				seen[table] = struct{}{}
				expanded.Tables = append(expanded.Tables, table)
			}
		}
	}

	return &expanded
}

func (e *executor) bypass(ctx context.Context, reason string) {
	e.driver.stats.RecordBypass(reason)
	if info := infoFrom(ctx); info != nil {
		info.Bypass = reason
	}
}
