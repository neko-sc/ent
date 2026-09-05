package ent

import (
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/sqljson"
)

// ColumnRef is the dialect-independent identity of a column.
// An empty Table means the current selector's table.
type ColumnRef struct {
	Table string
	Name  string
}

func (c ColumnRef) column(selector *sql.Selector) string {
	if c.Table != "" && c.Table != selector.TableName() {
		table := sql.Table(c.Table)
		table.SetDialect(selector.Dialect())
		return table.C(c.Name)
	}
	return selector.C(c.Name)
}

// Column describes a column with entity type E and stored value type T.
type Column[E, T any] struct {
	ColumnRef
	// Valuer converts a Go value to its database representation, when needed.
	Valuer func(T) (driver.Value, error)
}

// OrderedColumn adds ordering comparisons for numeric, time, and string values.
type OrderedColumn[E, T any] struct{ Column[E, T] }

// StringColumn adds text matching predicates, including for named string types.
type StringColumn[E, T any] struct{ OrderedColumn[E, T] }

// JSONColumn describes a JSON document and supports predicates on its paths.
type JSONColumn[E, T any] struct{ Column[E, T] }

// ArrayColumn describes a native PostgreSQL array or a SQLite JSON array.
// T is the entire stored value (for example, []string), not its element type.
// Membership arguments are untyped because T does not constrain the element type.
type ArrayColumn[E, T any] struct{ Column[E, T] }

// EntityColumn accepts every column kind belonging to entity E.
type EntityColumn[E any] interface {
	Selection
	entity(E)
}

// ColumnOf accepts every column kind with the same entity and value types.
type ColumnOf[E, T any] interface {
	Ref() ColumnRef
	entity(E)
	value(T)
}

// Predicate restricts a selector for entity E.
type Predicate[E any] func(*sql.Selector)

// OrderOption orders a selector for entity E.
type OrderOption[E any] func(*sql.Selector)

func (c Column[E, T]) Ref() ColumnRef { return c.ColumnRef }

func (Column[E, T]) entity(E) {}

func (Column[E, T]) value(T) {}

// EQ tests equality to a stored value, including when T is nillable.
// Use IsNull to test SQL NULL rather than equality to a nil value.
func (c Column[E, T]) EQ(value T) Predicate[E] {
	return c.compare(sql.EQ, value)
}

// NEQ tests inequality to a stored value.
func (c Column[E, T]) NEQ(value T) Predicate[E] {
	return c.compare(sql.NEQ, value)
}

// EQColumn compares columns with matching value types, including across tables.
func (c Column[E, T]) EQColumn(other SelectionOf[T]) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sql.ColumnsEQ(c.column(selector), other.Ref().column(selector)))
	}
}

// NEQColumn compares columns for inequality.
func (c Column[E, T]) NEQColumn(other SelectionOf[T]) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sql.ColumnsNEQ(c.column(selector), other.Ref().column(selector)))
	}
}

// In tests membership in a set of values. An empty set matches nothing.
func (c Column[E, T]) In(values ...T) Predicate[E] {
	return c.membership(sql.In, values)
}

// NotIn excludes a set of values. An empty set excludes nothing.
func (c Column[E, T]) NotIn(values ...T) Predicate[E] {
	return c.membership(sql.NotIn, values)
}

func (c Column[E, T]) compare(operation func(string, any) *sql.Predicate, value T) Predicate[E] {
	return func(selector *sql.Selector) {
		var argument any = value
		if c.Valuer != nil {
			converted, err := c.Valuer(value)
			if err != nil {
				selector.AddError(err)
				return
			}
			argument = converted
		}
		selector.Where(operation(c.column(selector), argument))
	}
}

func (c Column[E, T]) membership(operation func(string, ...any) *sql.Predicate, values []T) Predicate[E] {
	values = append([]T(nil), values...)
	return func(selector *sql.Selector) {
		arguments := make([]any, len(values))
		for index, value := range values {
			arguments[index] = value
			if c.Valuer != nil {
				converted, err := c.Valuer(value)
				if err != nil {
					selector.AddError(err)
					return
				}
				arguments[index] = converted
			}
		}
		selector.Where(operation(c.column(selector), arguments...))
	}
}

