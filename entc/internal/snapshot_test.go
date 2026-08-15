// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/neko-sc/ent/entc/gen"
	"github.com/neko-sc/ent/entc/load"
	"github.com/neko-sc/ent/schema/field"

	"github.com/stretchr/testify/require"
)

func TestMergeField_SemanticMetadata(t *testing.T) {
	semantic := func() *load.FieldType {
		expression, err := load.TypeExpressionFor[string]()
		require.NoError(t, err)
		return &load.FieldType{
			ID:             "string",
			Logical:        load.LogicalTypeString,
			Base:           expression,
			Representation: expression,
			Storage:        load.StorageType{Family: "string"},
			Capabilities:   load.TypeCapabilities{Comparable: true},
		}
	}
	t.Run("matching metadata merges", func(t *testing.T) {
		local := &load.Field{Name: "name", Type: field.TypeString, Semantic: semantic(), Annotations: map[string]any{"local": true}}
		other := &load.Field{Name: "name", Type: field.TypeString, Semantic: semantic(), Annotations: map[string]any{"other": true}, Immutable: true}
		require.NoError(t, mergeField(local, other))
		require.Equal(t, map[string]any{"local": true, "other": true}, local.Annotations)
		require.True(t, local.Immutable)
	})
	tests := []struct {
		name   string
		change func(*load.Field)
		error  string
	}{
		{
			name:   "logical type",
			change: func(other *load.Field) { other.Type = field.TypeInt },
			error:  `field "name" has conflicting logical types "string" and "int"`,
		},
		{
			name:   "semantic identity",
			change: func(field *load.Field) { field.Semantic.ID = "other" },
			error:  `field "name" has conflicting semantic type metadata`,
		},
		{
			name:   "semantic storage",
			change: func(field *load.Field) { field.Semantic.Storage.Family = "other" },
			error:  `field "name" has conflicting semantic type metadata`,
		},
		{
			name:   "semantic capabilities",
			change: func(field *load.Field) { field.Semantic.Capabilities.Comparable = false },
			error:  `field "name" has conflicting semantic type metadata`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := &load.Field{Name: "name", Type: field.TypeString, Semantic: semantic()}
			other := &load.Field{Name: "name", Type: field.TypeString, Semantic: semantic()}
			tt.change(other)
			require.EqualError(t, mergeField(local, other), tt.error)
		})
	}
}

func TestSnapshot_AddFeaturesDeterministic(t *testing.T) {
	require.GreaterOrEqual(t, len(gen.AllFeatures), 3)
	config := &gen.Config{Features: []gen.Feature{gen.AllFeatures[1]}}
	snapshot := &gen.Snapshot{Features: []string{gen.AllFeatures[2].Name, gen.AllFeatures[0].Name, gen.AllFeatures[1].Name}}
	(&Snapshot{Config: config}).addFeatures(snapshot)
	require.Equal(t, []string{gen.AllFeatures[1].Name, gen.AllFeatures[0].Name, gen.AllFeatures[2].Name}, []string{config.Features[0].Name, config.Features[1].Name, config.Features[2].Name})
}

func TestSnapshot_ParseSnapshotStaleness(t *testing.T) {
	encode := func(value string) []byte {
		return fmt.Appendf(nil, "const Schema = %q", value)
	}
	tests := []struct {
		name     string
		snapshot []byte
		message  string
	}{
		{
			name:     "invalid serialization",
			snapshot: encode(`{"Version":`),
			message:  "unmarshal snapshot header",
		},
		{
			name:     "version mismatch",
			snapshot: encode(`{"Version":1}`),
			message:  "unsupported schema snapshot version 1",
		},
		{
			name:     "incompatible schema",
			snapshot: encode(`{"Version":2,"Schemas":[{"fields":[{"type":{}}]}]}`),
			message:  "unmarshal snapshot",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := (&Snapshot{Path: "schema.go"}).parseSnapshot(tt.snapshot)
			require.ErrorIs(t, err, ErrStaleSnapshot)
			require.ErrorContains(t, err, tt.message)
		})
	}
}

func TestSnapshot_RestoreReadErrorIsNotStale(t *testing.T) {
	path := t.TempDir()
	err := (&Snapshot{Path: path}).Restore()
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrStaleSnapshot)
	var pathError *os.PathError
	require.ErrorAs(t, err, &pathError)
	require.Equal(t, path, pathError.Path)
}

func TestSnapshot_ParseSnapshotRejectsSemanticConflict(t *testing.T) {
	expression, err := load.TypeExpressionFor[string]()
	require.NoError(t, err)
	semantic := &load.FieldType{
		ID:             "string",
		Logical:        load.LogicalTypeString,
		Base:           expression,
		Representation: expression,
		Storage:        load.StorageType{Family: "string"},
	}
	local := &gen.Snapshot{Version: 2, Schemas: []*load.Schema{{Name: "User", Fields: []*load.Field{{Name: "name", Type: field.TypeString, Semantic: semantic}}}}}
	other := &gen.Snapshot{Version: 2, Schemas: []*load.Schema{{Name: "User", Fields: []*load.Field{{Name: "name", Type: field.TypeString, Semantic: semantic}}}}}
	other.Schemas[0].Fields[0].Semantic = &load.FieldType{
		ID:             semantic.ID,
		Logical:        semantic.Logical,
		Base:           semantic.Base,
		Representation: semantic.Representation,
		Storage:        load.StorageType{Family: "text"},
	}
	encode := func(snapshot *gen.Snapshot) string {
		encoded, err := json.Marshal(snapshot)
		require.NoError(t, err)
		return fmt.Sprintf("const Schema = %q", encoded)
	}
	_, err = (&Snapshot{Path: "schema.go"}).parseSnapshot([]byte(encode(local) + "\n<<<<<<<\n" + encode(other)))
	require.EqualError(t, err, `merge schema "User": field "name" has conflicting semantic type metadata`)
}

func TestSnapshot_Restore(t *testing.T) {
	t.Log("Running snapshot-restore integration test")
	const testPackage = "../integration/privacy/ent"
	restore, err := addConflicts(testPackage)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, restore()) })
	storage, err := gen.NewStorage("sql")
	require.NoError(t, err)
	snap := &Snapshot{
		Path: filepath.Join(testPackage, "internal/schema.go"),
		Config: &gen.Config{
			Storage: storage,
			Target:  testPackage,
			Schema:  filepath.Join(testPackage, "schema"),
			Header: `
			// Copyright 2019-2026 Facebook Inc.
			// SPDX-License-Identifier: Apache-2.0

			// Code generated by ent, DO NOT EDIT.
			`,
		}}
	require.NoError(t, snap.Restore())
	cmd := exec.Command("go", "generate", "./ent")
	cmd.Dir = filepath.Dir(testPackage)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

// addConflicts adds VCS conflicts to the files that match the given patterns.
func addConflicts(dir string) (func() error, error) {
	infos, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type backup struct {
		content []byte
		mode    os.FileMode
	}
	files := make(map[string]backup)
	restore := func() error {
		for path, file := range files {
			if err := os.WriteFile(path, file.content, file.mode); err != nil {
				return err
			}
		}
		return nil
	}
	for _, info := range infos {
		if info.IsDir() || info.Name() == "generate.go" || info.Name() == "entc.go" {
			continue
		}
		path := filepath.Join(dir, info.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files[path] = backup{content: content, mode: info.Type().Perm()}
		if err := os.WriteFile(path, append(content, '\n', '<', '<', '<', '<', '<', '<', '<'), info.Type().Perm()); err != nil {
			_ = restore()
			return nil, err
		}
	}
	return restore, nil
}
