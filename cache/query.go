// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"database/sql/driver"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
)

// Raw SQL has no structural metadata, so false positives in literals or comments
// are preferable to missing a writable CTE. Generated statements use their metadata.
var rawWriteKeyword = regexp.MustCompile(`(?i)\b(?:INSERT|UPDATE|DELETE|MERGE)\b`)

func (e *executor) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	options := queryOptionsFrom(ctx)
	statement := dialect.StatementFrom(ctx)
	if info := infoFrom(ctx); info != nil {
		*info = Info{}
	}
	if statement == nil {
		trimmed := strings.ToUpper(strings.TrimSpace(query))
		if strings.HasPrefix(trimmed, "SELECT") ||
			(strings.HasPrefix(trimmed, "WITH") && !rawWriteKeyword.MatchString(query)) {
			if options.enabled || (e.driver.global && !skipFrom(ctx)) {
				e.bypass(ctx, BypassNoStatement)
			}
			return e.ExecQuerier.Query(ctx, query, arguments)
		}
		return e.writeQuery(ctx, query, arguments, nil)
	}
	if statement.Kind == dialect.StatementWrite {
		return e.writeQuery(ctx, query, arguments, statement)
	}

	statement = e.driver.expand(statement, options)
	if info := infoFrom(ctx); info != nil {
		info.Tables = slices.Clone(statement.Tables)
		info.PointRead = statement.PointRead
	}
	if reason := e.readBypass(ctx, statement, options); reason != "" {
		return e.bypassQuery(ctx, reason, nil, query, arguments)
	}
	now, err := e.driver.generations.Now(ctx)
	if err != nil {
		return e.bypassQuery(ctx, BypassGeneration, err, query, arguments)
	}
	key, reason, err := e.driver.entryKey(ctx, query, arguments, statement, options)
	if err != nil {
		return e.bypassQuery(ctx, reason, err, query, arguments)
	}

	ttl := e.driver.ttl
	if options.ttl > 0 {
		ttl = options.ttl
	}
	if !options.swrSet {
		options.swr = e.driver.swr
	}

	for index, level := range e.driver.levels {
		entry, err := level.Get(ctx, key)
		if err != nil {
			e.driver.onError(ctx, err)
			continue
		}
		if entry == nil {
			continue
		}
		remaining, storage, err := e.driver.remaining(ctx, entry)
		if err != nil {
			return e.bypassQuery(ctx, BypassGeneration, err, query, arguments)
		}
		if storage <= 0 || (remaining <= 0 && options.swr <= 0) {
			continue
		}
		valid, err := e.driver.valid(ctx, entry)
		if err != nil {
			if remaining <= 0 {
				e.driver.onError(ctx, err)
				continue
			}
			return e.bypassQuery(ctx, BypassGeneration, err, query, arguments)
		}
		if !valid {
			continue
		}
		if err := entry.decode(); err != nil {
			e.driver.onError(ctx, err)
			continue
		}
		e.driver.hit(ctx, levelName(level))
		if remaining <= 0 {
			e.driver.stats.StaleHits.Add(1)
			if info := infoFrom(ctx); info != nil {
				info.Stale = true
			}
		}
		if !options.noFill {
			for _, earlier := range e.driver.levels[:index] {
				if err := earlier.Set(context.WithoutCancel(ctx), key, entry, storage); err != nil {
					e.driver.onError(ctx, err)
				}
			}
			// Backfill before refreshing so a fast refresh cannot be overwritten by stale data.
			if remaining <= 0 && e.transaction == nil {
				e.driver.triggerRefresh(ctx, key, query, arguments, options, statement, ttl)
			}
		}
		return replay(entry), nil
	}

	e.driver.stats.Misses.Add(1)
	var leader *flight
	if e.driver.singleflight {
		e.driver.mutex.Lock()
		for key, pending := range e.driver.inflight {
			if !pending.refreshing && time.Since(pending.started) >= e.driver.singleflightWait {
				delete(e.driver.inflight, key)
			}
		}
		pending := e.driver.inflight[key]
		if pending == nil {
			leader = &flight{done: make(chan struct{}), started: time.Now()}
			e.driver.inflight[key] = leader
		}
		e.driver.mutex.Unlock()
		if pending != nil {
			timer := time.NewTimer(max(e.driver.singleflightWait-time.Since(pending.started), 0))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
				e.driver.mutex.Lock()
				if e.driver.inflight[key] == pending && !pending.refreshing {
					delete(e.driver.inflight, key)
				}
				e.driver.mutex.Unlock()
				pending = nil
			case <-pending.done:
				timer.Stop()
			}
			if pending != nil && pending.entry != nil {
				remaining, _, err := e.driver.remaining(ctx, pending.entry)
				if err != nil {
					return e.bypassQuery(ctx, BypassGeneration, err, query, arguments)
				}
				valid, err := e.driver.valid(ctx, pending.entry)
				if err != nil {
					return e.bypassQuery(ctx, BypassGeneration, err, query, arguments)
				}
				if valid && remaining > 0 {
					e.driver.stats.SingleflightShared.Add(1)
					e.driver.hit(ctx, "singleflight")
					return replay(pending.entry), nil
				}
			}
		}
	}

	rows, err := e.ExecQuerier.Query(ctx, query, arguments)
	if err != nil {
		e.driver.resolve(key, leader, nil)
		return nil, err
	}
	recorded := newRecorder(rows, statement.IDColumn, e.driver.limits)
	return &recordedRows{recorder: recorded, finish: func() {
		entry, complete := recorded.Entry()
		if !complete {
			e.driver.stats.DroppedFills.Add(1)
			e.driver.resolve(key, leader, nil)
			return
		}
		e.driver.resolve(key, leader, e.driver.fill(ctx, key, entry, statement, options, now, ttl))
	}}, nil
}