// IsNull tests for SQL NULL.
func (c Column[E, T]) IsNull() Predicate[E] {
	return func(selector *sql.Selector) { selector.Where(sql.IsNull(c.column(selector))) }
}

// NotNull excludes SQL NULL.
func (c Column[E, T]) NotNull() Predicate[E] {
	return func(selector *sql.Selector) { selector.Where(sql.NotNull(c.column(selector))) }
}

// EQExpr compares the column to an expression with the same value type.
func (c Column[E, T]) EQExpr(expression Expr[T]) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sql.P(func(builder *sql.Builder) {
			builder.Ident(c.column(selector)).WriteOp(sql.OpEQ)
			expression.render(builder, selector)
		}))
	}
}

// NEQExpr compares the column to an expression for inequality.
func (c Column[E, T]) NEQExpr(expression Expr[T]) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sql.P(func(builder *sql.Builder) {
			builder.Ident(c.column(selector)).WriteOp(sql.OpNEQ)
			expression.render(builder, selector)
		}))
	}
}

// Expr returns the column as a typed expression. Inside predicates, an empty
// Table resolves against the selector; standalone rendering leaves it unqualified.
func (c Column[E, T]) Expr() Expr[T] {
	return Expr[T]{render: func(builder *sql.Builder, selector *sql.Selector) {
		if selector != nil {
			builder.Ident(c.column(selector))
			return
		}
		if c.Table != "" {
			builder.Ident(c.Table).WriteString(".")
		}
		builder.Ident(c.Name)
	}}
}

// Term returns an ordering term for an edge's neighbor columns.
func (c Column[E, T]) Term(options ...sql.OrderTermOption) *sql.OrderFieldTerm {
	return sql.OrderByField(c.Name, options...)
}

// Asc orders ascending; options may request NULLS FIRST or NULLS LAST.
func (c Column[E, T]) Asc(options ...sql.OrderTermOption) OrderOption[E] {
	return c.order(" ASC", options...)
}

// Desc orders descending; options may request NULLS FIRST or NULLS LAST.
func (c Column[E, T]) Desc(options ...sql.OrderTermOption) OrderOption[E] {
	return c.order(" DESC", options...)
}

func (c Column[E, T]) order(direction string, options ...sql.OrderTermOption) OrderOption[E] {
	ordering := sql.NewOrderTermOptions(options...)
	return func(selector *sql.Selector) {
		selector.OrderExprFunc(func(builder *sql.Builder) {
			builder.Ident(c.column(selector)).WriteString(direction)
			if ordering.NullsFirst {
				builder.WriteString(" NULLS FIRST")
			} else if ordering.NullsLast {
				builder.WriteString(" NULLS LAST")
			}
		})
	}
}

// And joins predicates with AND without absorbing existing selector predicates.
func And[E any](predicates ...Predicate[E]) Predicate[E] {
	return sql.AndPredicates(predicates...)
}

// Or joins predicates with OR without absorbing existing selector predicates.
func Or[E any](predicates ...Predicate[E]) Predicate[E] {
	return sql.OrPredicates(predicates...)
}

// Not negates a predicate without negating existing selector predicates.
func Not[E any](predicate Predicate[E]) Predicate[E] { return sql.NotPredicates(predicate) }

// And joins this predicate and others with AND.
func (p Predicate[E]) And(others ...Predicate[E]) Predicate[E] {
	return And(append([]Predicate[E]{p}, others...)...)
}

// Or joins this predicate and others with OR.
func (p Predicate[E]) Or(others ...Predicate[E]) Predicate[E] {
	return Or(append([]Predicate[E]{p}, others...)...)
}

// Not negates this predicate.
func (p Predicate[E]) Not() Predicate[E] { return Not(p) }

