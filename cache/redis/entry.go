// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neko-sc/ent/cache"
	"github.com/redis/rueidis"
)

func (l *Level) Get(ctx context.Context, key cache.Key) (*cache.Entry, error) {
	data, err := l.client.Do(ctx, l.client.B().Get().Key(l.entryPrefix+key.String()).Build()).AsBytes()
	if rueidis.IsRedisNil(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return cache.DecodeEntry(data)
}

// Set encodes before returning, but never waits for Redis or a free fill worker.
// Accepted writes survive request cancellation and have a two-second deadline.
// A previous asynchronous failure is returned without scheduling another fill.
func (l *Level) Set(ctx context.Context, key cache.Key, entry *cache.Entry, ttl time.Duration) error {
	if entry == nil || ttl <= 0 {
		return fmt.Errorf("cache/redis: entry and positive TTL required")
	}

	data, err := entry.Bytes()
	if err != nil {
		return err
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.closed {
		return rueidis.ErrClosing
	}
	if l.fillError != nil {
		err := l.fillError
		l.fillError = nil
		return err
	}

	select {
	case l.fills <- struct{}{}:
	default:
		l.dropped.Add(1)
		return nil
	}
	l.workers.Go(func() {
		defer func() { <-l.fills }()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), operationTimeout)
		defer cancel()
		// Redis only accepts integral positive milliseconds; submillisecond TTLs
		// must not become an invalid PX 0 or an accidentally persistent entry.
		if err := l.client.Do(ctx, l.client.B().Set().Key(l.entryPrefix+key.String()).Value(string(data)).PxMilliseconds(max(ttl.Milliseconds(), 1)).Build()).Error(); err != nil {
			l.mutex.Lock()
			l.fillError = errors.Join(l.fillError, err)
			l.mutex.Unlock()
		}
	})
	return nil
}

func (l *Level) Delete(ctx context.Context, keys ...cache.Key) error {
	commands := make([]rueidis.Completed, len(keys))
	for index, key := range keys {
		commands[index] = l.client.B().Del().Key(l.entryPrefix + key.String()).Build()
	}

	var failure error
	for _, result := range l.client.DoMulti(ctx, commands...) {
		failure = errors.Join(failure, result.Error())
	}

	return failure
}
