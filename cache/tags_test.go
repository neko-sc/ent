// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/hex"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

func rowTag(t *testing.T, table string, id any) string {
	t.Helper()
	encoded, err := encodeKeyValue(nil, id)
	require.NoError(t, err)
	return table + "#" + hex.EncodeToString(encoded)
}

func TestKeyTags(t *testing.T) {
	list := &dialect.Statement{Tables: []string{"public.users", "groups", "groups"}}
	point := &dialect.Statement{Tables: []string{"public.users", "groups"}, PointRead: true}
	pointIDs := &dialect.Statement{Tables: []string{"public.users", "groups"}, PointRead: true, RequestedIDs: []any{1, 2}}

	for _, test := range []struct {
		name          string
		statement     *dialect.Statement
		options       queryOptions
		ttlOnlyTables set
		want          []string
	}{
		{
			name:      "list read tags every table once",
			statement: list,
			options:   queryOptions{tags: []string{"feed", "feed"}},
			want:      []string{"*", "groups", "public.users", "~feed"},
		},
		{
			name:      "point read with identifiers tags each row",
			statement: pointIDs,
			want:      []string{"*", "groups", "public.users!bulk", rowTag(t, "public.users", 1), rowTag(t, "public.users", 2)},
		},
		{
			name:      "point read without identifiers tags bulk writes only",
			statement: point,
			want:      []string{"*", "groups", "public.users!bulk"},
		},
		{
			name:    "a statement-less query tags the wildcard",
			options: queryOptions{tags: []string{"feed"}},
			want:    []string{"*", "~feed"},
		},
		{
			name:      "a statement without tables tags the wildcard",
			statement: &dialect.Statement{PointRead: true},
			want:      []string{"*"},
		},
		{
			name:      "declared dependencies widen a point read",
			statement: point,
			options:   queryOptions{dependsOn: []string{"public.users"}},
			want:      []string{"*", "groups", "public.users", "public.users!bulk"},
		},
		{
			name:          "ttl-only tables are never tagged",
			statement:     point,
			ttlOnlyTables: set{"public.users": {}},
			want:          []string{"*", "groups"},
		},
		{
			name:      "a ttl-only query keeps only user tags",
			statement: point,
			options:   queryOptions{ttlOnly: true, tags: []string{"feed"}, dependsOn: []string{"other"}},
			want:      []string{"*", "~feed"},
		},
		{
			name:      "an unencodable identifier falls back to the whole table",
			statement: &dialect.Statement{Tables: []string{"public.users", "groups"}, PointRead: true, RequestedIDs: []any{make(chan int)}},
			want:      []string{"*", "groups", "public.users", "public.users!bulk"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, keyTags(test.statement, test.options, test.ttlOnlyTables, nil))
		})
	}
}

func TestValidationTags(t *testing.T) {
	point := &dialect.Statement{Tables: []string{"users"}, PointRead: true}

	for _, test := range []struct {
		name          string
		statement     *dialect.Statement
		ids           []any
		ttlOnlyTables set
		want          []string
	}{
		{
			name:      "recorded identifiers become row tags",
			statement: point,
			ids:       []any{1, int64(1)},
			want:      []string{rowTag(t, "users", 1)},
		},
		{
			name:      "an empty result validates against the whole table",
			statement: point,
			want:      []string{"users"},
		},
		{
			name:      "an unencodable identifier validates against the whole table",
			statement: point,
			ids:       []any{make(chan int)},
			want:      []string{"users"},
		},
		{
			name:          "ttl-only tables are never validated",
			statement:     point,
			ids:           []any{1},
			ttlOnlyTables: set{"users": {}},
		},
		{
			name:      "requested identifiers are already part of the key",
			statement: &dialect.Statement{Tables: []string{"users"}, PointRead: true, RequestedIDs: []any{1}},
			ids:       []any{1},
		},
		{
			name:      "list reads are not validated",
			statement: &dialect.Statement{Tables: []string{"users"}},
			ids:       []any{1},
		},
		{
			name: "a statement-less query is not validated",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, validationTags(test.statement, test.ids, test.ttlOnlyTables))
		})
	}
}

func TestWriteTags(t *testing.T) {
	for _, test := range []struct {
		name      string
		statement *dialect.Statement
		touches   []string
		want      []string
	}{
		{
			name: "a statement tags its rows and bulk tables",
			statement: &dialect.Statement{
				Tables: []string{"users", "groups", "users"},
				Rows:   []dialect.RowRef{{Table: "users", ID: 1}, {Table: "users", ID: int64(1)}},
				Bulk:   []string{"groups", "groups"},
			},
			want: []string{"groups", "groups!bulk", "users", rowTag(t, "users", 1)},
		},
		{
			name: "declared touches widen statement invalidation",
			statement: &dialect.Statement{
				Tables: []string{"users", "groups"},
				Rows:   []dialect.RowRef{{Table: "users", ID: 1}},
				Bulk:   []string{"groups"},
			},
			touches: []string{"users", "groups", "pets", "pets"},
			want:    []string{"groups", "groups!bulk", "pets", "pets!bulk", "users", "users!bulk", rowTag(t, "users", 1)},
		},
		{
			name:      "declared touches apply to an empty statement",
			statement: &dialect.Statement{},
			touches:   []string{"users"},
			want:      []string{"users", "users!bulk"},
		},
		{
			name: "an unknown write invalidates everything",
			want: []string{"*"},
		},
		{
			name:    "declared touches invalidate whole tables",
			touches: []string{"users", "groups", "users"},
			want:    []string{"groups", "groups!bulk", "users", "users!bulk"},
		},
		{
			name: "an unencodable row identifier invalidates the whole table",
			statement: &dialect.Statement{
				Tables: []string{"users"},
				Rows:   []dialect.RowRef{{Table: "users", ID: make(chan int)}},
			},
			want: []string{"users", "users!bulk"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, writeTags(test.statement, test.touches))
		})
	}
}
