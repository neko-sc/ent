// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko-sc/ent/cache"
	"github.com/neko-sc/ent/cache/redis"
	"github.com/redis/rueidis"
	"github.com/stretchr/testify/require"
)

func newLevel(t *testing.T, client rueidis.Client, options ...redis.Option) (*redis.Level, string) {
	t.Helper()
	prefix := fmt.Sprintf("%s:%d", t.Name(), time.Now().UnixNano())
	level := redis.New(client, prefix, options...)
	t.Cleanup(func() { require.NoError(t, level.Close()) })
	return level, prefix
}

func TestRedisLevelGetSetDeleteExpiry(t *testing.T) {
	client := newClient(t)
	level, prefix := newLevel(t, client)
	require.Equal(t, "redis", level.Name())
	entry := newEntry(t)

	// eventualGet polls until the asynchronous fill makes the key observable (or
	// until it disappears again when expecting absence).
	eventualGet := func(t *testing.T, key cache.Key, present bool) *cache.Entry {
		t.Helper()
		var found *cache.Entry
		require.Eventually(t, func() bool {
			var err error
			found, err = level.Get(t.Context(), key)
			return err == nil && (found != nil) == present
		}, time.Second, time.Millisecond)
		return found
	}

	t.Run("missing key returns no entry", func(t *testing.T) {
		found, err := level.Get(t.Context(), cache.Key{9})
		require.NoError(t, err)
		require.Nil(t, found)
	})

	t.Run("round trip preserves the payload", func(t *testing.T) {
		key := cache.Key{1}
		require.NoError(t, level.Set(t.Context(), key, entry, time.Minute))
		found := eventualGet(t, key, true)
		want, err := entry.Bytes()
		require.NoError(t, err)
		actual, err := found.Bytes()
		require.NoError(t, err)
		require.Equal(t, want, actual)
		require.Equal(t, []string{"id"}, found.Columns())
		require.Equal(t, 1, found.Len())
	})

	t.Run("set applies the requested TTL", func(t *testing.T) {
		key := cache.Key{1}
		remaining, err := client.Do(t.Context(), client.B().Pttl().Key(prefix+":e:"+key.String()).Build()).AsInt64()
		require.NoError(t, err)
		require.Positive(t, remaining)
		require.LessOrEqual(t, remaining, time.Minute.Milliseconds())
	})

	t.Run("delete removes the entry", func(t *testing.T) {
		key := cache.Key{1}
		require.NoError(t, level.Delete(t.Context(), key, cache.Key{2}))
		found, err := level.Get(t.Context(), key)
		require.NoError(t, err)
		require.Nil(t, found)
	})

	t.Run("entries expire", func(t *testing.T) {
		key := cache.Key{3}
		require.NoError(t, level.Set(t.Context(), key, entry, 100*time.Millisecond))
		require.NotNil(t, eventualGet(t, key, true))
		require.Nil(t, eventualGet(t, key, false))
	})

	t.Run("corrupt payload reports an error", func(t *testing.T) {
		key := cache.Key{4}
		require.NoError(t, client.Do(t.Context(), client.B().Set().Key(prefix+":e:"+key.String()).Value("corrupt").Build()).Error())
		_, err := level.Get(t.Context(), key)
		require.Error(t, err)
	})
}

func TestRedisGenerationLoadBumpCache(t *testing.T) {
	client := newClient(t)
	level, prefix := newLevel(t, client)
	tags := []string{"users", "groups", "*"}
	initial, err := level.Load(t.Context(), tags)
	require.NoError(t, err)

	// settle orders assertions after the tracking invalidation pushes that
	// rueidis delivers asynchronously on its multiplexed connections.
	settle := func(t *testing.T, tags ...string) {
		t.Helper()
		for _, tag := range tags {
			require.NoError(t, client.Do(t.Context(), client.B().Get().Key(prefix+":g:"+tag).Build()).Error())
		}
	}

	t.Run("initial tokens are positive", func(t *testing.T) {
		for _, token := range initial {
			require.Positive(t, token)
		}
	})

	t.Run("repeated loads are stable", func(t *testing.T) {
		for range 2 {
			tokens, err := level.Load(t.Context(), tags)
			require.NoError(t, err)
			require.Equal(t, initial, tokens)
		}
	})

	t.Run("repeated loads are served client side", func(t *testing.T) {
		for _, tag := range tags {
			result := client.DoCache(t.Context(), client.B().Get().Key(prefix+":g:"+tag).Cache(), time.Minute)
			require.NoError(t, result.Error())
			require.True(t, result.IsCacheHit())
		}
	})

	t.Run("bump advances every tag", func(t *testing.T) {
		require.NoError(t, level.Bump(t.Context(), tags))
		settle(t, tags...)
		tokens, err := level.Load(t.Context(), tags)
		require.NoError(t, err)
		for index := range tags {
			require.Greater(t, tokens[index], initial[index])
		}
	})

	t.Run("bump increments a token that is already ahead of the clock", func(t *testing.T) {
		tokens, err := level.Load(t.Context(), []string{"users"})
		require.NoError(t, err)
		future := tokens[0] + 60_000_000
		require.NoError(t, client.Do(t.Context(), client.B().Set().Key(prefix+":g:users").Value(fmt.Sprint(future)).Build()).Error())
		require.NoError(t, level.Bump(t.Context(), []string{"users"}))
		settle(t, "users")
		tokens, err = level.Load(t.Context(), []string{"users"})
		require.NoError(t, err)
		require.Equal(t, future+1, tokens[0])
	})

	t.Run("empty tag lists are no-ops", func(t *testing.T) {
		empty, err := level.Load(t.Context(), nil)
		require.NoError(t, err)
		require.Empty(t, empty)
		require.NoError(t, level.Bump(t.Context(), nil))
		require.NoError(t, level.Delete(t.Context()))
	})
}

