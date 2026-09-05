// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"testing"

	"github.com/neko-sc/ent/entc/integration/ent"
	file "github.com/neko-sc/ent/entc/integration/ent/file"
	filetype "github.com/neko-sc/ent/entc/integration/ent/filetype"
	user "github.com/neko-sc/ent/entc/integration/ent/user"

	"github.com/stretchr/testify/require"
)

func Indexes(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)
	t.Log("prevent inserting 2 files with the same (name, user)")
	f1, err := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).Set(file.User, "bar").Save(ctx)
	require.NoError(err)
	require.Equal("foo", f1.Name)
	require.Equal("bar", *f1.User)
	f2, err := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).Set(file.User, "bar").Save(ctx)
	require.Nil(f2)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "%T: %v", err, err)

	t.Log("deletion should allow recreation")
	client.File.DeleteOne(f1).ExecX(ctx)
	f3, err := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).Set(file.User, "bar").Save(ctx)
	require.NoError(err)
	require.Equal("foo", f3.Name)
	require.Equal("bar", *f3.User)

	t.Log("allow inserting 2 files the same name, type and NULL user (optional field)")
	png := client.FileType.Create().Set(filetype.Name, "png").SaveX(ctx)
	f4 := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).SetEdge(file.Type, png.ID).SaveX(ctx)
	f5 := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).SetEdge(file.Type, png.ID).SaveX(ctx)

	t.Log("index on edge sub-graph")
	a8m := client.User.Create().Set(user.Name, "a8m").Set(user.Age, 18).SaveX(ctx)
	err = a8m.Update().AddIDs(user.Files, f4.ID).Exec(ctx)
	require.NoError(err)
	err = a8m.Update().AddIDs(user.Files, f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "cannot have 2 files with the same (name, type, owner)")
	png.Update().RemoveIDs(filetype.Files, f5.ID).ExecX(ctx)
	err = a8m.Update().AddIDs(user.Files, f5.ID).Exec(ctx)
	require.NoError(err)
	err = png.Update().AddIDs(filetype.Files, f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "%T: %v", err, err)
	a8m.Update().RemoveIDs(user.Files, f4.ID, f5.ID).ExecX(ctx)
	png.Update().AddIDs(filetype.Files, f5.ID).ExecX(ctx)

	t.Log("prevent inserting duplicates files in the same insert")
	err = a8m.Update().AddIDs(user.Files, f4.ID, f5.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "%T: %v", err, err)
	require.Zero(a8m.QueryFiles().CountX(ctx))

	t.Log("edge indexes should applied on the edge sub-graph")
	nati := client.User.Create().Set(user.Name, "nati").Set(user.Age, 18).AddIDs(user.Files, f5.ID).SaveX(ctx)
	err = nati.Update().AddIDs(user.Files, f4.ID).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err), "%T: %v", err, err)
	err = a8m.Update().AddIDs(user.Files, f4.ID).Exec(ctx)
	require.NoError(err)

	require.Equal(1, a8m.QueryFiles().CountX(ctx))
	require.Equal(1, nati.QueryFiles().CountX(ctx))
}
