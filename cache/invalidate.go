// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"encoding/hex"
	"slices"
)

type InvalidateOption interface {
	applyInvalidation([]string) ([]string, error)
}

type invalidateOption func([]string) ([]string, error)

func (option invalidateOption) applyInvalidation(tags []string) ([]string, error) {
	return option(tags)
}

// Tables invalidates both list and point reads of the named tables.
func Tables(tables ...string) InvalidateOption {
	tables = slices.Clone(tables)
	return invalidateOption(func(tags []string) ([]string, error) {
		for _, table := range tables {
			tags = append(tags, table, table+"!bulk")
		}
		return tags, nil
	})
}

// Rows invalidates the named rows and table-wide reads that may include them.
func Rows(table string, ids ...any) InvalidateOption {
	ids = slices.Clone(ids)
	return invalidateOption(func(tags []string) ([]string, error) {
		tags = append(tags, table)
		for _, id := range ids {
			encoded, err := encodeKeyValue(nil, id)
			if err != nil {
				return nil, err
			}
			tags = append(tags, table+"#"+hex.EncodeToString(encoded))
		}
		return tags, nil
	})
}

func All() InvalidateOption {
	return invalidateOption(func(tags []string) ([]string, error) { return append(tags, "*"), nil })
}

func (d *Driver) Invalidate(ctx context.Context, options ...InvalidateOption) error {
	tags, err := invalidationTags(options)
	if err != nil {
		return err
	}
	return d.bump(ctx, tags)
}

type Invalidator struct {
	generations GenerationStore
}

func NewInvalidator(generations GenerationStore) *Invalidator {
	return &Invalidator{generations: generations}
}

func (i *Invalidator) Invalidate(ctx context.Context, options ...InvalidateOption) error {
	tags, err := invalidationTags(options)
	if err != nil {
		return err
	}
	if len(tags) == 0 {
		return nil
	}
	return i.generations.Bump(ctx, tags)
}

func invalidationTags(options []InvalidateOption) ([]string, error) {
	var tags []string

	for _, option := range options {
		var err error
		tags, err = option.applyInvalidation(tags)
		if err != nil {
			return nil, err
		}
	}

	slices.Sort(tags)
	return slices.Compact(tags), nil
}
