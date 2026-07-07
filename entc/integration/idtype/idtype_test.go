// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package idtype

import (
	"context"
	"testing"

	"github.com/neko-sc/ent/entc/integration/idtype/ent"
	"github.com/neko-sc/ent/entc/integration/idtype/ent/migrate"
	"github.com/neko-sc/ent/entc/integration/idtype/ent/user"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestIDType(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	a8m := client.User.Create().SetName("a8m").SaveX(ctx)
	require.Equal(t, "a8m", a8m.Name)
	neta := client.User.Create().SetName("neta").SetSpouse(a8m).SaveX(ctx)
	require.Equal(t, "neta", neta.Name)
	require.Equal(t, []string{a8m.Name}, neta.QuerySpouse().Select(user.FieldName).StringsX(ctx))
}
