// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

func TestSelectorStatement(t *testing.T) {
	for _, dialectName := range []string{dialect.SQLite, dialect.Postgres} {
		t.Run(dialectName, func(t *testing.T) {
			users := Dialect(dialectName).Table("users")
			groups := Dialect(dialectName).Table("groups")
			for _, test := range []struct {
				name        string
				selector    *Selector
				tables      []string
				equalities  []Equality
				disjunctive bool
				locked      bool
				references  map[string]int
			}{
				{
					name:     "plain",
					selector: Select().From(users),
					tables:   []string{"users"},
				},
				{
					name:     "join",
					selector: Select().From(users).Join(groups).On(users.C("group_id"), groups.C("id")),
					tables:   []string{"users", "groups"},
				},
				{
					name:       "equality",
					selector:   Select().From(users).Where(EQ(users.C("id"), 1)),
					tables:     []string{"users"},
					equalities: []Equality{{Column: users.C("id"), Values: []any{1}}},
				},
				{
					name:       "in",
					selector:   Select().From(users).Where(In(users.C("id"), 1, 2)),
					tables:     []string{"users"},
					equalities: []Equality{{Column: users.C("id"), Values: []any{1, 2}}},
				},
				{
					name:        "or",
					selector:    Select().From(users).Where(Or(EQ("id", 1), EQ("id", 2))),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}, {Column: "id", Values: []any{2}}},
					disjunctive: true,
				},
				{
					name:        "not",
					selector:    Select().From(users).Where(Not(EQ("id", 1))),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}},
					disjunctive: true,
				},
				{
					name:       "and",
					selector:   Select().From(users).Where(And(EQ("id", 1), EQ("name", "a"))),
					tables:     []string{"users"},
					equalities: []Equality{{Column: "id", Values: []any{1}}, {Column: "name", Values: []any{"a"}}},
				},
				{
					name:       "multiple where",
					selector:   Select().From(users).Where(EQ("id", 1)).Where(EQ("name", "a")),
					tables:     []string{"users"},
					equalities: []Equality{{Column: "id", Values: []any{1}}, {Column: "name", Values: []any{"a"}}},
				},
				{
					name:        "selector or",
					selector:    Select().From(users).Where(EQ("id", 1)).Or().Where(EQ("id", 2)),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}, {Column: "id", Values: []any{2}}},
					disjunctive: true,
				},
				{
					name:        "selector not",
					selector:    Select().From(users).Not().Where(EQ("id", 1)),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}},
					disjunctive: true,
				},
				{
					name:     "in subquery",
					selector: Select().From(users).Where(In(users.C("id"), Select("id").From(groups).Where(EQ("id", 2)))),
					tables:   []string{"users", "groups"},
				},
				{
					name:     "exists",
					selector: Select().From(users).Where(Exists(Select().From(groups).Where(Not(EQ("id", 2))))),
					tables:   []string{"users", "groups"},
				},
				{
					name:     "equality subquery",
					selector: Select().From(users).Where(EQ("id", Select("id").From(groups).Where(EQ("id", 2)))),
					tables:   []string{"users", "groups"},
				},
				{
					name:        "derived table",
					selector:    Select().From(Select().From(users).Where(EQ("id", 1)).As("derived")),
					tables:      []string{"users"},
					disjunctive: true,
				},
				{
					name:       "derived join",
					selector:   Select().From(users).Join(Select().From(groups).Where(Not(EQ("id", 2)))).Where(EQ("id", 1)),
					tables:     []string{"users", "groups"},
					equalities: []Equality{{Column: "id", Values: []any{1}}},
				},
				{
					name:        "union",
					selector:    Select().From(users).Where(EQ("id", 1)).Union(Select().From(groups).Where(EQ("id", 2))),
					tables:      []string{"users", "groups"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}},
					disjunctive: true,
				},
				{
					name:        "union table",
					selector:    Select().From(users).Union(groups),
					tables:      []string{"users", "groups"},
					disjunctive: true,
				},
				{
					name:       "cte prefix",
					selector:   Select().Prefix(With("filtered").As(Select().From(groups).Where(Not(EQ("id", 2))))).From(users).Where(EQ("id", 1)),
					tables:     []string{"groups", "users"},
					equalities: []Equality{{Column: "id", Values: []any{1}}},
				},
				{
					name:     "cte reference",
					selector: Select().Prefix(With("filtered").As(Select().From(groups))).From(Table("filtered")),
					tables:   []string{"groups", "filtered"},
				},
				{
					name:     "cte set operation",
					selector: Select().Prefix(With("filtered").As(UnionAll(Select().From(groups), Select().From(users)))).From(users),
					tables:   []string{"groups", "users"},
				},
				{
					name:     "schema",
					selector: Select().From(Table("users").Schema("tenant").As("people")),
					tables:   []string{"tenant.users"},
				},
				{
					name:     "deduplicate",
					selector: Select().From(users).AppendFrom(Table("users").As("other")).Where(Exists(Select().From(users))),
					tables:   []string{"users"},
				},
				{
					name:     "column equality",
					selector: Select().From(users).Where(ColumnsEQ("id", "other_id")),
					tables:   []string{"users"},
				},
				{
					name:     "expression equality",
					selector: Select().From(users).Where(EQ("id", Raw("other_id"))),
					tables:   []string{"users"},
				},
				{
					name:     "expression in",
					selector: Select().From(users).Where(In("id", 1, Expr("other_id"))),
					tables:   []string{"users"},
				},
				{
					name:       "join on and having excluded",
					selector:   Select().From(users).Join(groups).OnP(Not(EQ("group_id", 2))).Where(EQ("id", 1)).Having(Or(EQ("total", 2), EQ("total", 3))),
					tables:     []string{"users", "groups"},
					equalities: []Equality{{Column: "id", Values: []any{1}}},
				},
				{
					name:     "join on and having tables",
					selector: Select().From(users).Join(groups).OnP(Exists(Select().From(Table("memberships")))).Having(Exists(Select().From(Table("events")))),
					tables:   []string{"users", "groups", "memberships", "events"},
				},
				{
					name: "selection and order expressions",
					selector: SelectExpr(ExprFunc(func(builder *Builder) {
						builder.Wrap(func(builder *Builder) { builder.Join(Select().From(groups).Where(Not(EQ("id", 2)))) })
					})).From(users).Where(EQ("id", 1)).OrderExpr(ExprFunc(func(builder *Builder) {
						builder.Nested(func(builder *Builder) { builder.Join(Select().From(Table("events"))) })
					})),
					tables:     []string{"groups", "users", "events"},
					equalities: []Equality{{Column: "id", Values: []any{1}}},
				},
				{
					name:     "wrapped query",
					selector: Select().From(users).Where(Exists(&Wrapper{format: "%s", wrapped: Select().From(groups).Where(EQ("id", 2))})),
					tables:   []string{"users", "groups"},
				},
				{
					name:     "queries prefix",
					selector: Select().Prefix(Queries{With("filtered").As(Select().From(groups))}).From(users),
					tables:   []string{"groups", "users"},
				},
				{
					name:        "expression from",
					selector:    Select().FromExpr(Union(Select().From(users), Select().From(groups))),
					tables:      []string{"users", "groups"},
					disjunctive: true,
				},
				{
					name:        "nested disjunction",
					selector:    Select().From(users).Where(And(EQ("id", 1), Or(EQ("name", "a"), EQ("name", "b")))),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "id", Values: []any{1}}, {Column: "name", Values: []any{"a"}}, {Column: "name", Values: []any{"b"}}},
					disjunctive: true,
				},
				{
					name:     "empty in",
					selector: Select().From(users).Where(In("id")),
					tables:   []string{"users"},
				},
				{
					name:       "boolean true",
					selector:   Select().From(users).Where(EQ("active", true)),
					tables:     []string{"users"},
					equalities: []Equality{{Column: "active", Values: []any{true}}},
				},
				{
					name:        "boolean false",
					selector:    Select().From(users).Where(EQ("active", false)),
					tables:      []string{"users"},
					equalities:  []Equality{{Column: "active", Values: []any{false}}},
					disjunctive: true,
				},
				{
					name:        "not in",
					selector:    Select().From(users).Where(NotIn("id", 1, 2)),
					tables:      []string{"users"},
					disjunctive: true,
				},
				{
					name:        "not exists",
					selector:    Select().From(users).Where(NotExists(Select().From(groups))),
					tables:      []string{"users", "groups"},
					disjunctive: true,
				},
				{
					name:     "update lock",
					selector: Select().From(users).ForUpdate(),
					tables:   []string{"users"},
					locked:   true,
				},
				{
					name:     "share lock",
					selector: Select().From(users).ForShare(),
					tables:   []string{"users"},
					locked:   true,
				},
				{
					name:       "self join references",
					selector:   Select().From(Table("users").Schema("tenant")).Join(Table("users").Schema("tenant").As("friends")),
					tables:     []string{"tenant.users"},
					references: map[string]int{"tenant.users": 2},
				},
				{
					name:       "nested repeated references",
					selector:   Select().From(Table("users")).Where(In("id", Select("id").From(Table("users")).Join(Table("users").As("friends")))),
					tables:     []string{"users"},
					references: map[string]int{"users": 3},
				},
				{
					name:       "other table subquery references",
					selector:   Select().From(Table("users")).Where(In("pet_id", Select("id").From(Table("pets")))),
					tables:     []string{"users", "pets"},
					references: map[string]int{"users": 1, "pets": 1},
				},
				{
					name:       "different schema references",
					selector:   Select().From(Table("users").Schema("tenant")).Join(Table("users").Schema("other")),
					tables:     []string{"tenant.users", "other.users"},
					references: map[string]int{"tenant.users": 1, "other.users": 1},
				},
				{
					name:       "prefix and wrapped subquery references",
					selector:   Select().Prefix(Queries{With("filtered").As(Select().From(Table("users")))}).From(Table("users")).Where(Exists(&Wrapper{format: "%s", wrapped: Select().From(Table("users"))})),
					tables:     []string{"users"},
					references: map[string]int{"users": 3},
				},
			} {
				t.Run(test.name, func(t *testing.T) {
					test.selector.SetDialect(dialectName)
					require.Equal(t, test.locked, test.selector.Locked())
					// Repeat to prove the derived statement metadata is reset and
					// recomputed rather than accumulated across renders.
					for range 2 {
						test.selector.Query()
						require.NoError(t, test.selector.Err())
						require.Equal(t, test.tables, test.selector.Tables())
						require.Equal(t, test.equalities, test.selector.Equalities())
						require.Equal(t, !test.disjunctive, test.selector.Conjunctive())
						if test.references != nil {
							require.Equal(t, test.references, test.selector.TableReferences())
						}
					}
				})
			}
		})
	}
}

