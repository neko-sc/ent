// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

type failingGenerations struct {
	GenerationStore
	nowError  error
	loadError error
	bumpError error
}

func (g *failingGenerations) Now(ctx context.Context) (uint64, error) {
	if g.nowError != nil {
		return 0, g.nowError
	}
	return g.GenerationStore.Now(ctx)
}

func (g *failingGenerations) Load(ctx context.Context, tags []string) ([]uint64, error) {
	if g.loadError != nil {
		return nil, g.loadError
	}
	return g.GenerationStore.Load(ctx, tags)
}

func (g *failingGenerations) Bump(ctx context.Context, tags []string) error {
	if g.bumpError != nil {
		return g.bumpError
	}
	return g.GenerationStore.Bump(ctx, tags)
}

type failingLevel struct {
	Level
	getError error
	setError error
}

func (l *failingLevel) Get(ctx context.Context, key Key) (*Entry, error) {
	if l.getError != nil {
		return nil, l.getError
	}
	return l.Level.Get(ctx, key)
}

func (l *failingLevel) Set(ctx context.Context, key Key, entry *Entry, ttl time.Duration) error {
	if l.setError != nil {
		return l.setError
	}
	return l.Level.Set(ctx, key, entry, ttl)
}

// validationFailureGenerations fails only the single-tag loads used to validate
// point reads, once failing is enabled.
type validationFailureGenerations struct {
	GenerationStore
	failing atomic.Bool
}

func (g *validationFailureGenerations) Load(ctx context.Context, tags []string) ([]uint64, error) {
	if g.failing.Load() && len(tags) == 1 {
		return nil, errors.New("validation")
	}
	return g.GenerationStore.Load(ctx, tags)
}

func TestDriverGenerationFailures(t *testing.T) {
	for _, test := range []struct {
		name  string
		store *failingGenerations
	}{
		{name: "now", store: &failingGenerations{GenerationStore: NewMemoryGenerations(), nowError: errors.New("now")}},
		{name: "load", store: &failingGenerations{GenerationStore: NewMemoryGenerations(), loadError: errors.New("load")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var reported error
			cached, mock := newTestDriver(t, Generations(test.store), OnError(func(_ context.Context, err error) { reported = err }))
			ctx, info := WithInfo(Cache(readContext(t.Context())))
			expectRead(mock, "SELECT id", 1)
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			require.Error(t, reported)
			require.Equal(t, BypassGeneration, info.Bypass)
			require.Zero(t, cached.Stats().Snapshot().Fills)
		})
	}
}

func TestDriverLevelFailures(t *testing.T) {
	var reported []error
	failing := &failingLevel{getError: errors.New("get"), setError: errors.New("set")}
	cached, mock := newTestDriver(t,
		Levels(failing, newMemoryLevel(t)),
		OnError(func(_ context.Context, err error) { reported = append(reported, err) }),
	)
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	require.Len(t, reported, 2)
	readIDs(t, cached, ctx, "SELECT id")
	require.Len(t, reported, 4)
	require.True(t, info.Hit)
	require.Equal(t, "memory", info.Level)
}

func TestDriverBumpFailure(t *testing.T) {
	var reported error
	failure := errors.New("bump")
	store := &failingGenerations{GenerationStore: NewMemoryGenerations(), bumpError: failure}
	cached, mock := newTestDriver(t, Generations(store), OnError(func(_ context.Context, err error) { reported = err }))
	executeWrite(t, cached, mock, writeContext(t.Context(), 1))
	require.ErrorIs(t, reported, failure)
	require.ErrorIs(t, cached.Invalidate(t.Context(), All()), failure)
	require.ErrorIs(t, NewInvalidator(store).Invalidate(t.Context(), All()), failure)
}

func TestDriverUnsupportedKey(t *testing.T) {
	var reported error
	underlying := &gatedDriver{entered: make(chan struct{}, 1), gate: make(chan struct{})}
	close(underlying.gate)
	cached, err := New(underlying, Levels(ContextLevel()), OnError(func(_ context.Context, err error) { reported = err }))
	require.NoError(t, err)
	ctx, info := WithInfo(Cache(readContext(t.Context())))
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id", make(chan int)))
	require.Error(t, reported)
	require.Equal(t, BypassKey, info.Bypass)
	require.Zero(t, cached.Stats().Snapshot().Fills)
}

func TestDriverWriteFailureDoesNotInvalidate(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx := Cache(readContext(t.Context()))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	mock.ExpectExec("UPDATE users").WillReturnError(errors.New("write"))
	_, err := cached.Exec(writeContext(t.Context()), "UPDATE users", nil)
	require.Error(t, err)
	mock.ExpectQuery("UPDATE users RETURNING id").WillReturnError(errors.New("write"))
	_, err = cached.Query(writeContext(t.Context()), "UPDATE users RETURNING id", nil)
	require.Error(t, err)
	readIDs(t, cached, ctx, "SELECT id")
	require.Empty(t, cached.Stats().Snapshot().Bumps)
}

func TestDriverValidationLoadFailure(t *testing.T) {
	var reported error
	store := &validationFailureGenerations{GenerationStore: newClockGenerations()}
	cached, mock := newTestDriver(t, Generations(store), OnError(func(_ context.Context, err error) { reported = err }))
	statement := &dialect.Statement{Kind: dialect.StatementRead, PointRead: true, IDColumn: "id", Tables: []string{"users"}}
	ctx, info := WithInfo(Cache(dialect.WithStatement(t.Context(), statement)))
	expectRead(mock, "SELECT unique", 1)
	readIDs(t, cached, ctx, "SELECT unique")
	store.failing.Store(true)
	expectRead(mock, "SELECT unique", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT unique"))
	require.Equal(t, BypassGeneration, info.Bypass)
	require.ErrorContains(t, reported, "validation")
}
