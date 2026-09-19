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

func TestEdgeCountQuery(t *testing.T) {
	pets := ent.NewRelation[columnUser, columnPet, int]("pets", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("pets", "id"), sqlgraph.Edge(sqlgraph.O2M, false, "pets", "owner_id"))
	})
	groups := ent.NewRelation[columnUser, columnPet, int]("groups", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("groups", "id"), sqlgraph.Edge(sqlgraph.M2M, false, "user_groups", "user_id", "group_id"))
	})
	members := ent.NewRelation[columnUser, columnPet, int]("members", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("groups", "id"), sqlgraph.To("users", "id"), sqlgraph.Edge(sqlgraph.M2M, true, "user_groups", "user_id", "group_id"))
	})
	tenant := ent.NewRelation[columnUser, columnPet, int]("tenant_groups", func() *sqlgraph.Step {
		return sqlgraph.NewStep(sqlgraph.From("users", "id"), sqlgraph.To("groups", "id"), sqlgraph.Edge(sqlgraph.M2M, false, "user_groups", "user_id", "group_id"))
	})
	tenant.Configure = func(_ *sql.Selector, step *sqlgraph.Step) { step.To.Schema = "tenant" }
	petName := ent.StringColumn[columnPet, string]{Table: "pets", Name: "name"}
	groupName := ent.StringColumn[columnPet, string]{Table: "groups", Name: "name"}
	memberName := ent.StringColumn[columnPet, string]{Table: "users", Name: "name"}
	for _, test := range []struct {
		name      string
		count     ent.EdgeCount
		query     string
		arguments []any
	}{
		{
			"unfiltered one to many",
			ent.CountEdge(pets),
			`SELECT "pets"."owner_id", COUNT(*) FROM "pets" WHERE "pets"."owner_id" IN ($1, $2) GROUP BY "pets"."owner_id"`,
			[]any{1, 2},
		},
		{
			"filtered one to many",
			ent.CountEdge(pets, petName.EQ("Ada")),
			`SELECT "pets"."owner_id", COUNT(*) FROM "pets" WHERE "pets"."owner_id" IN ($1, $2) AND "pets"."name" = $3 GROUP BY "pets"."owner_id"`,
			[]any{1, 2, "Ada"},
		},
		{
			"filtered many to many",
			ent.CountEdge(groups, groupName.EQ("Ada")),
			`SELECT "user_groups"."user_id", COUNT(*) FROM "user_groups" WHERE "user_groups"."user_id" IN ($1, $2) AND "user_groups"."group_id" IN (SELECT "groups"."id" FROM "groups" WHERE "groups"."name" = $3) GROUP BY "user_groups"."user_id"`,
			[]any{1, 2, "Ada"},
		},
		{
			"filtered inverse many to many",
			ent.CountEdge(members, memberName.EQ("Ada")),
			`SELECT "user_groups"."group_id", COUNT(*) FROM "user_groups" WHERE "user_groups"."group_id" IN ($1, $2) AND "user_groups"."user_id" IN (SELECT "users"."id" FROM "users" WHERE "users"."name" = $3) GROUP BY "user_groups"."group_id"`,
			[]any{1, 2, "Ada"},
		},
		{
			"filtered many to many with a neighbor schema",
			ent.CountEdge(tenant, groupName.EQ("Ada")),
			`SELECT "user_groups"."user_id", COUNT(*) FROM "user_groups" WHERE "user_groups"."user_id" IN ($1, $2) AND "user_groups"."group_id" IN (SELECT "tenant"."groups"."id" FROM "tenant"."groups" WHERE "tenant"."groups"."name" = $3) GROUP BY "user_groups"."user_id"`,
			[]any{1, 2, "Ada"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := sql.Dialect(dialect.Postgres).Select().From(sql.Table("users"))
			query, arguments := test.count.CountQuery(selector, 1, 2).Query()
			require.NoError(t, selector.Err())
			require.Equal(t, test.query, query)
			require.Equal(t, test.arguments, arguments)
		})
	}
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
