package ent_test

import (
	"context"
	"database/sql"
	"errors"

	entsql "github.com/neko-sc/ent/dialect/sql"
	"testing"

	"github.com/neko-sc/ent"
	"github.com/stretchr/testify/require"
)

func TestRowTypedDestinations(t *testing.T) {
	aliased := userName.As("display_name")
	optional := userAge.Nullable().As("optional_age")
	row, destinations := ent.NewRow([]ent.Selection{aliased, optional})
	*destinations[0].(*string) = "Alice"
	var name string = ent.Get(row, aliased)
	require.Equal(t, "Alice", name)
	value, present := ent.GetNullable(row, optional)
	require.Zero(t, value)
	require.False(t, present)
	require.Panics(t, func() { ent.Get(row, optional) })
	age := 0
	*destinations[1].(**int) = &age
	require.Equal(t, 0, ent.Get(row, optional))
	value, present = ent.GetNullable(row, optional)
	require.Zero(t, value)
	require.True(t, present)
	require.Equal(t, userName.Ref(), aliased.Ref())
	require.Equal(t, "display_name", aliased.Alias())
	require.Panics(t, func() { ent.Get(row, userName) })
	require.Panics(t, func() { ent.Get(row, userAge.As("display_name")) })
	require.Panics(t, func() { ent.NewRow([]ent.Selection{userName, userName}) })
}

func TestRowScan(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	for _, test := range []struct {
		name    string
		query   string
		present bool
		age     int
	}{
		{"null", `SELECT 'Alice', NULL`, false, 0},
		{"zero", `SELECT 'Alice', 0`, true, 0},
		{"value", `SELECT 'Alice', 42`, true, 42},
	} {
		t.Run(test.name, func(t *testing.T) {
			optional := userAge.As("joined_age").Nullable()
			row, destinations := ent.NewRow([]ent.Selection{userName, optional})
			require.NoError(t, db.QueryRow(test.query).Scan(destinations...))
			require.Equal(t, "Alice", ent.Get(row, userName))
			age, present := ent.GetNullable(row, optional)
			require.Equal(t, test.age, age)
			require.Equal(t, test.present, present)
		})
	}
}

func TestSelectionRendering(t *testing.T) {
	for _, dialect := range []string{"sqlite3", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			selector := entsql.Dialect(dialect).Select().From(entsql.Table("users"))
			expression := ent.ExprSelection(ent.Add(userAge.Expr(), ent.Literal(2)), "adjusted")
			ent.SelectColumns(selector, userName.As("display"), ent.Count(), ent.CountColumn(userAge), ent.Sum(userAge), ent.Min(userAge), ent.Max(userAge), ent.Avg(userAge), expression)
			selector.Where(entsql.EQ(selector.C("age"), 10))
			query, arguments := selector.Query()
			require.NoError(t, selector.Err())
			require.Contains(t, query, "COUNT(*) AS")
			require.Contains(t, query, "sum_age")
			require.Contains(t, query, "min_age")
			require.Contains(t, query, "max_age")
			require.Contains(t, query, "avg_age")
			require.Equal(t, []any{2, 10}, arguments)
			if dialect == "postgres" {
				require.Contains(t, query, "$1")
				require.Contains(t, query, "$2")
			}
			require.Empty(t, ent.Sum(userAge).Ref().Name)
			require.Empty(t, expression.Ref().Name)
		})
	}
}

type rowsQuery struct {
	rows []*ent.Row
	err  error
}

func (q rowsQuery) Rows(context.Context) ([]*ent.Row, error) { return q.rows, q.err }

func TestValuesNullable(t *testing.T) {
	column := userAge.Nullable()
	first, destinations := ent.NewRow([]ent.Selection{column})
	zero := 0
	*destinations[0].(**int) = &zero
	second, _ := ent.NewRow([]ent.Selection{column})
	values, err := ent.ValuesNullable(t.Context(), rowsQuery{rows: []*ent.Row{first, second}}, column)
	require.NoError(t, err)
	require.Equal(t, []*int{&zero, nil}, values)
	failure := errors.New("query failed")
	_, err = ent.Values(t.Context(), rowsQuery{err: failure}, userAge)
	require.ErrorIs(t, err, failure)
	_, err = ent.ValuesNullable(t.Context(), rowsQuery{err: failure}, column)
	require.ErrorIs(t, err, failure)
}

func TestRowNillableValueType(t *testing.T) {
	column := ent.Column[columnUser, *int]{ColumnRef: ent.ColumnRef{Name: "pointer"}}
	row, destinations := ent.NewRow([]ent.Selection{column})
	require.IsType(t, new(*int), destinations[0])
	value, present := ent.GetNullable(row, column)
	require.Nil(t, value)
	require.True(t, present, "an ordinary selection stores T, even when T itself is nil")
	row, destinations = ent.NewRow([]ent.Selection{column.Nullable()})
	require.IsType(t, new(**int), destinations[0])
	value, present = ent.GetNullable(row, column.Nullable())
	require.Nil(t, value)
	require.False(t, present)
}
