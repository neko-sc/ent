// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

// Package redis provides a Redis cache level and server-clock generation store.
// Clients must use RESP3 with server-assisted client-side caching enabled.
// Generation invalidations propagate asynchronously through Redis tracking;
// WithGenerationLocalTTL bounds cached reads if a notification is delayed.
// Row and user generation tags expire WithGenerationTTL after their last write
// or first load, while global and table tags stay persistent.
package redis

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neko-sc/ent/cache"
	"github.com/redis/rueidis"
)

const operationTimeout = 2 * time.Second

// Level shares its client with the caller; Close never closes that client.
// A Level must not be copied after construction.
type Level struct {
	client             rueidis.Client
	entryPrefix        string
	generationPrefix   string
	fillConcurrency    int
	generationLocalTTL time.Duration
	generationTTL      time.Duration
	generationExpiry   []string
	timeSync           time.Duration
	fills              chan struct{}
	dropped            atomic.Uint64
	mutex              sync.Mutex
	closed             bool
	fillError          error
	workers            sync.WaitGroup
	closeOnce          sync.Once
	closeError         error
	stop               context.CancelFunc
	done               chan struct{}
	origin             time.Time
	clock              atomic.Pointer[clockState]
	clockMutex         sync.Mutex
}

var (
	_ cache.Level           = (*Level)(nil)
	_ cache.GenerationStore = (*Level)(nil)
)

type Option func(*Level)

// New uses prefix as the database namespace shared by cooperating processes.
// Invalid options or a nil client panic. No network request is made until use.
func New(client rueidis.Client, prefix string, options ...Option) *Level {
	level := &Level{
		client:             client,
		entryPrefix:        prefix + ":e:",
		generationPrefix:   prefix + ":g:",
		fillConcurrency:    64,
		generationLocalTTL: time.Minute,
		generationTTL:      time.Hour,
		timeSync:           30 * time.Second,
		origin:             time.Now(),
		done:               make(chan struct{}),
	}
	for _, option := range options {
		option(level)
	}

	if client == nil || level.fillConcurrency <= 0 || level.generationLocalTTL <= 0 ||
		level.generationTTL < time.Millisecond || level.timeSync <= 0 {
		panic("cache/redis: client, positive concurrency, cache TTL, time sync interval, and generation TTL of at least 1ms required")
	}

	level.generationExpiry = []string{"PX", strconv.FormatInt(level.generationTTL.Milliseconds(), 10)}
	level.fills = make(chan struct{}, level.fillConcurrency)
	ctx, cancel := context.WithCancel(context.Background())
	level.stop = cancel
	go level.refreshClock(ctx)
	return level
}

// WithFillConcurrency bounds outstanding asynchronous writes; excess fills drop.
func WithFillConcurrency(concurrency int) Option {
	return func(level *Level) { level.fillConcurrency = concurrency }
}

// WithGenerationLocalTTL bounds the lifetime of server-tracked generation reads.
func WithGenerationLocalTTL(duration time.Duration) Option {
	return func(level *Level) { level.generationLocalTTL = duration }
}

// WithGenerationTTL expires row and user generation tags, as reported by
// cache.Expirable, the duration after their last write or first load; later
// loads do not extend it. An expired tag restarts at the server time, so the
// duration must exceed any lead its tokens gained over the server clock.
func WithGenerationTTL(duration time.Duration) Option {
	return func(level *Level) { level.generationTTL = duration }
}

func WithTimeSync(duration time.Duration) Option {
	return func(level *Level) { level.timeSync = duration }
}

func (*Level) Name() string {
	return "redis"
}

// Dropped counts fills rejected because all asynchronous workers were occupied.
func (l *Level) Dropped() uint64 {
	return l.dropped.Load()
}

// Close rejects new fills, drains accepted fills, and stops clock synchronization.
// Asynchronous write failures cannot be returned by their already-completed Set;
// unreported failures are returned by the next Set or Close instead.
func (l *Level) Close() error {
	l.closeOnce.Do(func() {
		l.mutex.Lock()
		l.closed = true
		l.mutex.Unlock()
		l.stop()
		l.workers.Wait()
		<-l.done
		l.mutex.Lock()
		l.closeError = l.fillError
		l.mutex.Unlock()
	})
	return l.closeError
}