// GT tests whether the column is greater than a value.
func (c OrderedColumn[E, T]) GT(value T) Predicate[E] {
	return c.compare(sql.GT, value)
}

// GTE tests whether the column is greater than or equal to a value.
func (c OrderedColumn[E, T]) GTE(value T) Predicate[E] {
	return c.compare(sql.GTE, value)
}

// LT tests whether the column is less than a value.
func (c OrderedColumn[E, T]) LT(value T) Predicate[E] {
	return c.compare(sql.LT, value)
}

// LTE tests whether the column is less than or equal to a value.
func (c OrderedColumn[E, T]) LTE(value T) Predicate[E] {
	return c.compare(sql.LTE, value)
}

// Between tests an inclusive range.
func (c OrderedColumn[E, T]) Between(lower, upper T) Predicate[E] {
	return And(c.GTE(lower), c.LTE(upper))
}

// Contains matches a literal substring, escaping SQL wildcard characters.
func (c StringColumn[E, T]) Contains(substring string) Predicate[E] {
	return c.match(sql.Contains, substring)
}

// HasPrefix matches a literal prefix.
func (c StringColumn[E, T]) HasPrefix(prefix string) Predicate[E] {
	return c.match(sql.HasPrefix, prefix)
}

// HasSuffix matches a literal suffix.
func (c StringColumn[E, T]) HasSuffix(suffix string) Predicate[E] {
	return c.match(sql.HasSuffix, suffix)
}

// EqualFold tests case-insensitive text equality.
func (c StringColumn[E, T]) EqualFold(value string) Predicate[E] {
	return c.match(sql.EqualFold, value)
}

// ContainsFold matches a literal substring without case sensitivity.
func (c StringColumn[E, T]) ContainsFold(substring string) Predicate[E] {
	return c.match(sql.ContainsFold, substring)
}

func (c StringColumn[E, T]) match(operation func(string, string) *sql.Predicate, text string) Predicate[E] {
	return func(selector *sql.Selector) {
		text := text
		if c.Valuer != nil && reflect.TypeOf(text).ConvertibleTo(reflect.TypeFor[T]()) {
			converted, err := c.Valuer(reflect.ValueOf(text).Convert(reflect.TypeFor[T]()).Interface().(T))
			if err != nil {
				selector.AddError(err)
				return
			}
			value, ok := converted.(string)
			if !ok {
				selector.AddError(fmt.Errorf("ent: column %s text value is %T, not string", c.Name, converted))
				return
			}
			text = value
		}
		selector.Where(operation(c.column(selector), text))
	}
}

// JSONPath identifies a path within an entity's JSON column.
type JSONPath[E any] struct {
	column ColumnRef
	path   []string
}

// Path selects JSON object keys or array indexes as supported by sqljson.Path.
func (c JSONColumn[E, T]) Path(path ...string) JSONPath[E] {
	path = append([]string(nil), path...)
	for index := range path {
		// sqljson embeds path components in SQL string literals on both dialects.
		path[index] = strings.ReplaceAll(path[index], "'", "''")
	}
	return JSONPath[E]{column: c.ColumnRef, path: path}
}

// EQ compares the JSON path's value to a scalar.
func (p JSONPath[E]) EQ(value any) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sqljson.ValueEQ(p.column.column(selector), value, sqljson.Path(p.path...)))
	}
}

// NEQ compares the JSON path's value to a scalar for inequality.
func (p JSONPath[E]) NEQ(value any) Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sqljson.ValueNEQ(p.column.column(selector), value, sqljson.Path(p.path...)))
	}
}

// IsNull tests a JSON null literal, not a missing path or SQL NULL document.
func (p JSONPath[E]) IsNull() Predicate[E] {
	return func(selector *sql.Selector) {
		selector.Where(sqljson.ValueIsNull(p.column.column(selector), sqljson.Path(p.path...)))
	}
}

// Contains tests for an array element. NULL elements do not match.
func (c ArrayColumn[E, T]) Contains(value any) Predicate[E] { return c.HasAll(value) }

