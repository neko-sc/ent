package ent

import (
	"context"
	"fmt"

	"github.com/neko-sc/ent/dialect/sql"
)

// Selection describes a projected column and allocates its typed scan destination.
// Columns select into *T; Nullable selections select into **T to preserve SQL NULL.
type Selection interface {
	Ref() ColumnRef
	Alias() string
	newDestination() any
	render(*sql.Selector) string
}

// SelectionOf retains a selection's value type for Get and GetNullable inference.
type SelectionOf[T any] interface {
	Selection
	value(T)
	As(string) SelectionOf[T]
	Nullable() SelectionOf[T]
}

// Alias returns the unaliased column's name.
func (c Column[E, T]) Alias() string { return c.Name }

func (Column[E, T]) newDestination() any { return new(T) }

func (c Column[E, T]) render(selector *sql.Selector) string { return c.column(selector) }

// As gives a projected column a unique output alias while retaining its value type.
func (c Column[E, T]) As(alias string) SelectionOf[T] {
	return selection[T]{column: c.ColumnRef, alias: alias}
}

// Nullable allocates **T for scanning a potentially NULL column, such as a left join.
// A nil *T represents SQL NULL; a non-nil pointer contains the stored T value.
func (c Column[E, T]) Nullable() SelectionOf[T] {
	return selection[T]{column: c.ColumnRef, alias: c.Name, nullable: true}
}

type selection[T any] struct {
	column     ColumnRef
	alias      string
	nullable   bool
	aggregate  func(string) string
	expression *Expr[T]
}

func (s selection[T]) Ref() ColumnRef {
	if s.aggregate != nil || s.expression != nil {
		return ColumnRef{}
	}
	return s.column
}

func (s selection[T]) render(selector *sql.Selector) string {
	if s.expression != nil {
		selector.AppendSelectExprAs(sql.ExprFunc(func(builder *sql.Builder) {
			s.expression.render(builder, selector)
		}), s.alias)
		return ""
	}
	column := "*"
	if s.column.Name != "" {
		column = s.column.column(selector)
	}
	if s.aggregate != nil {
		return sql.As(s.aggregate(column), s.alias)
	}
	if s.alias != s.column.Name {
		return sql.As(column, s.alias)
	}
	return column
}

// SelectColumns applies typed selections without losing bound expression arguments.
func SelectColumns(selector *sql.Selector, selections ...Selection) {
	selector.Select()
	for _, selection := range selections {
		if column := selection.render(selector); column != "" {
			selector.AppendSelect(column)
		}
	}
}

// RowsQuerier executes a typed projection.
type RowsQuerier interface {
	Rows(context.Context) ([]*Row, error)
}

// Values extracts a selection from every result row.
func Values[T any](ctx context.Context, query RowsQuerier, selection SelectionOf[T]) ([]T, error) {
	rows, err := query.Rows(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]T, len(rows))
	for index, row := range rows {
		values[index] = Get(row, selection)
	}
	return values, nil
}

// ValuesNullable preserves SQL NULL as a nil pointer.
func ValuesNullable[T any](ctx context.Context, query RowsQuerier, selection SelectionOf[T]) ([]*T, error) {
	rows, err := query.Rows(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]*T, len(rows))
	for index, row := range rows {
		if value, present := GetNullable(row, selection); present {
			values[index] = &value
		}
	}
	return values, nil
}
func (s selection[T]) Alias() string { return s.alias }
func (selection[T]) value(T)         {}

func (s selection[T]) newDestination() any {
	if s.nullable {
		return new(*T)
	}
	return new(T)
}

func (s selection[T]) As(alias string) SelectionOf[T] {
	s.alias = alias
	return s
}

func (s selection[T]) Nullable() SelectionOf[T] {
	s.nullable = true
	return s
}

// Row holds typed scan destinations for a projection. Output aliases must be unique.
type Row struct {
	index  map[string]int
	values []any
}

// NewRow allocates a row and destinations to pass to the driver's scan operation.
// It panics for duplicate output aliases; use As to disambiguate joined columns.
func NewRow(selections []Selection) (*Row, []any) {
	row := &Row{index: make(map[string]int, len(selections)), values: make([]any, len(selections))}
	for index, selection := range selections {
		if _, exists := row.index[selection.Alias()]; exists {
			panic(fmt.Sprintf("ent: duplicate selection alias %q", selection.Alias()))
		}
		row.index[selection.Alias()] = index
		row.values[index] = selection.newDestination()
	}
	return row, row.values
}

// Get returns a selected value. It panics if the selection is absent, has the
// wrong type, or holds SQL NULL. Use GetNullable for a potentially NULL selection.
func Get[T any](row *Row, selection SelectionOf[T]) T {
	value, present := GetNullable(row, selection)
	if !present {
		panic(fmt.Sprintf("ent: selection %q is NULL", selection.Alias()))
	}
	return value
}

// GetNullable returns a selected value and false for SQL NULL. Ordinary *T scan
// destinations always return true, even for zero values. Missing selections and
// incompatible value types panic rather than silently returning zero values.
func GetNullable[T any](row *Row, selection SelectionOf[T]) (T, bool) {
	index, exists := row.index[selection.Alias()]
	if !exists {
		panic(fmt.Sprintf("ent: selection %q is absent", selection.Alias()))
	}
	switch destination := row.values[index].(type) {
	case *T:
		return *destination, true
	case **T:
		if *destination != nil {
			return **destination, true
		}
		var zero T
		return zero, false
	default:
		panic(fmt.Sprintf("ent: selection %q has an incompatible value type", selection.Alias()))
	}
}
