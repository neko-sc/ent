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

type gatedDriver struct {
	dialect.Driver
	calls   atomic.Int64
	entered chan struct{}
	gate    chan struct{}
	failure error
}

func (d *gatedDriver) Dialect() dialect.Dialect {
	return dialect.SQLite
}

func (d *gatedDriver) Query(ctx context.Context, _ string, _ []any) (dialect.Rows, error) {
	d.calls.Add(1)
	select {
	case d.entered <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-d.gate:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if d.failure != nil {
		return nil, d.failure
	}
	return &fakeRows{columns: []string{"id"}, rows: [][]any{{int64(1)}}}, nil
}

// openGatedDriver answers every query immediately.
func openGatedDriver(capacity int) *gatedDriver {
	underlying := &gatedDriver{entered: make(chan struct{}, capacity), gate: make(chan struct{})}
	close(underlying.gate)
	return underlying
}

// waitingContext reports when a caller starts waiting on it.
type waitingContext struct {
	context.Context
	waiting chan struct{}
}

func (ctx *waitingContext) Done() <-chan struct{} {
	select {
	case ctx.waiting <- struct{}{}:
	default:
	}
	return ctx.Context.Done()
}

func TestSingleflightDeduplicatesReads(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			underlying := &gatedDriver{entered: make(chan struct{}, 2), gate: make(chan struct{})}
			cached, err := New(underlying, Levels(ContextLevel()), Singleflight(enabled))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(Cache(readContext(t.Context())), 5*time.Second)
			defer cancel()
			leader := make(chan dialect.Rows, 1)
			go func() {
				rows, err := cached.Query(ctx, "SELECT id", nil)
				if err != nil {
					t.Error(err)
				}
				leader <- rows
			}()
			<-underlying.entered
			follower := make(chan dialect.Rows, 1)
			waiting := &waitingContext{Context: ctx, waiting: make(chan struct{}, 1)}
			go func() {
				rows, err := cached.Query(waiting, "SELECT id", nil)
				if err != nil {
					t.Error(err)
				}
				follower <- rows
			}()
			require.Eventually(t, func() bool { return cached.Stats().Misses.Load() == 2 }, time.Second, time.Millisecond)
			if !enabled {
				<-underlying.entered
			}
			close(underlying.gate)
			rows := <-leader
			require.NotNil(t, rows)
			require.True(t, rows.Next())
			var id int64
			require.NoError(t, rows.Scan(&id))
			require.False(t, rows.Next())
			if enabled {
				select {
				case <-waiting.waiting:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			require.NoError(t, rows.Close())
			rows = <-follower
			require.NotNil(t, rows)
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(&id))
			require.Equal(t, int64(1), id)
			require.False(t, rows.Next())
			require.NoError(t, rows.Close())
			if enabled {
				require.Equal(t, int64(1), underlying.calls.Load(), "the follower must reuse the leader's result")
				require.Equal(t, uint64(1), cached.Stats().SingleflightShared.Load())
			} else {
				require.Equal(t, int64(2), underlying.calls.Load())
				require.Zero(t, cached.Stats().SingleflightShared.Load())
			}
		})
	}
}

func TestSingleflightWaiterCancellation(t *testing.T) {
	underlying := openGatedDriver(2)
	cached, err := New(underlying, Levels(ContextLevel()))
	require.NoError(t, err)
	ctx := Cache(readContext(t.Context()))
	rows, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	waiting, cancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() {
		_, err := cached.Query(waiting, "SELECT id", nil)
		result <- err
	}()
	require.Eventually(t, func() bool { return cached.Stats().Misses.Load() == 2 }, time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.Equal(t, int64(1), underlying.calls.Load(), "a canceled waiter must not query the database")
	require.NoError(t, rows.Close())
	// The abandoned wait must leave nothing behind that blocks later readers.
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.Equal(t, int64(2), underlying.calls.Load())
}

func TestSingleflightIncompleteLeaderRetries(t *testing.T) {
	underlying := openGatedDriver(2)
	cached, err := New(underlying, Levels(ContextLevel()))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(Cache(readContext(t.Context())), 5*time.Second)
	defer cancel()
	// The leader never scans its rows, so its result is not cacheable.
	rows, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	waiting := &waitingContext{Context: ctx, waiting: make(chan struct{}, 1)}
	follower := make(chan dialect.Rows, 1)
	go func() {
		rows, err := cached.Query(waiting, "SELECT id", nil)
		if err != nil {
			t.Error(err)
		}
		follower <- rows
	}()
	select {
	case <-waiting.waiting:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, rows.Close())
	rows = <-follower
	require.NotNil(t, rows)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	require.NoError(t, rows.Close())
	require.Equal(t, int64(2), underlying.calls.Load(), "an incomplete leader must not be shared")
	require.Zero(t, cached.Stats().SingleflightShared.Load())
}

func TestSingleflightLeakedLeaderWaitBound(t *testing.T) {
	underlying := openGatedDriver(3)
	cached, err := New(underlying, Levels(ContextLevel()), SingleflightWait(20*time.Millisecond))
	require.NoError(t, err)
	ctx := Cache(readContext(t.Context()))
	// The leader never closes its rows, so it never resolves its flight.
	leader, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	follower := make(chan dialect.Rows, 1)
	go func() {
		rows, err := cached.Query(ctx, "SELECT id", nil)
		if err != nil {
			t.Error(err)
		}
		follower <- rows
	}()
	select {
	case rows := <-follower:
		require.NotNil(t, rows)
		require.NoError(t, rows.Close())
	case <-time.After(5 * time.Second):
		t.Fatal("singleflight waiter did not escape a leaked leader")
	}
	require.Equal(t, int64(2), underlying.calls.Load())
	require.Zero(t, cached.Stats().SingleflightShared.Load())
	require.NoError(t, leader.Close())
}

func TestSingleflightAbandonedLeaderIsReplaced(t *testing.T) {
	underlying := openGatedDriver(4)
	cached, err := New(underlying, Levels(ContextLevel()), SingleflightWait(20*time.Millisecond))
	require.NoError(t, err)
	ctx := Cache(readContext(NewContext(t.Context())))
	abandoned, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	time.Sleep(40 * time.Millisecond)

	// The replacement leads its own flight rather than waiting on the stale one.
	replacement, err := cached.Query(ctx, "SELECT id", nil)
	require.NoError(t, err)
	require.Equal(t, int64(2), underlying.calls.Load())
	require.Zero(t, cached.Stats().SingleflightShared.Load())

	// Closing the abandoned leader must not unregister the replacement.
	require.NoError(t, abandoned.Close())
	require.True(t, replacement.Next())
	var id int64
	require.NoError(t, replacement.Scan(&id))
	require.False(t, replacement.Next())
	require.NoError(t, replacement.Close())

	ctx, info := WithInfo(ctx)
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.True(t, info.Hit)
	require.Equal(t, int64(2), underlying.calls.Load())
}

func TestSingleflightLeaderError(t *testing.T) {
	underlying := openGatedDriver(2)
	underlying.failure = errors.New("query")
	cached, err := New(underlying, Levels(ContextLevel()))
	require.NoError(t, err)
	ctx := Cache(readContext(t.Context()))
	_, err = cached.Query(ctx, "SELECT id", nil)
	require.ErrorContains(t, err, "query")
	// A failed leader must release its flight instead of stranding later readers.
	underlying.failure = nil
	require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
	require.Equal(t, int64(2), underlying.calls.Load())
}