// HasAny tests array overlap. An empty argument list matches no non-null arrays.
func (c ArrayColumn[E, T]) HasAny(values ...any) Predicate[E] {
	values = append([]any{}, values...)
	return func(selector *sql.Selector) {
		selector.Where(sql.P(func(builder *sql.Builder) {
			switch builder.Dialect() {
			case dialect.Postgres:
				builder.Ident(c.column(selector)).WriteString(" && ").Arg(values)
			case dialect.SQLite:
				builder.WriteString("CASE WHEN ").Ident(c.column(selector)).WriteString(" IS NULL THEN NULL ELSE ")
				if len(values) == 0 {
					builder.WriteString("FALSE")
				} else {
					builder.WriteString("EXISTS (SELECT 1 FROM json_each(").Ident(c.column(selector))
					builder.WriteString(") WHERE value IN (").Args(values...).WriteString("))")
				}
				builder.WriteString(" END")
			default:
				builder.AddError(&dialect.UnsupportedError{Feature: "array predicates", Dialect: dialect.Dialect(builder.Dialect())})
			}
		}))
	}
}

// HasAll tests array containment. An empty list matches every non-null array.
// Duplicates are ignored, matching PostgreSQL array containment semantics.
func (c ArrayColumn[E, T]) HasAll(values ...any) Predicate[E] {
	values = append([]any{}, values...)
	return func(selector *sql.Selector) {
		selector.Where(sql.P(func(builder *sql.Builder) {
			switch builder.Dialect() {
			case dialect.Postgres:
				builder.Ident(c.column(selector)).WriteString(" @> ").Arg(values)
			case dialect.SQLite:
				builder.WriteString("CASE WHEN ").Ident(c.column(selector)).WriteString(" IS NULL THEN NULL ELSE (")
				if len(values) == 0 {
					builder.WriteString("TRUE")
				}
				for index, value := range values {
					if index > 0 {
						builder.WriteString(" AND ")
					}
					builder.WriteString("EXISTS (SELECT 1 FROM json_each(").Ident(c.column(selector))
					builder.WriteString(") WHERE value = ").Arg(value).WriteString(")")
				}
				builder.WriteString(") END")
			default:
				builder.AddError(&dialect.UnsupportedError{Feature: "array predicates", Dialect: dialect.Dialect(builder.Dialect())})
			}
		}))
	}
}

func (c ArrayColumn[E, T]) length() Expr[int] {
	return Expr[int]{render: func(builder *sql.Builder, selector *sql.Selector) {
		switch builder.Dialect() {
		case dialect.Postgres:
			builder.WriteString("cardinality(")
		case dialect.SQLite:
			builder.WriteString("json_array_length(")
		default:
			builder.AddError(&dialect.UnsupportedError{Feature: "array length", Dialect: dialect.Dialect(builder.Dialect())})
			return
		}
		c.Expr().render(builder, selector)
		builder.WriteString(")")
	}}
}

// LenEQ tests equality to the array's element count.
func (c ArrayColumn[E, T]) LenEQ(length int) Predicate[E] { return c.length().EQ(length) }

// LenNEQ tests inequality to the array's element count.
func (c ArrayColumn[E, T]) LenNEQ(length int) Predicate[E] { return c.length().NEQ(length) }

// LenGT tests whether the array has more than length elements.
func (c ArrayColumn[E, T]) LenGT(length int) Predicate[E] { return c.length().GT(length) }

// LenGTE tests whether the array has at least length elements.
func (c ArrayColumn[E, T]) LenGTE(length int) Predicate[E] { return c.length().GTE(length) }

// LenLT tests whether the array has fewer than length elements.
func (c ArrayColumn[E, T]) LenLT(length int) Predicate[E] { return c.length().LT(length) }

// LenLTE tests whether the array has at most length elements.
func (c ArrayColumn[E, T]) LenLTE(length int) Predicate[E] { return c.length().LTE(length) }
