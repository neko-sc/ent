// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

type clockGenerations struct {
	mutex  sync.Mutex
	now    uint64
	tokens map[string]uint64
}

func newClockGenerations() *clockGenerations {
	return &clockGenerations{now: 1_000_000, tokens: make(map[string]uint64)}
}

func (g *clockGenerations) Now(context.Context) (uint64, error) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.now, nil
}

func (g *clockGenerations) Load(_ context.Context, tags []string) ([]uint64, error) {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	tokens := make([]uint64, len(tags))
	for index, tag := range tags {
		if g.tokens[tag] == 0 {
			g.tokens[tag] = g.now
		}
		tokens[index] = g.tokens[tag]
	}
	return tokens, nil
}

func (g *clockGenerations) Bump(_ context.Context, tags []string) error {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	for _, tag := range tags {
		g.tokens[tag] = max(g.tokens[tag]+1, g.now)
	}
	return nil
}

// advance moves the clock one second forward.
func (g *clockGenerations) advance() {
	g.mutex.Lock()
	g.now += 1_000_000
	g.mutex.Unlock()
}

// wireLevel keeps entries in their encoded form, like a remote cache level.
type wireLevel struct {
	mutex   sync.Mutex
	entries map[Key][]byte
}

func newWireLevel() *wireLevel {
	return &wireLevel{entries: make(map[Key][]byte)}
}

func (*wireLevel) Name() string {
	return "wire"
}

func (l *wireLevel) Get(_ context.Context, key Key) (*Entry, error) {
	l.mutex.Lock()
	encoded, found := l.entries[key]
	l.mutex.Unlock()
	if !found {
		return nil, nil
	}
	return DecodeEntry(encoded)
}

func (l *wireLevel) Set(_ context.Context, key Key, entry *Entry, ttl time.Duration) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if entry == nil || ttl <= 0 {
		delete(l.entries, key)
		return nil
	}
	encoded, err := entry.Bytes()
	if err != nil {
		return err
	}
	l.entries[key] = encoded
	return nil
}

func (l *wireLevel) Delete(_ context.Context, keys ...Key) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
	return nil
}

func pointReadContext(ctx context.Context) context.Context {
	return dialect.WithStatement(ctx, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, PointRead: true, IDColumn: "id"})
}

func TestDriverUniqueValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		ids  []int64
	}{
		{name: "row found", ids: []int64{1}},
		{name: "row missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newClockGenerations()
			cached, mock := newTestDriver(t, Generations(store))
			ctx, info := WithInfo(Cache(pointReadContext(t.Context())))
			// Missing validation tokens are initialized at the first lookup, so the
			// first entry must be rejected. A later fill uses the initialized tokens.
			expectRead(mock, "SELECT unique", test.ids...)
			readIDs(t, cached, ctx, "SELECT unique")
			expectRead(mock, "SELECT unique", test.ids...)
			readIDs(t, cached, ctx, "SELECT unique")
			require.False(t, info.Hit)
			store.advance()
			expectRead(mock, "SELECT unique", test.ids...)
			readIDs(t, cached, ctx, "SELECT unique")
			require.Equal(t, test.ids, readIDs(t, cached, ctx, "SELECT unique"))
			require.True(t, info.Hit)
			if len(test.ids) > 0 {
				// Writes to other rows leave a validated point read intact.
				executeWrite(t, cached, mock, writeContext(t.Context(), 2))
				readIDs(t, cached, ctx, "SELECT unique")
				require.True(t, info.Hit)
			}
			executeWrite(t, cached, mock, writeContext(t.Context(), 1))
			expectRead(mock, "SELECT unique", 3)
			require.Equal(t, []int64{3}, readIDs(t, cached, ctx, "SELECT unique"))
			require.False(t, info.Hit)
		})
	}
}

func TestDriverFillStartPrecedesQuery(t *testing.T) {
	store := newClockGenerations()
	cached, mock := newTestDriver(t, Generations(store))
	ctx := Cache(pointReadContext(t.Context()))
	// The first read initializes the validation tokens of the returned row.
	expectRead(mock, "SELECT unique", 1)
	readIDs(t, cached, ctx, "SELECT unique")
	store.advance()
	expectRead(mock, "SELECT unique", 1)
	rows, err := cached.Query(ctx, "SELECT unique", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	// A write racing the open read invalidates the entry it is about to fill.
	store.advance()
	executeWrite(t, cached, mock, writeContext(t.Context(), 1))
	require.NoError(t, rows.Close())
	expectRead(mock, "SELECT unique", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT unique"))
}

func TestMemoryValidationRejectsConcurrentWrite(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx, info := WithInfo(Cache(pointReadContext(t.Context())))
	expectRead(mock, "SELECT unique", 1)
	rows, err := cached.Query(ctx, "SELECT unique", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var identifier int64
	require.NoError(t, rows.Scan(&identifier))
	require.False(t, rows.Next())
	executeWrite(t, cached, mock, writeContext(t.Context(), 1))
	require.NoError(t, rows.Close())
	expectRead(mock, "SELECT unique", 1)
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT unique"))
	require.False(t, info.Hit)
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT unique"))
	require.True(t, info.Hit)
}

func TestInvalidEntryDoesNotBackfill(t *testing.T) {
	store := newClockGenerations()
	front := newTrackingLevel("front")
	back := newTrackingLevel("back")
	cached, mock := newTestDriver(t, Generations(store), Levels(front, back))
	ctx := Cache(pointReadContext(t.Context()))
	expectRead(mock, "SELECT unique", 1)
	readIDs(t, cached, ctx, "SELECT unique")
	store.advance()
	front.clear()
	executeWrite(t, cached, mock, writeContext(t.Context(), 1))
	expectRead(mock, "SELECT unique", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, Cache(ctx, NoFill()), "SELECT unique"))
	require.Empty(t, front.stored())
}

func TestDriverSkewMargin(t *testing.T) {
	for _, test := range []struct {
		name   string
		margin time.Duration
		hit    bool
	}{
		{name: "margin below the write distance", margin: 10 * time.Millisecond, hit: true},
		{name: "margin above the write distance", margin: 2 * time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newClockGenerations()
			cached, mock := newTestDriver(t, Generations(store), SkewMargin(test.margin), Levels(newWireLevel()))
			ctx, info := WithInfo(Cache(pointReadContext(t.Context())))
			expectRead(mock, "SELECT unique", 1)
			readIDs(t, cached, ctx, "SELECT unique")
			// The second read initializes the validation token of the returned row.
			expectRead(mock, "SELECT unique", 1)
			readIDs(t, cached, ctx, "SELECT unique")
			// The last fill starts one second after that token, so the margin alone
			// decides whether the entry may be trusted.
			store.advance()
			expectRead(mock, "SELECT unique", 1)
			readIDs(t, cached, ctx, "SELECT unique")
			if !test.hit {
				expectRead(mock, "SELECT unique", 1)
			}
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT unique"))
			require.Equal(t, test.hit, info.Hit)
		})
	}
}
