// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package entc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/neko-sc/ent/entc/gen"
	"github.com/neko-sc/ent/entc/internal"

	"github.com/stretchr/testify/require"
)

func TestMayRecover_StaleSnapshot(t *testing.T) {
	target := t.TempDir()
	internalDirectory := filepath.Join(target, "internal")
	require.NoError(t, os.Mkdir(internalDirectory, 0o755))
	snapshot := filepath.Join(internalDirectory, "schema.go")
	sibling := filepath.Join(internalDirectory, "keep.go")
	require.NoError(t, os.WriteFile(snapshot, []byte("snapshot"), 0o600))
	require.NoError(t, os.WriteFile(sibling, []byte("keep"), 0o600))

	err := mayRecover(fmt.Errorf("load schema: %w", internal.ErrStaleSnapshot), t.TempDir(), &gen.Config{
		Target:   target,
		Features: []gen.Feature{gen.FeatureSnapshot},
	})
	require.NoError(t, err)
	_, err = os.Stat(snapshot)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(sibling)
	require.NoError(t, err)
}

func TestMayRecover_StaleSnapshotFeatureDisabled(t *testing.T) {
	stale := fmt.Errorf("load schema: %w", internal.ErrStaleSnapshot)
	err := mayRecover(stale, t.TempDir(), &gen.Config{Target: t.TempDir()})
	require.ErrorIs(t, err, stale)
}

func TestMayRecover_UnrelatedError(t *testing.T) {
	unrelated := errors.New("unrelated error")
	err := mayRecover(unrelated, t.TempDir(), &gen.Config{
		Target:   t.TempDir(),
		Features: []gen.Feature{gen.FeatureSnapshot},
	})
	require.ErrorIs(t, err, unrelated)
}

func TestMayRecover_StaleSnapshotRemovalFailure(t *testing.T) {
	target := t.TempDir()
	snapshot := filepath.Join(target, "internal", "schema.go")
	require.NoError(t, os.MkdirAll(snapshot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(snapshot, "keep"), []byte("keep"), 0o600))

	err := mayRecover(fmt.Errorf("load schema: %w", internal.ErrStaleSnapshot), t.TempDir(), &gen.Config{
		Target:   target,
		Features: []gen.Feature{gen.FeatureSnapshot},
	})
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	require.Equal(t, snapshot, pathError.Path)
}
