// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"slices"
	"strings"

	"github.com/neko-sc/ent/dialect"
)

func (e *executor) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	result, err := e.ExecQuerier.Exec(ctx, query, arguments)
	if err == nil {
		e.wrote(ctx, writeTags(dialect.StatementFrom(ctx), touchesFrom(ctx)))
	}
	return result, err
}

func (e *executor) writeQuery(
	ctx context.Context, query string, arguments []any, statement *dialect.Statement,
) (dialect.Rows, error) {
	rows, err := e.ExecQuerier.Query(ctx, query, arguments)
	if err != nil {
		return nil, err
	}
	wrapped := &writeRows{Rows: rows, statement: statement, touches: touchesFrom(ctx)}
	if e.transaction != nil {
		e.transaction.dirty.Store(true)
		e.transaction.mutex.Lock()
		e.transaction.openWrites[wrapped] = struct{}{}
		e.transaction.mutex.Unlock()
		wrapped.finish = func() {
			e.transaction.mutex.Lock()
			defer e.transaction.mutex.Unlock()
			if _, open := e.transaction.openWrites[wrapped]; open {
				e.transaction.pending = append(e.transaction.pending, writeTags(statement, wrapped.touches)...)
				delete(e.transaction.openWrites, wrapped)
			}
		}
	} else {
		wrapped.finish = func() { e.wrote(ctx, writeTags(statement, wrapped.touches)) }
	}
	return wrapped, nil
}

func (e *executor) wrote(ctx context.Context, tags []string) {
	if e.transaction != nil {
		e.transaction.dirty.Store(true)
		e.transaction.mutex.Lock()
		e.transaction.pending = append(e.transaction.pending, tags...)
		e.transaction.mutex.Unlock()
		return
	}
	if err := e.driver.bump(context.WithoutCancel(ctx), tags); err != nil {
		e.driver.onError(ctx, err)
	}
}

func (d *Driver) bump(ctx context.Context, tags []string) error {
	if len(tags) == 0 {
		return nil
	}

	slices.Sort(tags)
	tags = slices.Compact(tags)

	if err := d.generations.Bump(ctx, tags); err != nil {
		return err
	}
	tables := make(set)
	for _, tag := range tags {
		if tag == "*" || strings.HasPrefix(tag, "~") {
			continue
		}
		table, _, _ := strings.Cut(strings.TrimSuffix(tag, "!bulk"), "#")
		tables[table] = struct{}{}
	}

	for table := range tables {
		d.stats.RecordBump(table)
		d.hotTables.record(table)
	}

	return nil
}

type writeRows struct {
	dialect.Rows
	closed    bool
	statement *dialect.Statement
	touches   []string
	finish    func()
}

func (r *writeRows) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	err := r.Rows.Close()
	// Even an interrupted RETURNING stream may have changed rows. Invalidation
	// is conservative rather than trusting Close to report whether it committed.
	r.finish()
	return err
}