func TestGenerationsAcrossClients(t *testing.T) {
	first, prefix := newLevel(t, newClient(t))
	secondClient := newClient(t)
	second := redis.New(secondClient, prefix)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	initial, err := first.Load(t.Context(), []string{"users", "groups"})
	require.NoError(t, err)
	tokens, err := second.Load(t.Context(), []string{"users", "groups"})
	require.NoError(t, err)
	require.Equal(t, initial, tokens)
	require.NoError(t, first.Bump(t.Context(), []string{"users"}))
	require.NoError(t, secondClient.Do(t.Context(), secondClient.B().Get().Key(prefix+":g:users").Build()).Error())
	tokens, err = second.Load(t.Context(), []string{"users", "groups"})
	require.NoError(t, err)
	require.Greater(t, tokens[0], initial[0])
	require.Equal(t, initial[1], tokens[1])
}

type countedClient struct {
	rueidis.Client
	calls atomic.Uint64
}

func (c *countedClient) Do(ctx context.Context, command rueidis.Completed) rueidis.RedisResult {
	c.calls.Add(1)
	return c.Client.Do(ctx, command)
}

func TestServerClock(t *testing.T) {
	client := &countedClient{Client: newClient(t)}
	level, _ := newLevel(t, client, redis.WithTimeSync(20*time.Millisecond))
	require.Zero(t, client.calls.Load())
	now, err := level.Now(t.Context())
	require.NoError(t, err)
	require.Equal(t, uint64(1), client.calls.Load())
	for range 10 {
		next, err := level.Now(t.Context())
		require.NoError(t, err)
		require.GreaterOrEqual(t, next, now)
	}
	server, err := client.Client.Do(t.Context(), client.B().Time().Build()).AsIntSlice()
	require.NoError(t, err)
	require.InDelta(t, server[0]*1_000_000+server[1], now, 1_000_000)
	require.Eventually(t, func() bool { return client.calls.Load() > 1 }, time.Second, time.Millisecond)
	require.NoError(t, level.Close())
	_, err = level.Now(t.Context())
	require.ErrorIs(t, err, rueidis.ErrClosing)
	require.NoError(t, client.Client.Do(t.Context(), client.B().Ping().Build()).Error())
}

type blockedClient struct {
	rueidis.Client
	started chan struct{}
	release chan struct{}
	failure error
}

func (c *blockedClient) Do(ctx context.Context, command rueidis.Completed) rueidis.RedisResult {
	if command.Commands()[0] != "SET" {
		return c.Client.Do(ctx, command)
	}
	close(c.started)
	select {
	case <-c.release:
	case <-ctx.Done():
		return rueidis.NewErrorResult(ctx.Err())
	}
	if c.failure != nil {
		return rueidis.NewErrorResult(c.failure)
	}
	return c.Client.Do(ctx, command)
}

func TestAsyncFillSaturationAndClose(t *testing.T) {
	client := &blockedClient{Client: newClient(t), started: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	level := redis.New(client, fmt.Sprintf("%s:%d", t.Name(), time.Now().UnixNano()), redis.WithFillConcurrency(1))
	t.Cleanup(func() {
		release.Do(func() { close(client.release) })
		require.NoError(t, level.Close())
	})
	entry := newEntry(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.NoError(t, level.Set(ctx, cache.Key{1}, entry, time.Minute))
	select {
	case <-client.started:
	case <-time.After(time.Second):
		t.Fatal("asynchronous fill did not start")
	}
	returned := make(chan error, 1)
	go func() { returned <- level.Set(t.Context(), cache.Key{2}, entry, time.Minute) }()
	select {
	case err := <-returned:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("saturated Set blocked")
	}
	require.Equal(t, uint64(1), level.Dropped())
	closed := make(chan error, 1)
	go func() { closed <- level.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before draining: %v", err)
	default:
	}
	release.Do(func() { close(client.release) })
	require.NoError(t, <-closed)
	found, err := level.Get(t.Context(), cache.Key{1})
	require.NoError(t, err)
	require.NotNil(t, found)
	require.ErrorIs(t, level.Set(t.Context(), cache.Key{3}, entry, time.Minute), rueidis.ErrClosing)
	require.NoError(t, level.Close())
}

func TestAsyncFailureReturnedByClose(t *testing.T) {
	failure := errors.New("write failed")
	client := &blockedClient{Client: newClient(t), started: make(chan struct{}), release: make(chan struct{}), failure: failure}
	close(client.release)
	level := redis.New(client, fmt.Sprintf("%s:%d", t.Name(), time.Now().UnixNano()))
	require.NoError(t, level.Set(t.Context(), cache.Key{1}, newEntry(t), time.Minute))
	require.ErrorIs(t, level.Close(), failure)
}
