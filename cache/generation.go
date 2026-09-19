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
	// Load returns one token per tag. Stores that can lose tags must initialize
	// missing tags with the current time to reject entries predating that loss.
	Load(ctx context.Context, tags []string) ([]uint64, error)
	// Bump sets each token to max(previous+1, current time).
	Bump(ctx context.Context, tags []string) error
}

type memoryGenerations struct {
	mutex  sync.Mutex
	tokens map[string]uint64
	clock  uint64
	origin uint64
}

func NewMemoryGenerations() GenerationStore {
	return &memoryGenerations{tokens: make(map[string]uint64), origin: uint64(max(time.Now().UnixMicro(), 0))}
}

func (g *memoryGenerations) Now(ctx context.Context) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.clock = max(g.clock+1, g.origin+1, uint64(max(time.Now().UnixMicro(), 0)))
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
		// Tags never expire, so an absent token means no writes since creation.
		tokens[index] = max(g.tokens[tag], g.origin)
	}

	return tokens, nil
}

func (g *memoryGenerations) Bump(ctx context.Context, tags []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	g.mutex.Lock()
	defer g.mutex.Unlock()
	g.clock = max(g.clock+1, g.origin+1, uint64(max(time.Now().UnixMicro(), 0)))
	for _, tag := range tags {
		g.tokens[tag] = max(g.tokens[tag]+1, g.clock)
		g.clock = max(g.clock, g.tokens[tag])
	}

	return nil
}