func TestSelectorStatementReset(t *testing.T) {
	t.Run("clone is isolated from its source", func(t *testing.T) {
		selector := Select().From(Table("users")).Where(Not(EQ("id", 1)))
		selector.Query()
		clone := selector.Clone().From(Table("groups")).SetP(EQ("id", 2))
		clone.Query()
		require.Equal(t, []string{"groups"}, clone.Tables())
		require.Equal(t, []Equality{{Column: "id", Values: []any{2}}}, clone.Equalities())
		require.True(t, clone.Conjunctive())
		require.Equal(t, []string{"users"}, selector.Tables())
		require.False(t, selector.Conjunctive())
	})

	t.Run("re-querying recomputes the tables", func(t *testing.T) {
		selector := Select().From(Table("users")).Where(EQ("id", 1))
		selector.Query()
		require.Equal(t, []string{"users"}, selector.Tables())
		selector.From(Table("events")).Query()
		require.Equal(t, []string{"events"}, selector.Tables())
	})

	t.Run("a nil predicate clears the recorded equalities", func(t *testing.T) {
		selector := Select().From(Table("users")).Where(Not(EQ("id", 1)))
		selector.Query()
		require.Equal(t, []Equality{{Column: "id", Values: []any{1}}}, selector.Equalities())
		require.False(t, selector.Conjunctive())
		selector.SetP(nil).Query()
		require.Empty(t, selector.Equalities())
		require.True(t, selector.Conjunctive())
	})
}

