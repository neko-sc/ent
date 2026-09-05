package ent_test

import (
	"testing"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/sqlgraph"
	"github.com/stretchr/testify/require"
)

func TestEdgePredicates(t *testing.T) {
	pets := ent.NewRelation[columnUser, columnPet, int]("pets", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("pets", "id"), sqlgraph.Edge(sqlgraph.O2M, false, "pets", "owner_id"))
	})
	owner := ent.NewUniqueRelation[columnPet, columnUser, int]("owner", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("pets", "id"), sqlgraph.To("users", "id"), sqlgraph.Edge(sqlgraph.M2O, true, "pets", "owner_id"))
	})
	var _ ent.RelationOf[columnUser, columnPet, int] = pets
	var _ ent.RelationOf[columnPet, columnUser, int] = owner
	require.Equal(t, "pets", pets.Ref().Name)
	require.False(t, pets.Ref().Unique)
	require.True(t, owner.Ref().Unique)
	name := ent.StringColumn[columnPet, string]{Table: "pets", Name: "name"}
	for _, test := range []struct {
		predicate ent.Predicate[columnUser]
		where     string
		arguments []any
	}{
		{pets.Has(), `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id")`, nil},
		{pets.HasWith(name.EQ("Ada")), `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id" AND "pets"."name" = $1)`, []any{"Ada"}},
		{pets.HasWith(), `EXISTS (SELECT "pets"."owner_id" FROM "pets" WHERE "users"."id" = "pets"."owner_id")`, nil},
	} {
		selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("users"))
		test.predicate(selector)
		query, arguments := selector.Query()
		require.NoError(t, selector.Err())
		require.Equal(t, "SELECT * FROM \"users\" WHERE "+test.where, query)
		require.Equal(t, test.arguments, arguments)
	}
	selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("pets"))
	owner.HasWith(userName.EQ("Ada"))(selector)
	query, arguments := selector.Query()
	require.Equal(t, `SELECT * FROM "pets" WHERE EXISTS (SELECT "users"."id" FROM "users" WHERE "pets"."owner_id" = "users"."id" AND "users"."name" = $1)`, query)
	require.Equal(t, []any{"Ada"}, arguments)
	selector = sql.Dialect(dialect.Postgres).Select().From(sql.Table("pets"))
	owner.Has()(selector)
	query, _ = selector.Query()
	require.Equal(t, `SELECT * FROM "pets" WHERE "pets"."owner_id" IS NOT NULL`, query)
}

func TestEdgeSelfReferenceAndSchema(t *testing.T) {
	parent := ent.NewUniqueRelation[columnUser, columnUser, int]("parent", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("users", "id"), sqlgraph.Edge(sqlgraph.M2O, false, "users", "parent_id"))
	})
	parent.Configure = func(_ *sql.Selector, step *sqlgraph.Step) { step.To.Schema = "tenant" }
	name := ent.StringColumn[columnUser, string]{Table: "users", Name: "name"}
	selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("users"))
	parent.HasWith(name.EQ("Ada"))(selector)
	query, arguments := selector.Query()
	require.NoError(t, selector.Err())
	require.Equal(t, `SELECT * FROM "users" WHERE EXISTS (SELECT "users_edge"."id" FROM "tenant"."users" AS "users_edge" WHERE "users"."parent_id" = "users_edge"."id" AND "users_edge"."name" = $1)`, query)
	require.Equal(t, []any{"Ada"}, arguments)

	friends := ent.NewRelation[columnUser, columnUser, int]("friends", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("users", "id"), sqlgraph.Edge(sqlgraph.M2M, false, "friends", "user_id", "friend_id"))
	})
	selector = sql.Dialect(dialect.Postgres).Select().From(sql.Table("users"))
	friends.HasWith(name.EQ("Ada"))(selector)
	query, arguments = selector.Query()
	require.NoError(t, selector.Err())
	require.Equal(t, `SELECT * FROM "users" WHERE "users"."id" IN (SELECT "friends"."user_id" FROM "friends" JOIN "users" AS "t1" ON "friends"."friend_id" = "t1"."id" WHERE "t1"."name" = $1)`, query)
	require.Equal(t, []any{"Ada"}, arguments)
}

func TestEdgeOrdering(t *testing.T) {
	pets := ent.NewRelation[columnUser, columnPet, int]("pets", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("pets", "id"), sqlgraph.Edge(sqlgraph.O2M, false, "pets", "owner_id"))
	})
	for _, ordering := range []ent.OrderOption[columnUser]{pets.OrderByCount(sql.OrderDesc()), pets.OrderBy(sql.OrderBySum("age"))} {
		selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("users"))
		ordering(selector)
		query, _ := selector.Query()
		require.NoError(t, selector.Err())
		require.Contains(t, query, `LEFT JOIN`)
		require.Contains(t, query, `ORDER BY`)
	}
	owner := ent.NewUniqueRelation[columnPet, columnUser, int]("owner", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("pets", "id"), sqlgraph.To("users", "id"), sqlgraph.Edge(sqlgraph.M2O, true, "pets", "owner_id"))
	})
	selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("pets"))
	owner.OrderBy(userName.Term(sql.OrderDesc()))(selector)
	query, _ := selector.Query()
	require.NoError(t, selector.Err())
	require.Contains(t, query, `ORDER BY "t1"."name" DESC`)
}
