package ent

import "github.com/neko-sc/ent/dialect/sql"

// Count selects the number of rows.
func Count() SelectionOf[int] {
	return selection[int]{alias: "count", aggregate: sql.Count}
}

// CountColumn selects the number of non-null column values.
func CountColumn[E, T any](column ColumnOf[E, T]) SelectionOf[int] {
	return selection[int]{column: column.Ref(), alias: "count_" + column.Ref().Name, aggregate: sql.Count}
}

// Sum selects the sum of non-null column values.
func Sum[E any, T Number](column ColumnOf[E, T]) SelectionOf[T] {
	return selection[T]{column: column.Ref(), alias: "sum_" + column.Ref().Name, aggregate: sql.Sum}
}

// Min selects the smallest non-null column value.
func Min[E, T any](column ColumnOf[E, T]) SelectionOf[T] {
	return selection[T]{column: column.Ref(), alias: "min_" + column.Ref().Name, aggregate: sql.Min}
}

// Max selects the largest non-null column value.
func Max[E, T any](column ColumnOf[E, T]) SelectionOf[T] {
	return selection[T]{column: column.Ref(), alias: "max_" + column.Ref().Name, aggregate: sql.Max}
}

// Avg selects the mean of non-null column values.
func Avg[E any, T Number](column ColumnOf[E, T]) SelectionOf[float64] {
	return selection[float64]{column: column.Ref(), alias: "avg_" + column.Ref().Name, aggregate: sql.Avg}
}

// ExprSelection selects a typed expression under the given output alias.
func ExprSelection[T any](expression Expr[T], alias string) SelectionOf[T] {
	return selection[T]{alias: alias, expression: &expression}
}