func TestMutationPredicateTracking(t *testing.T) {
	for _, dialectName := range []string{dialect.Postgres, dialect.SQLite} {
		t.Run(dialectName, func(t *testing.T) {
			builder := Dialect(dialectName)
			t.Run("conjunctive predicates expose their equalities", func(t *testing.T) {
				for _, test := range []struct {
					name     string
					mutation interface {
						Query() (string, []any)
						Equalities() []Equality
						Conjunctive() bool
					}
				}{
					{
						name:     "update with a disjunctive subquery value",
						mutation: builder.Update("users").Set("name", builder.Select("name").From(builder.Table("other")).Where(Or(EQ("id", 9), EQ("id", 10)))).Where(EQ("id", 1)),
					},
					{
						name:     "delete",
						mutation: builder.Delete("users").Where(EQ("id", 1)),
					},
				} {
					t.Run(test.name, func(t *testing.T) {
						// Repeat to prove the tracked predicates are reset per render.
						for range 2 {
							test.mutation.Query()
							require.True(t, test.mutation.Conjunctive())
							require.Equal(t, []Equality{{Column: "id", Values: []any{1}}}, test.mutation.Equalities())
						}
					})
				}
			})

			t.Run("disjunctive predicates are not conjunctive", func(t *testing.T) {
				for _, test := range []struct {
					name     string
					mutation interface {
						Query() (string, []any)
						Conjunctive() bool
					}
				}{
					{
						name:     "update",
						mutation: builder.Update("users").Set("name", "a").Where(Or(EQ("id", 1), EQ("id", 2))),
					},
					{
						name:     "delete",
						mutation: builder.Delete("users").Where(Or(EQ("id", 1), EQ("id", 2))),
					},
				} {
					t.Run(test.name, func(t *testing.T) {
						test.mutation.Query()
						require.False(t, test.mutation.Conjunctive())
					})
				}
			})
		})
	}
}
