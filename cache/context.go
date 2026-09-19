// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"slices"
	"sync"
	"time"
)

type contextKey uint8

const (
	levelContextKey contextKey = iota
	queryContextKey
	skipContextKey
	touchesContextKey
	infoContextKey
)

type contextEntry struct {
	entry   *Entry
	expires time.Time
}

type contextEntries struct {
	mutex   sync.Mutex
	entries map[Key]contextEntry
}

func NewContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, levelContextKey, &contextEntries{entries: make(map[Key]contextEntry)})
}

type contextLevel struct{}

func ContextLevel() Level {
	return contextLevel{}
}

func (contextLevel) Name() string {
	return "context"
}

func (contextLevel) Get(ctx context.Context, key Key) (*Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, ok := ctx.Value(levelContextKey).(*contextEntries)
	if !ok {
		return nil, nil
	}
	entries.mutex.Lock()
	defer entries.mutex.Unlock()
	entry, found := entries.entries[key]
	if !found {
		return nil, nil
	}
	if !time.Now().Before(entry.expires) {
		delete(entries.entries, key)
		return nil, nil
	}
	return entry.entry, nil
}

func (contextLevel) Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, ok := ctx.Value(levelContextKey).(*contextEntries)
	if !ok {
		return nil
	}
	entries.mutex.Lock()
	defer entries.mutex.Unlock()
	if entry == nil || ttl <= 0 {
		delete(entries.entries, key)
		return nil
	}
	entries.entries[key] = contextEntry{entry: entry, expires: time.Now().Add(ttl)}
	return nil
}

func (contextLevel) Delete(ctx context.Context, keys ...Key) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, ok := ctx.Value(levelContextKey).(*contextEntries)
	if !ok {
		return nil
	}
	entries.mutex.Lock()
	defer entries.mutex.Unlock()
	for _, key := range keys {
		delete(entries.entries, key)
	}
	return nil
}

type queryOptions struct {
	enabled   bool
	ttl       time.Duration
	swr       time.Duration
	swrSet    bool
	tags      []string
	ttlOnly   bool
	dependsOn []string
	noFill    bool
}

type QueryOption interface {
	applyQuery(*queryOptions)
}

type queryOption func(*queryOptions)

func (option queryOption) applyQuery(options *queryOptions) {
	option(options)
}

func Cache(ctx context.Context, options ...QueryOption) context.Context {
	settings := queryOptionsFrom(ctx)
	settings.enabled = true
	for _, option := range options {
		option.applyQuery(&settings)
	}
	return context.WithValue(ctx, queryContextKey, settings)
}

func WithTTL(duration time.Duration) QueryOption {
	return queryOption(func(options *queryOptions) { options.ttl = duration })
}

// WithSWR overrides the driver's stale window for this query. Nonpositive
// durations disable stale-while-revalidate.
func WithSWR(duration time.Duration) QueryOption {
	return queryOption(func(options *queryOptions) {
		options.swr = max(duration, 0)
		options.swrSet = true
	})
}

// Tags can annotate a cached query or invalidate entries carrying the same user tags.
func Tags(tags ...string) tagOption {
	return tagOption{tags: slices.Clone(tags)}
}

type tagOption struct {
	tags []string
}

func (option tagOption) applyQuery(options *queryOptions) {
	options.tags = append(slices.Clone(options.tags), option.tags...)
}

func (option tagOption) applyInvalidation(tags []string) ([]string, error) {
	for _, tag := range option.tags {
		tags = append(tags, "~"+tag)
	}
	return tags, nil
}

func TTLOnly() QueryOption {
	return queryOption(func(options *queryOptions) { options.ttlOnly = true })
}

func DependsOn(tables ...string) QueryOption {
	tables = slices.Clone(tables)
	return queryOption(func(options *queryOptions) {
		options.dependsOn = append(slices.Clone(options.dependsOn), tables...)
	})
}

func NoFill() QueryOption {
	return queryOption(func(options *queryOptions) { options.noFill = true })
}

// Skip is honored by the driver only when global caching is enabled.
func Skip(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipContextKey, true)
}

func Touches(ctx context.Context, tables ...string) context.Context {
	return context.WithValue(ctx, touchesContextKey, slices.Clone(tables))
}

// Info belongs to one query; callers must not share it between concurrent queries.
type Info struct {
	Hit       bool
	Stale     bool
	Level     string
	Tables    []string
	PointRead bool
	Bypass    string
}

func WithInfo(ctx context.Context) (context.Context, *Info) {
	info := &Info{}
	return context.WithValue(ctx, infoContextKey, info), info
}

func queryOptionsFrom(ctx context.Context) queryOptions {
	options, _ := ctx.Value(queryContextKey).(queryOptions)
	options.tags = slices.Clone(options.tags)
	options.dependsOn = slices.Clone(options.dependsOn)
	return options
}

func infoFrom(ctx context.Context) *Info {
	info, _ := ctx.Value(infoContextKey).(*Info)
	return info
}

func touchesFrom(ctx context.Context) []string {
	tables, _ := ctx.Value(touchesContextKey).([]string)
	return slices.Clone(tables)
}

func skipFrom(ctx context.Context) bool {
	skip, _ := ctx.Value(skipContextKey).(bool)
	return skip
}
