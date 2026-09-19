// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyDeterminism(t *testing.T) {
	const query = "SELECT id FROM users WHERE id = $1"
	key := func(t *testing.T, arguments []any, tags []string, tokens []uint64) Key {
		t.Helper()
		var builder keyBuilder
		require.NoError(t, builder.query("postgres", query, arguments))
		builder.tags(tags, tokens)
		return builder.sum()
	}
	original := key(t, []any{1}, []string{"users", "*"}, []uint64{2, 3})

	t.Run("keys are fixed width", func(t *testing.T) {
		require.Len(t, original.String(), 32)
	})

	t.Run("equal inputs produce one key", func(t *testing.T) {
		require.Equal(t, original, key(t, []any{int64(1)}, []string{"*", "users"}, []uint64{3, 2}))
	})

	t.Run("different arguments produce different keys", func(t *testing.T) {
		require.NotEqual(t, original, key(t, []any{2}, []string{"*", "users"}, []uint64{3, 2}))
	})

	t.Run("different tokens produce different keys", func(t *testing.T) {
		require.NotEqual(t, original, key(t, []any{1}, []string{"*", "users"}, []uint64{3, 4}))
	})

	t.Run("unsupported arguments fail", func(t *testing.T) {
		var builder keyBuilder
		require.Error(t, builder.query("postgres", "SELECT ?", []any{func() {}}))
	})

	t.Run("duplicate tags order their tokens", func(t *testing.T) {
		require.Equal(t,
			key(t, nil, []string{"users", "users"}, []uint64{1, 2}),
			key(t, nil, []string{"users", "users"}, []uint64{2, 1}),
		)
	})
}

func TestKeySessionVariables(t *testing.T) {
	keys := make(map[Key]bool)
	for _, variables := range [][][2]string{
		nil,
		{{"tenant", "first"}},
		{{"tenant", "second"}},
		{{"role", "first"}},
		{{"tenant", "first"}, {"tenant", "second"}},
		{{"tenant", "second"}, {"tenant", "first"}},
		{{"ab", "c"}},
		{{"a", "bc"}},
	} {
		sum := func(t *testing.T) Key {
			t.Helper()
			var builder keyBuilder
			require.NoError(t, builder.query("postgres", "SELECT id", nil))
			builder.variables(variables)
			builder.tags([]string{"*"}, []uint64{1})
			return builder.sum()
		}
		key := sum(t)
		require.False(t, keys[key], "session variables %v collide with an earlier set", variables)
		keys[key] = true
		require.Equal(t, key, sum(t), "the same session variables must produce the same key")
	}
}

func TestKeyArgBoundaryCollision(t *testing.T) {
	sum := func(t *testing.T, dialect, query string, arguments []any) Key {
		t.Helper()
		var builder keyBuilder
		require.NoError(t, builder.query(dialect, query, arguments))
		return builder.sum()
	}
	original := sum(t, "ab", "c", []any{"ab", "c"})

	t.Run("dialect and query are framed apart", func(t *testing.T) {
		require.NotEqual(t, original, sum(t, "a", "bc", []any{"ab", "c"}))
	})

	t.Run("arguments are framed apart", func(t *testing.T) {
		require.NotEqual(t, original, sum(t, "ab", "c", []any{"a", "bc"}))
	})
}
