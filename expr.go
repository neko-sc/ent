package ent

import "github.com/neko-sc/ent/dialect/sql"

// Expr is a SQL expression with a result type. Values should be constructed with
// ExprFunc, Literal, column methods, or the expression combinators.
type Expr[T any] struct {
	render func(*sql.Builder, *sql.Selector)
}

// ExprFunc wraps a SQL renderer. Bind external values with Builder.Arg.
func ExprFunc[T any](render func(*sql.Builder)) Expr[T] {
	return Expr[T]{render: func(builder *sql.Builder, _ *sql.Selector) { render(builder) }}
}

// Render appends the expression to a builder. Columns without an explicit table
// render unqualified outside a selector predicate.
func (e Expr[T]) Render(builder *sql.Builder) { e.render(builder, nil) }

// Literal renders a value as a bound SQL argument.
func Literal[T any](value T) Expr[T] {
	return ExprFunc[T](func(builder *sql.Builder) { builder.Arg(value) })
}

// Number permits arithmetic on built-in and named numeric types.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

func Add[T Number](left, right Expr[T]) Expr[T] {
	return Expr[T]{render: func(builder *sql.Builder, selector *sql.Selector) {
		builder.WriteString("(")
		left.render(builder, selector)
		builder.WriteString(" + ")
		right.render(builder, selector)
		builder.WriteString(")")
	}}
}

func Sub[T Number](left, right Expr[T]) Expr[T] {
	return Expr[T]{render: func(builder *sql.Builder, selector *sql.Selector) {
		builder.WriteString("(")
		left.render(builder, selector)
		builder.WriteString(" - ")
		right.render(builder, selector)
		builder.WriteString(")")
	}}
}

func Mul[T Number](left, right Expr[T]) Expr[T] {
	return Expr[T]{render: func(builder *sql.Builder, selector *sql.Selector) {
		builder.WriteString("(")
		left.render(builder, selector)
		builder.WriteString(" * ")
		right.render(builder, selector)
		builder.WriteString(")")
	}}
}

// Coalesce returns the first non-null expression. At least one is required.
func Coalesce[T any](expressions ...Expr[T]) Expr[T] {
	if len(expressions) == 0 {
		panic("ent: Coalesce requires an expression")
	}
	return Expr[T]{render: func(builder *sql.Builder, selector *sql.Selector) {
		builder.WriteString("COALESCE(")
		for index, expression := range expressions {
			if index > 0 {
				builder.Comma()
			}
			expression.render(builder, selector)
		}
		builder.WriteString(")")
	}}
}

// Excluded refers to the proposed value of a column in an upsert.
func Excluded[E, T any](column ColumnOf[E, T]) Expr[T] {
	return ExprFunc[T](func(builder *sql.Builder) {
		builder.Ident("excluded").WriteString(".").Ident(column.Ref().Name)
	})
}

// PredicateFunc is an expression predicate without an entity type restriction.
// It can be passed directly to a function accepting Predicate[E].
type PredicateFunc = func(*sql.Selector)

// EQ compares an expression to a value for equality.
func (e Expr[T]) EQ(value T) PredicateFunc { return e.compare(sql.OpEQ, value) }

// NEQ compares an expression to a value for inequality.
func (e Expr[T]) NEQ(value T) PredicateFunc { return e.compare(sql.OpNEQ, value) }

// GT tests whether an expression is greater than a value.
func (e Expr[T]) GT(value T) PredicateFunc { return e.compare(sql.OpGT, value) }

// GTE tests whether an expression is greater than or equal to a value.
func (e Expr[T]) GTE(value T) PredicateFunc { return e.compare(sql.OpGTE, value) }

// LT tests whether an expression is less than a value.
func (e Expr[T]) LT(value T) PredicateFunc { return e.compare(sql.OpLT, value) }

// LTE tests whether an expression is less than or equal to a value.
func (e Expr[T]) LTE(value T) PredicateFunc { return e.compare(sql.OpLTE, value) }

func (e Expr[T]) compare(op sql.Op, value T) PredicateFunc {
	return func(selector *sql.Selector) {
		selector.Where(sql.P(func(builder *sql.Builder) {
			e.render(builder, selector)
			builder.WriteOp(op).Arg(value)
		}))
	}
}
