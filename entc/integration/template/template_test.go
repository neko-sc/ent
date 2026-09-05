// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package template

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/template/ent"
	group "github.com/neko-sc/ent/entc/integration/template/ent/group"
	"github.com/neko-sc/ent/entc/integration/template/ent/migrate"
	"github.com/neko-sc/ent/entc/integration/template/ent/pet"
	"github.com/neko-sc/ent/entc/integration/template/ent/user"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestCustomTemplate(t *testing.T) {
	client, err := ent.Open(
		"sqlite3",
		"file:ent?mode=memory&cache=shared&_fk=1",
		// Custom config option.
		ent.HTTPClient(http.DefaultClient),
	)
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	p := client.Pet.Create().Set(pet.Age, 1).SaveX(ctx)
	u := client.User.Create().Set(user.Name, "a8m").AddIDs(user.Pets, p.ID).SaveX(ctx)
	g := client.Group.Create().Set(group.MaxUsers, 10).SaveX(ctx)

	node, err := client.Node(ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p.ID, node.ID)
	require.Equal(t, &ent.Field{Type: "int", Name: "Age", Value: "1"}, node.Fields[0])
	require.Equal(t, &ent.Edge{Type: "User", Name: "Owner", IDs: []int{u.ID}}, node.Edges[0])

	node, err = client.Node(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, u.ID, node.ID)
	require.Equal(t, &ent.Field{Type: "string", Name: "Name", Value: "\"a8m\""}, node.Fields[0])
	require.Equal(t, &ent.Edge{Type: "Pet", Name: "Pets", IDs: []int{p.ID}}, node.Edges[0])

	node, err = client.Node(ctx, g.ID)
	require.NoError(t, err)
	require.Equal(t, g.ID, node.ID)
	require.Equal(t, &ent.Field{Type: "int", Name: "MaxUsers", Value: "10"}, node.Fields[0])

	// check for client additional fields.
	require.True(t, reflect.ValueOf(client).Elem().FieldByName("tables").IsValid())

	result := client.User.Query().Where(user.NameGlob("a8*")).
		AllX(ctx)
	require.Equal(t, 1, len(result))

	var v []struct{ ID, Owner int }
	require.NoError(t, client.Pet.Query().
		Modify(func(s *sql.Selector) {
			t := sql.Table(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldID))
			s.Select(s.C(pet.FieldID), sql.As(t.C(user.FieldID), "owner"))
		}).
		Select().Scan(ctx, &v))
	require.Equal(t, p.ID, v[0].ID)
	require.Equal(t, u.ID, v[0].Owner)

	var sum int
	ages, projectionError := ent.Values(ctx, client.Pet.Query().Select(pet.Age), pet.Age)
	require.NoError(t, projectionError)
	for _, age := range ages {
		sum += age
	}
	var totals []int
	require.NoError(t, client.Pet.Query().Modify(func(s *sql.Selector) {
		s.Select(sql.Sum(pet.FieldAge))
	}).Select().Scan(ctx, &totals))
	require.Equal(t, []int{sum}, totals)

	require.Equal(t, 20, client.HiddenData())
}
