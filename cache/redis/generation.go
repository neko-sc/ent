// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/redis/rueidis"
)

var (
	//go:embed initialize_generation.lua
	initializeGenerationSource string
	//go:embed bump_generation.lua
	bumpGenerationSource string

	initializeGeneration = rueidis.NewLuaScript(initializeGenerationSource)
	bumpGeneration       = rueidis.NewLuaScript(bumpGenerationSource)
)

func (l *Level) Load(ctx context.Context, tags []string) ([]uint64, error) {
	commands := make([]rueidis.CacheableTTL, len(tags))
	for index, tag := range tags {
		commands[index] = rueidis.CT(l.client.B().Get().Key(l.generationPrefix+tag).Cache(), l.generationLocalTTL)
	}

	tokens := make([]uint64, len(tags))
	var missing []rueidis.LuaExec
	var indices []int
	for index, result := range l.client.DoMultiCache(ctx, commands...) {
		token, err := result.AsUint64()
		if rueidis.IsRedisNil(err) {
			missing = append(missing, rueidis.LuaExec{Keys: []string{l.generationPrefix + tags[index]}})
			indices = append(indices, index)
			continue
		}
		if err != nil {
			return nil, err
		}
		if token == 0 {
			return nil, fmt.Errorf("cache/redis: zero generation for %q", tags[index])
		}
		tokens[index] = token
	}
	if len(missing) == 0 {
		return tokens, nil
	}

	// Tags of one query can hash to different cluster slots. Each script gets
	// exactly one key, while ExecMulti pipelines the independent executions.
	for index, result := range initializeGeneration.ExecMulti(ctx, l.client, missing...) {
		token, err := result.AsUint64()
		if err != nil {
			return nil, err
		}
		if token == 0 {
			return nil, fmt.Errorf("cache/redis: zero generation for %q", tags[indices[index]])
		}
		tokens[indices[index]] = token
	}

	return tokens, nil
}

func (l *Level) Bump(ctx context.Context, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	executions := make([]rueidis.LuaExec, len(tags))
	for index, tag := range tags {
		executions[index].Keys = []string{l.generationPrefix + tag}
	}

	var failure error
	for _, result := range bumpGeneration.ExecMulti(ctx, l.client, executions...) {
		failure = errors.Join(failure, result.Error())
	}

	return failure
}
