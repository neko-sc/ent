// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheckDir(t *testing.T) {
	target := filepath.Join(os.TempDir(), "entvcs")
	require.NoError(t, os.MkdirAll(target, os.ModePerm), "creating tmpdir")
	defer os.RemoveAll(target)
	err := os.WriteFile(filepath.Join(target, "a.go"), []byte(`package schema`), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(target, "b.go"), []byte(`package schema

type User struct {
<<<<<<< local
        ent.Schema
=======
        schema
>>>>>>> other
}
`), 0644)
	require.NoError(t, err)
	err = CheckDir(target)
	require.Error(t, err)
	expected := fmt.Sprintf("vcs conflict %s:4", filepath.Join(target, "b.go"))
	require.EqualError(t, err, expected)
}
