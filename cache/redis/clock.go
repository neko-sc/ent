// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/rueidis"
)

type clockState struct {
	offset int64
	err    error
}

// Now estimates Redis time in microseconds using Go's monotonic clock between
// synchronizations. Fill-start timestamps and generation tokens must share the
// server clock because entry validation directly compares the two.
func (l *Level) Now(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	select {
	case <-l.done:
		return 0, rueidis.ErrClosing
	default:
	}
	state := l.clock.Load()
	if state == nil {
		l.clockMutex.Lock()
		state = l.clock.Load()
		if state == nil {
			state = l.synchronizeClock(ctx)
			// A canceled first request must not poison future callers.
			if state.err == nil {
				l.clock.Store(state)
			}
		}
		l.clockMutex.Unlock()
	}
	if state.err != nil {
		return 0, state.err
	}
	return uint64(max(time.Since(l.origin).Microseconds()+state.offset, 0)), nil
}

func (l *Level) synchronizeClock(ctx context.Context) *clockState {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	started := time.Since(l.origin).Microseconds()
	values, err := l.client.Do(ctx, l.client.B().Time().Build()).AsIntSlice()
	if err != nil {
		return &clockState{err: err}
	}
	if len(values) != 2 || values[0] <= 0 || values[0] > (math.MaxInt64-999999)/1000000 || values[1] < 0 || values[1] >= 1000000 {
		return &clockState{err: fmt.Errorf("cache/redis: invalid TIME response %v", values)}
	}
	// The midpoint removes half of the round-trip delay from the estimate.

	return &clockState{offset: values[0]*1000000 + values[1] - (started+time.Since(l.origin).Microseconds())/2}
}

func (l *Level) refreshClock(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.timeSync)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if l.clock.Load() == nil {
				continue
			}
			l.clockMutex.Lock()
			// Surface synchronization failures from Now rather than silently
			// serving an increasingly inaccurate clock during an outage.
			l.clock.Store(l.synchronizeClock(ctx))
			l.clockMutex.Unlock()
		}
	}
}
