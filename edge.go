package ent

import (
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/sqlgraph"
)

// RelationRef is the dialect-independent identity of an edge.
type RelationRef struct {
	Name   string
	Unique bool
	step   func() *sqlgraph.Step
	// Configure applies selector-local schema configuration to a fresh step.
	Configure func(*sql.Selector, *sqlgraph.Step)
}

// Relation describes a non-unique relation from entity E to neighbor N with key K.
type Relation[E, N, K any] struct{ RelationRef }

// UniqueRelation describes a unique relation from entity E to neighbor N with key K.
type UniqueRelation[E, N, K any] struct{ RelationRef }

// RelationOf accepts either edge kind with matching owner, neighbor and key types.
type RelationOf[E, N, K any] interface {
	Ref() RelationRef
	entity(E)
	neighbor(N)
	key(K)
}

// NewRelation creates a non-unique edge descriptor.
func NewRelation[E, N, K any](name string, step func() *sqlgraph.Step) Relation[E, N, K] {
	return Relation[E, N, K]{RelationRef{Name: name, step: step}}
}

// NewUniqueRelation creates a unique edge descriptor.
func NewUniqueRelation[E, N, K any](name string, step func() *sqlgraph.Step) UniqueRelation[E, N, K] {
	return UniqueRelation[E, N, K]{RelationRef{Name: name, Unique: true, step: step}}
}

func (e RelationRef) queryStep(selector *sql.Selector) *sqlgraph.Step {
	step := e.step()
	if e.Configure != nil {
		e.Configure(selector, step)
	}
	return step
}

// CountQuery builds one grouped count query for the requested parent IDs.
func (e RelationRef) CountQuery(selector *sql.Selector, ids ...any) *sql.Selector {
	step := e.queryStep(selector)
	column := step.Edge.Columns[0]
	if step.Edge.Rel == sqlgraph.M2M && step.Edge.Inverse {
		column = step.Edge.Columns[1]
	}
	table := sql.Dialect(selector.Dialect()).Table(step.Edge.Table).Schema(step.Edge.Schema)
	return sql.Dialect(selector.Dialect()).Select(table.C(column), sql.Count("*")).From(table).
		Where(sql.In(table.C(column), ids...)).GroupBy(table.C(column))
}

func (e RelationRef) Ref() RelationRef { return e }

func (Relation[E, N, K]) entity(E) {}

func (Relation[E, N, K]) neighbor(N) {}

func (Relation[E, N, K]) key(K) {}

func (UniqueRelation[E, N, K]) entity(E) {}

func (UniqueRelation[E, N, K]) neighbor(N) {}

func (UniqueRelation[E, N, K]) key(K) {}

// Has tests whether the edge has any neighbors.
func (e Relation[E, N, K]) Has() Predicate[E] { return e.has() }

// Has tests whether the edge has a neighbor.
func (e UniqueRelation[E, N, K]) Has() Predicate[E] { return e.has() }

func (e RelationRef) has() func(*sql.Selector) {
	return func(selector *sql.Selector) { sqlgraph.HasNeighbors(selector, e.queryStep(selector)) }
}

// HasWith tests whether any neighbor matches all predicates.
func (e Relation[E, N, K]) HasWith(predicates ...Predicate[N]) Predicate[E] {
	return hasNeighborsWith[E](e.RelationRef, predicates)
}

// HasWith tests whether the neighbor matches all predicates.
func (e UniqueRelation[E, N, K]) HasWith(predicates ...Predicate[N]) Predicate[E] {
	return hasNeighborsWith[E](e.RelationRef, predicates)
}

func hasNeighborsWith[E, N any](edge RelationRef, predicates []Predicate[N]) Predicate[E] {
	predicates = append([]Predicate[N](nil), predicates...)
	return func(selector *sql.Selector) {
		sqlgraph.HasNeighborsWith(selector, edge.queryStep(selector), func(neighbor *sql.Selector) {
			for _, predicate := range predicates {
				predicate(neighbor)
			}
		})
	}
}

// OrderByCount orders by the number of neighbors.
func (e Relation[E, N, K]) OrderByCount(options ...sql.OrderTermOption) OrderOption[E] {
	return func(selector *sql.Selector) {
		sqlgraph.OrderByNeighborsCount(selector, e.queryStep(selector), options...)
	}
}

// OrderBy orders by neighbor terms.
func (e Relation[E, N, K]) OrderBy(term sql.OrderTerm, terms ...sql.OrderTerm) OrderOption[E] {
	return e.orderBy(append([]sql.OrderTerm{term}, terms...))
}

// OrderBy orders by neighbor terms.
func (e UniqueRelation[E, N, K]) OrderBy(term sql.OrderTerm, terms ...sql.OrderTerm) OrderOption[E] {
	return e.orderBy(append([]sql.OrderTerm{term}, terms...))
}

func (e RelationRef) orderBy(terms []sql.OrderTerm) func(*sql.Selector) {
	return func(selector *sql.Selector) { sqlgraph.OrderByNeighborTerms(selector, e.queryStep(selector), terms...) }
}
