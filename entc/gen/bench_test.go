// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neko-sc/ent/entc"
	"github.com/neko-sc/ent/entc/gen"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func BenchmarkGraph_Gen(b *testing.B) {
	target := filepath.Join(os.TempDir(), "ent")
	require.NoError(b, os.MkdirAll(target, os.ModePerm), "creating tmpdir")
	defer os.RemoveAll(target)
	storage, err := gen.NewStorage("sql")
	require.NoError(b, err)
	graph, err := entc.LoadGraph("../integration/ent/schema", &gen.Config{
		Storage: storage,
		IDType:  &field.TypeInfo{Type: field.TypeInt},
		Target:  target,
		Package: "github.com/neko-sc/ent/entc/integration/ent",
		Templates: []*gen.Template{
			gen.MustParse(gen.NewTemplate("template").
				Funcs(gen.Funcs).
				ParseGlob("../integration/ent/template/*.tmpl")),
		},
	})
	require.NoError(b, err)
	b.ResetTimer()
	for range b.N {
		err := graph.Gen()
		require.NoError(b, err)
	}
}
