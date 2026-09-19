// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInvalidateHitMissMatrix(t *testing.T) {
	for _, test := range []struct {
		name      string
		option    InvalidateOption
		pointMiss bool
		listMiss  bool
		tagMiss   bool
	}{
		{name: "table", option: Tables("users"), pointMiss: true, listMiss: true, tagMiss: true},
		{name: "row", option: Rows("users", 1), pointMiss: true, listMiss: true, tagMiss: true},
		{name: "other row", option: Rows("users", 2), listMiss: true, tagMiss: true},
		{name: "tag", option: Tags("feed"), tagMiss: true},
		{name: "all", option: All(), pointMiss: true, listMiss: true, tagMiss: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			cached, mock := newTestDriver(t)
			contexts := []context.Context{Cache(readContext(t.Context(), 1)), Cache(readContext(t.Context())), Cache(readContext(t.Context()), Tags("feed"))}
			for _, ctx := range contexts {
				expectRead(mock, "SELECT id", 1)
				readIDs(t, cached, ctx, "SELECT id")
			}
			require.NoError(t, cached.Invalidate(t.Context(), test.option))
			for index, miss := range []bool{test.pointMiss, test.listMiss, test.tagMiss} {
				if miss {
					expectRead(mock, "SELECT id", 2)
				}
				ctx, info := WithInfo(contexts[index])
				readIDs(t, cached, ctx, "SELECT id")
				require.Equal(t, !miss, info.Hit)
			}
		})
	}
}

func TestStandaloneInvalidator(t *testing.T) {
	store := NewMemoryGenerations()
	cached, mock := newTestDriver(t, Generations(store))
	ctx := Cache(readContext(t.Context(), 1), Tags("feed"))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	require.NoError(t, NewInvalidator(store).Invalidate(t.Context(), Tags("feed")))
	expectRead(mock, "SELECT id", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
	require.NoError(t, cached.Invalidate(t.Context()))
	require.NoError(t, NewInvalidator(store).Invalidate(t.Context()))
	require.Error(t, cached.Invalidate(t.Context(), Rows("users", make(chan int))))
	require.Error(t, NewInvalidator(store).Invalidate(t.Context(), Rows("users", make(chan int))))
	tags, err := invalidationTags([]InvalidateOption{Tables("users", "users"), Rows("users", 1), Tags("feed", "feed"), All()})
	require.NoError(t, err)
	require.Len(t, tags, 5)
}

func TestTouches(t *testing.T) {
	cached, mock := newTestDriver(t)
	ctx := Cache(readContext(t.Context()))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	executeWrite(t, cached, mock, Touches(t.Context(), "groups"))
	readIDs(t, cached, ctx, "SELECT id")
	executeWrite(t, cached, mock, Touches(t.Context(), "users"))
	expectRead(mock, "SELECT id", 2)
	require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
}
