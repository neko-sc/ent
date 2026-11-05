// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"sync"
	"time"
)

type GenerationStore interface {
	// Now supplies the same microsecond clock used by Load and Bump.
	Now(ctx context.Context) (uint64, error)
	// Load returns one token per tag. Stores that can lose tags, typically by
	// expiring those reported by Expirable, must initialize missing tags with
	// the current time to reject entries predating that loss.
	Load(ctx context.Context, tags []string) ([]uint64, error)
	// Bump sets each token to max(previous+1, current time).
	Bump(ctx context.Context, tags []string) error
}

type memoryGenerations struct {
	mutex    sync.Mutex
	maxTags  int
	bounded  map[string]uint64
	current  map[string]uint64
	previous map[string]uint64
	floor    uint64
	clock    uint64
	origin   uint64
}

// NewMemoryGenerations retains at most 2*maxTags recently used row and user
// tags; global and table tags are never dropped. Nonpositive bounds panic.
func NewMemoryGenerations(maxTags int) GenerationStore {
	if maxTags <= 0 {
		panic("cache: memory generation limit must be positive")
	}

	origin := uint64(max(time.Now().UnixMicro(), 0))
	return &memoryGenerations{
		maxTags:  maxTags,
		bounded:  make(map[string]uint64),
		current:  make(map[string]uint64),
		previous: make(map[string]uint64),
		floor:    origin,
		clock:    origin,
		origin:   origin,
	}
}

func (g *memoryGenerations) Now(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.clock = max(g.clock+1, uint64(max(time.Now().UnixMicro(), 0)))
	return g.clock, nil
}

func (g *memoryGenerations) Load(ctx context.Context, tags []string) ([]uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()
	tokens := make([]uint64, len(tags))

	for index, tag := range tags {
		if !Expirable(tag) {
			// Bounded tags never expire, so an absent token means no writes since creation.
			tokens[index] = max(g.bounded[tag], g.origin)
			continue
		}
		token, recent := g.token(tag)
		if !recent {
			g.retain(tag, token)
		}
		tokens[index] = token
	}

	return tokens, nil
}

func (g *memoryGenerations) Bump(ctx context.Context, tags []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.clock = max(g.clock+1, uint64(max(time.Now().UnixMicro(), 0)))
	for _, tag := range tags {
		if !Expirable(tag) {
			g.clock = max(g.bounded[tag]+1, g.clock)
			g.bounded[tag] = g.clock
			continue
		}
		token, _ := g.token(tag)
		g.clock = max(token+1, g.clock)
		g.retain(tag, g.clock)
	}

	return nil
}

func (g *memoryGenerations) token(tag string) (token uint64, recent bool) {
	if token, found := g.current[tag]; found {
		return token, true
	}
	if token, found := g.previous[tag]; found {
		return token, false
	}
	return g.floor, false
}

func (g *memoryGenerations) retain(tag string, token uint64) {
	g.current[tag] = token
	if len(g.current) >= g.maxTags {
		// The clock is at least every issued token, so tags dropped with the
		// previous map resume at or above their last token.
		g.floor = g.clock
		clear(g.previous)
		g.previous, g.current = g.current, g.previous
	}
}
