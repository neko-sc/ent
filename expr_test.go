package ent_test

import (
	"testing"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

func TestExpressions(t *testing.T) {
	for _, test := range []struct {
		name       string
		expression ent.Expr[int]
		query      string
		arguments  []any
	}{
		{"literal", ent.Literal(3), "$1", []any{3}},
		{"column", userAge.Expr(), `"age"`, nil},
		{"excluded", ent.Excluded(userAge), `"excluded"."age"`, nil},
		{"arithmetic", ent.Mul(ent.Add(userAge.Expr(), ent.Literal(1)), ent.Sub(ent.Literal(9), ent.Literal(3))), `(("age" + $1) * ($2 - $3))`, []any{1, 9, 3}},
		{"coalesce", ent.Coalesce(userAge.Expr(), ent.Literal(0)), `COALESCE("age", $1)`, []any{0}},
		{"custom", ent.ExprFunc[int](func(builder *sql.Builder) { builder.WriteString("42") }), "42", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			var builder sql.Builder
			builder.SetDialect(dialect.Postgres)
			test.expression.Render(&builder)
			query, arguments := builder.Query()
			require.Equal(t, test.query, query)
			require.Equal(t, test.arguments, arguments)
		})
	}
	require.Panics(t, func() { ent.Coalesce[int]() })
}

func TestExpressionPredicates(t *testing.T) {
	for _, test := range []struct {
		name      string
		predicate ent.PredicateFunc
		operator  string
	}{
		{"equal", userAge.Expr().EQ(2), "="},
		{"unequal", userAge.Expr().NEQ(2), "<>"},
		{"greater", userAge.Expr().GT(2), ">"},
		{"greater equal", userAge.Expr().GTE(2), ">="},
		{"less", userAge.Expr().LT(2), "<"},
		{"less equal", userAge.Expr().LTE(2), "<="},
	} {
		t.Run(test.name, func(t *testing.T) {
			selector := sql.Dialect(dialect.Postgres).Select("*").From(sql.Table("users").As("u"))
			ent.And(userName.EQ("a"), test.predicate)(selector)
			query, arguments := selector.Query()
			require.Equal(t, `SELECT * FROM "users" AS "u" WHERE "u"."name" = $1 AND "u"."age" `+test.operator+` $2`, query)
			require.Equal(t, []any{"a", 2}, arguments)
		})
	}
}
