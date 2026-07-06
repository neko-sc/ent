// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/config/ent"
	"github.com/neko-sc/ent/entc/integration/config/ent/migrate"
	"github.com/neko-sc/ent/entc/integration/config/ent/schema"

	_ "github.com/mattn/go-sqlite3"
)

func TestSchemaConfig(t *testing.T) {
	drv, err := sql.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer drv.Close()
	ctx := context.Background()
	client := ent.NewClient(ent.Driver(drv))
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))
	client.User.Create().SetID(1).SaveX(ctx)

	// Check that the table was created with the given custom name.
	table := schema.User{}.Annotations()[0].(entsql.Annotation).Table
	query, args := sql.Select().Count().
		From(sql.Table("sqlite_master")).
		Where(sql.And(sql.EQ("type", "table"), sql.EQ("name", table))).
		Query()
	rows := &sql.Rows{}
	require.NoError(t, drv.Query(ctx, query, args, rows))
	defer rows.Close()
	require.True(t, rows.Next(), "no rows returned")
	var n int
	require.NoError(t, rows.Scan(&n), "scanning count")
	require.Equalf(t, 1, n, "expecting table %q to be exist", table)

	// Check that the table was created with the expected values.
	idIncremental := schema.User{}.Fields()[0].Descriptor().Annotations[0].(entsql.Annotation).Incremental
	require.Equal(t, *idIncremental, migrate.Tables[0].Columns[0].Increment)
	size := schema.User{}.Fields()[1].Descriptor().Annotations[0].(entsql.Annotation).Size
	require.Equal(t, size, migrate.Tables[0].Columns[1].Size)

	fd := schema.User{}.Fields()[1].Descriptor()
	f, err := parser.ParseFile(token.NewFileSet(), "ent/user.go", nil, parser.ParseComments)
	require.NoError(t, err)
	ast.Inspect(f, func(n ast.Node) bool {
		if f, ok := n.(*ast.Field); ok && len(f.Names) > 0 && f.Names[0].Name == fd.Name {
			require.Contains(t, fd.Comment, f.Doc.Text())
			return false
		}
		return true
	})
}