func (d *Driver) fill(
	ctx context.Context, key Key, entry *Entry, statement *dialect.Statement,
	options queryOptions, now uint64, ttl time.Duration,
) *Entry {
	ctx = context.WithoutCancel(ctx)
	entry.fillStart = now
	entry.expiresAt = now + uint64(ttl.Microseconds()) //nolint:gosec // Effective TTL is positive.
	if options.swr > 0 {
		//nolint:gosec // The stale window is positive.
		entry.staleUntil = entry.expiresAt + uint64(options.swr.Microseconds())
	}

	remaining, storage, err := d.remaining(ctx, entry)
	if err != nil || remaining <= 0 {
		if err != nil {
			d.onError(ctx, err)
		}
		d.stats.DroppedFills.Add(1)
		return nil
	}
	if !options.ttlOnly {
		entry.validationTags = validationTags(statement, entry.ids, d.ttlOnlyTables)
	}
	if !options.noFill {
		for _, level := range d.levels {
			if err := level.Set(ctx, key, entry, storage); err != nil {
				d.onError(ctx, err)
			}
		}
		d.stats.Fills.Add(1)
	}

	return entry
}

// triggerRefresh registers a flight so concurrent stale readers share one reload,
// and copies everything the reload needs so it survives the caller's values and deadline.
func (d *Driver) triggerRefresh(
	ctx context.Context, key Key, query string, arguments []any,
	options queryOptions, statement *dialect.Statement, ttl time.Duration,
) {
	d.mutex.Lock()
	if d.inflight[key] != nil {
		d.mutex.Unlock()
		return
	}
	pending := &flight{done: make(chan struct{}), started: time.Now(), refreshing: true}
	d.inflight[key] = pending
	d.mutex.Unlock()

	// Values owned by the caller may be reused as soon as the stale read returns.
	arguments = slices.Clone(arguments)
	for index, argument := range arguments {
		if valuer, ok := argument.(driver.Valuer); ok {
			value, err := valuer.Value()
			if err != nil {
				d.resolve(key, pending, nil)
				d.onError(ctx, err)
				return
			}
			argument = value
		}
		value, err := snapshot(&argument)
		if err != nil {
			d.resolve(key, pending, nil)
			d.onError(ctx, err)
			return
		}
		arguments[index] = value
	}
	requested, err := snapshot(&statement.RequestedIDs)
	if err != nil {
		d.resolve(key, pending, nil)
		d.onError(ctx, err)
		return
	}

	detached := *statement
	detached.Tables = slices.Clone(statement.Tables)
	detached.RequestedIDs = requested.([]any)
	background, cancel := context.WithTimeout(context.Background(), ttl+30*time.Second)
	background = dialect.WithStatement(background, &detached)
	for _, variable := range sql.VarsFromContext(ctx) {
		background = sql.WithVar(background, variable[0], variable[1])
	}
	if entries, ok := ctx.Value(levelContextKey).(*contextEntries); ok {
		background = context.WithValue(background, levelContextKey, entries)
	}
	go func() {
		defer cancel()
		d.resolve(key, pending, d.refresh(background, key, query, arguments, options, &detached, ttl))
	}()
}

// refresh returns the reloaded entry only when it still belongs to key, which
// a generation bump between the stale read and the reload can change.
func (d *Driver) refresh(
	ctx context.Context, key Key, query string, arguments []any,
	options queryOptions, statement *dialect.Statement, ttl time.Duration,
) *Entry {
	d.stats.Refreshes.Add(1)
	now, err := d.generations.Now(ctx)
	if err != nil {
		d.onError(ctx, err)
		return nil
	}
	refreshedKey, _, err := d.entryKey(ctx, query, arguments, statement, options)
	if err != nil {
		d.onError(ctx, err)
		return nil
	}
	rows, err := d.Driver.Query(ctx, query, arguments)
	if err != nil {
		d.onError(ctx, err)
		return nil
	}

	recorded := newRecorder(rows, statement.IDColumn, d.limits)
	destinations := make([]any, len(recorded.entry.columns))
	for index := range destinations {
		destinations[index] = new(any)
	}

	for recorded.Next() {
		if err := recorded.Scan(destinations...); err != nil {
			d.onError(ctx, err)
			break
		}
	}
	if err := recorded.Err(); err != nil {
		d.onError(ctx, err)
	}
	if err := recorded.Close(); err != nil {
		d.onError(ctx, err)
	}

	entry, complete := recorded.Entry()
	if !complete {
		d.stats.DroppedFills.Add(1)
		return nil
	}

	// Refreshes scan driver values, not the caller's concrete types. Retain encoded
	// cells so replay can decode them into each caller's scan destinations instead.
	entry.encodedRows = make([][][]byte, len(entry.rows))
	for rowIndex, row := range entry.rows {
		entry.encodedRows[rowIndex] = make([][]byte, len(row))
		for columnIndex, value := range row {
			entry.encodedRows[rowIndex][columnIndex], err = encodeValue(value)
			if err != nil {
				d.onError(ctx, err)
				d.stats.DroppedFills.Add(1)
				return nil
			}
		}
	}

	entry = d.fill(ctx, refreshedKey, entry, statement, options, now, ttl)
	if refreshedKey != key {
		return nil
	}
	return entry
}

// The returned reason names the bypass to record when derivation fails.
func (d *Driver) entryKey(
	ctx context.Context, query string, arguments []any,
	statement *dialect.Statement, options queryOptions,
) (Key, string, error) {
	tags := keyTags(statement, options, d.ttlOnlyTables, d.hotTables)
	tokens, err := d.generations.Load(ctx, tags)
	if err != nil {
		return Key{}, BypassGeneration, err
	}
	builder := d.builders.Get().(*keyBuilder)
	defer d.builders.Put(builder)
	if err := builder.query(string(d.Dialect()), query, arguments); err != nil {
		return Key{}, BypassKey, err
	}
	builder.variables(sql.VarsFromContext(ctx))
	builder.tags(tags, tokens)
	return builder.sum(), "", nil
}

func (e *executor) bypassQuery(
	ctx context.Context, reason string, err error,
	query string, arguments []any,
) (dialect.Rows, error) {
	e.bypass(ctx, reason)
	if err != nil {
		e.driver.onError(ctx, err)
	}
	return e.ExecQuerier.Query(ctx, query, arguments)
}

func (e *executor) readBypass(ctx context.Context, statement *dialect.Statement, options queryOptions) string {
	if !options.enabled && (!e.driver.global || skipFrom(ctx)) {
		if e.driver.global && skipFrom(ctx) {
			return BypassSkip
		}
		return BypassDisabled
	}
	if len(statement.Tables) > 0 {
		if _, skipped := e.driver.skipTables[statement.Tables[0]]; skipped {
			return BypassSkip
		}
		if len(e.driver.cacheTables) > 0 {
			if _, allowed := e.driver.cacheTables[statement.Tables[0]]; !allowed {
				return BypassDisabled
			}
		}
	} else if len(e.driver.cacheTables) > 0 {
		return BypassDisabled
	}
	if statement.Lock {
		return BypassLock
	}
	if e.transaction != nil && (e.driver.txReads == TxReadsBypass || e.transaction.dirty.Load()) {
		return BypassTx
	}
	return ""
}

type recordedRows struct {
	*recorder
	finish func()
}

func (r *recordedRows) Close() error {
	if r.closed {
		return nil
	}
	err := r.recorder.Close()
	r.finish()
	return err
}
