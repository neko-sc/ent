// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

// Package sqlgraph provides graph abstraction capabilities on top
// of sql-based databases for ent codegen.
package sqlgraph

import (
	"cmp"
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/bits"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/schema/field"
)

// Rel is an edge relation type.
type Rel int

// Relation types.
const (
	_   Rel = iota // Unknown.
	O2O            // One to one / has one.
	O2M            // One to many / has many.
	M2O            // Many to one (inverse perspective for O2M).
	M2M            // Many to many.
)

// String returns the relation name.
func (r Rel) String() (s string) {
	switch r {
	case O2O:
		s = "O2O"
	case O2M:
		s = "O2M"
	case M2O:
		s = "M2O"
	case M2M:
		s = "M2M"
	default:
		s = "Unknown"
	}
	return s
}

// A ConstraintError represents an error from mutation that violates a specific constraint.
type ConstraintError struct {
	msg string
	Err error
}

func (e *ConstraintError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.msg
}
func (e *ConstraintError) Unwrap() error { return e.Err }

func constraintError(err error) error {
	if _, ok := errors.AsType[*ConstraintError](err); ok {
		return err
	}
	if _, ok := errors.AsType[*dialect.ConstraintError](err); ok {
		return &ConstraintError{Err: err}
	}
	return err
}

// A Step provides a path-step information to the traversal functions.
type Step struct {
	// From is the source of the step.
	From struct {
		// V can be either one vertex or set of vertices.
		// It can be a pre-processed step (sql.Query) or a simple Go type (integer or string).
		V any
		// Table holds the table name of V (from).
		Table string
		// Column to join with. Usually the "id" column.
		Column string
	}
	// Edge holds the edge information for getting the neighbors.
	Edge struct {
		// Rel of the edge.
		Rel Rel
		// Schema is an optional name of the database
		// where the table is defined.
		Schema string
		// Table name of where this edge columns reside.
		Table string
		// Columns of the edge.
		// In O2O and M2O, it holds the foreign-key column. Hence, len == 1.
		// In M2M, it holds the primary-key columns of the join table. Hence, len == 2.
		Columns []string
		// Inverse indicates if the edge is an inverse edge.
		Inverse bool
	}
	// To is the dest of the path (the neighbors).
	To struct {
		// Table holds the table name of the neighbors (to).
		Table string
		// Schema is an optional name of the database
		// where the table is defined.
		Schema string
		// Column to join with. Usually the "id" column.
		Column string
	}
}

// StepOption allows configuring Steps using functional options.
type StepOption func(*Step)

// From sets the source of the step.
func From(table, column string, v ...any) StepOption {
	return func(s *Step) {
		s.From.Table = table
		s.From.Column = column
		if len(v) > 0 {
			s.From.V = v[0]
		}
	}
}

// To sets the destination of the step.
func To(table, column string) StepOption {
	return func(s *Step) {
		s.To.Table = table
		s.To.Column = column
	}
}

// Edge sets the edge info for getting the neighbors.
func Edge(rel Rel, inverse bool, table string, columns ...string) StepOption {
	return func(s *Step) {
		s.Edge.Rel = rel
		s.Edge.Table = table
		s.Edge.Columns = columns
		s.Edge.Inverse = inverse
	}
}

// NewStep gets list of options and returns a configured step.
//
//	NewStep(
//		From("table", "pk", V),
//		To("table", "pk"),
//		Edge("name", O2M, "fk"),
//	)
func NewStep(opts ...StepOption) *Step {
	s := &Step{}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// FromEdgeOwner returns true if the step is from an edge owner.
// i.e., from the table that holds the foreign-key.
func (s *Step) FromEdgeOwner() bool {
	return s.Edge.Rel == M2O || (s.Edge.Rel == O2O && s.Edge.Inverse)
}

// ToEdgeOwner returns true if the step is to an edge owner.
// i.e., to the table that holds the foreign-key.
func (s *Step) ToEdgeOwner() bool {
	return s.Edge.Rel == O2M || (s.Edge.Rel == O2O && !s.Edge.Inverse)
}

// ThroughEdgeTable returns true if the step is through a join-table.
func (s *Step) ThroughEdgeTable() bool {
	return s.Edge.Rel == M2M
}

// Neighbors returns a Selector for evaluating the path-step
// and getting the neighbors of one vertex.
func Neighbors[T ~string](dialect T, s *Step) (q *sql.Selector) {
	builder := sql.Dialect(dialect)
	switch {
	case s.ThroughEdgeTable():
		pk1, pk2 := s.Edge.Columns[1], s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1, pk2 = pk2, pk1
		}
		to := builder.Table(s.To.Table).Schema(s.To.Schema)
		join := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		match := builder.Select(join.C(pk1)).
			From(join).
			Where(sql.EQ(join.C(pk2), s.From.V))
		q = builder.Select().
			From(to).
			Join(match).
			On(to.C(s.To.Column), match.C(pk1))
	case s.FromEdgeOwner():
		t1 := builder.Table(s.To.Table).Schema(s.To.Schema)
		t2 := builder.Select(s.Edge.Columns[0]).
			From(builder.Table(s.Edge.Table).Schema(s.Edge.Schema)).
			Where(sql.EQ(s.From.Column, s.From.V))
		q = builder.Select().
			From(t1).
			Join(t2).
			On(t1.C(s.To.Column), t2.C(s.Edge.Columns[0]))
	case s.ToEdgeOwner():
		q = builder.Select().
			From(builder.Table(s.To.Table).Schema(s.To.Schema)).
			Where(sql.EQ(s.Edge.Columns[0], s.From.V))
	}
	return q
}

// SetNeighbors returns a Selector for evaluating the path-step
// and getting the neighbors of set of vertices.
func SetNeighbors[T ~string](dialect T, s *Step) (q *sql.Selector) {
	set := s.From.V.(*sql.Selector)
	builder := sql.Dialect(dialect)
	switch {
	case s.ThroughEdgeTable():
		pk1, pk2 := s.Edge.Columns[1], s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1, pk2 = pk2, pk1
		}
		to := builder.Table(s.To.Table).Schema(s.To.Schema)
		set.Select(set.C(s.From.Column))
		join := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		match := builder.Select(join.C(pk1)).
			From(join).
			Join(set).
			On(join.C(pk2), set.C(s.From.Column))
		q = builder.Select().
			From(to).
			Join(match).
			On(to.C(s.To.Column), match.C(pk1))
	case s.FromEdgeOwner():
		t1 := builder.Table(s.To.Table).Schema(s.To.Schema)
		set.Select(set.C(s.Edge.Columns[0]))
		q = builder.Select().
			From(t1).
			Join(set).
			On(t1.C(s.To.Column), set.C(s.Edge.Columns[0]))
	case s.ToEdgeOwner():
		t1 := builder.Table(s.To.Table).Schema(s.To.Schema)
		set.Select(set.C(s.From.Column))
		q = builder.Select().
			From(t1).
			Join(set).
			On(t1.C(s.Edge.Columns[0]), set.C(s.From.Column))
	}
	return q
}

// HasNeighbors applies on the given Selector a neighbors check.
func HasNeighbors(q *sql.Selector, s *Step) {
	builder := sql.Dialect(q.Dialect())
	switch {
	case s.ThroughEdgeTable():
		pk1 := s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1 = s.Edge.Columns[1]
		}
		join := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		q.Where(
			sql.In(
				q.C(s.From.Column),
				builder.Select(join.C(pk1)).From(join),
			),
		)
	case s.FromEdgeOwner():
		q.Where(sql.NotNull(q.C(s.Edge.Columns[0])))
	case s.ToEdgeOwner():
		to := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		// In case the edge reside on the same table, give
		// the edge an alias to make qualifier different.
		if s.From.Table == s.Edge.Table {
			to.As(fmt.Sprintf("%s_edge", s.Edge.Table))
		}
		q.Where(
			sql.Exists(
				builder.Select(to.C(s.Edge.Columns[0])).
					From(to).
					Where(
						sql.ColumnsEQ(
							q.C(s.From.Column),
							to.C(s.Edge.Columns[0]),
						),
					),
			),
		)
	}
}

// HasNeighborsWith applies on the given Selector a neighbors check.
// The given predicate applies its filtering on the selector.
func HasNeighborsWith(q *sql.Selector, s *Step, pred func(*sql.Selector)) {
	builder := sql.Dialect(q.Dialect())
	switch {
	case s.ThroughEdgeTable():
		pk1, pk2 := s.Edge.Columns[1], s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1, pk2 = pk2, pk1
		}
		to := builder.Table(s.To.Table).Schema(s.To.Schema)
		edge := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		join := builder.Select(edge.C(pk2)).
			From(edge).
			Join(to).
			On(edge.C(pk1), to.C(s.To.Column))
		matches := builder.Select().From(to)
		matches.WithContext(q.Context())
		pred(matches)
		join.FromSelect(matches)
		q.Where(sql.In(q.C(s.From.Column), join))
	case s.FromEdgeOwner():
		to := builder.Table(s.To.Table).Schema(s.To.Schema)
		// Avoid ambiguity in case both source
		// and edge tables are the same.
		if s.To.Table == q.TableName() {
			to.As(fmt.Sprintf("%s_edge", s.To.Table))
			// Choose the alias name until we do not
			// have a collision. Limit to 5 iterations.
			for i := 1; i <= 5; i++ {
				if to.C("c") != q.C("c") {
					break
				}
				to.As(fmt.Sprintf("%s_edge_%d", s.To.Table, i))
			}
		}
		matches := builder.Select(to.C(s.To.Column)).
			From(to)
		matches.WithContext(q.Context())
		matches.Where(
			sql.ColumnsEQ(
				q.C(s.Edge.Columns[0]),
				to.C(s.To.Column),
			),
		)
		pred(matches)
		q.Where(sql.Exists(matches))
	case s.ToEdgeOwner():
		to := builder.Table(s.Edge.Table).Schema(s.Edge.Schema)
		// Avoid ambiguity in case both source
		// and edge tables are the same.
		if s.Edge.Table == q.TableName() {
			to.As(fmt.Sprintf("%s_edge", s.Edge.Table))
			// Choose the alias name until we do not
			// have a collision. Limit to 5 iterations.
			for i := 1; i <= 5; i++ {
				if to.C("c") != q.C("c") {
					break
				}
				to.As(fmt.Sprintf("%s_edge_%d", s.Edge.Table, i))
			}
		}
		matches := builder.Select(to.C(s.Edge.Columns[0])).
			From(to)
		matches.WithContext(q.Context())
		matches.Where(
			sql.ColumnsEQ(
				q.C(s.From.Column),
				to.C(s.Edge.Columns[0]),
			),
		)
		pred(matches)
		q.Where(sql.Exists(matches))
	}
}

// countAlias returns the alias to use for the count column.
func countAlias(q *sql.Selector, s *Step, opt *sql.OrderTermOptions) string {
	if opt.As != "" {
		return opt.As
	}
	selected := make(map[string]struct{})
	for _, c := range q.SelectedColumns() {
		selected[c] = struct{}{}
	}
	column := fmt.Sprintf("count_%s", s.To.Table)
	// If the column was already selected,
	// try to find a free alias.
	if _, ok := selected[column]; ok {
		for i := 1; i <= 5; i++ {
			ci := fmt.Sprintf("%s_%d", column, i)
			if _, ok := selected[ci]; !ok {
				return ci
			}
		}
	}
	return column
}

// OrderByNeighborsCount appends ordering based on the number of neighbors.
// For example, order users by their number of posts.
func OrderByNeighborsCount(q *sql.Selector, s *Step, opts ...sql.OrderTermOption) {
	var (
		join  *sql.Selector
		opt   = sql.NewOrderTermOptions(opts...)
		build = sql.Dialect(q.Dialect())
	)
	switch {
	case s.FromEdgeOwner():
		// For M2O and O2O inverse, the FK resides in the same table.
		// Hence, the order by is on the nullability of the column.
		x := func(b *sql.Builder) {
			b.Ident(s.From.Column)
			if opt.Desc {
				b.WriteOp(sql.OpNotNull)
			} else {
				b.WriteOp(sql.OpIsNull)
			}
		}
		q.OrderExpr(build.Expr(x))
	case s.ThroughEdgeTable():
		countAs := countAlias(q, s, opt)
		terms := []sql.OrderTerm{
			sql.OrderByCount("*", append([]sql.OrderTermOption{sql.OrderAs(countAs)}, opts...)...),
		}
		pk1 := s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1 = s.Edge.Columns[1]
		}
		joinT := build.Table(s.Edge.Table).Schema(s.Edge.Schema)
		join = build.Select(
			joinT.C(pk1),
		).From(joinT).GroupBy(joinT.C(pk1))
		selectTerms(join, terms)
		q.LeftJoin(join).
			On(
				q.C(s.From.Column),
				join.C(pk1),
			)
		orderTerms(q, join, terms)
	case s.ToEdgeOwner():
		countAs := countAlias(q, s, opt)
		terms := []sql.OrderTerm{
			sql.OrderByCount("*", append([]sql.OrderTermOption{sql.OrderAs(countAs)}, opts...)...),
		}
		edgeT := build.Table(s.Edge.Table).Schema(s.Edge.Schema)
		join = build.Select(
			edgeT.C(s.Edge.Columns[0]),
		).From(edgeT).GroupBy(edgeT.C(s.Edge.Columns[0]))
		selectTerms(join, terms)
		q.LeftJoin(join).
			On(
				q.C(s.From.Column),
				join.C(s.Edge.Columns[0]),
			)
		orderTerms(q, join, terms)
	}
}

func orderTerms(q, join *sql.Selector, ts []sql.OrderTerm) {
	for _, t := range ts {
		t := t
		var (
			// Order by column or expression.
			orderC string
			orderX func(*sql.Selector) sql.Querier
			// Order by options.
			desc, nullsfirst, nullslast bool
		)
		switch t := t.(type) {
		case *sql.OrderFieldTerm:
			f := t.Field
			if t.As != "" {
				f = t.As
			}
			orderC = join.C(f)
			if t.Selected {
				q.AppendSelect(orderC)
			}
			desc = t.Desc
			nullsfirst = t.NullsFirst
			nullslast = t.NullsLast
		case *sql.OrderExprTerm:
			if t.As != "" {
				orderC = join.C(t.As)
				if t.Selected {
					q.AppendSelect(orderC)
				}
			} else {
				orderX = t.Expr
			}
			desc = t.Desc
			nullsfirst = t.NullsFirst
			nullslast = t.NullsLast
		default:
			continue
		}
		q.OrderExprFunc(func(b *sql.Builder) {
			// Write the ORDER BY term.
			switch {
			case orderC != "":
				b.WriteString(orderC)
			case orderX != nil:
				b.Join(orderX(join))
			}
			// In PostgreSQL, NULL values sort as if larger than any other value. Therefore,
			// we need to explicitly order NULLs first on ASC and last on DESC unless specified otherwise.
			switch normalizePG := b.Dialect() == dialect.Postgres && !nullsfirst && !nullslast; {
			case normalizePG && desc:
				b.WriteString(" DESC NULLS LAST")
			case normalizePG:
				b.WriteString(" NULLS FIRST")
			case desc:
				b.WriteString(" DESC")
			}
			if nullsfirst {
				b.WriteString(" NULLS FIRST")
			} else if nullslast {
				b.WriteString(" NULLS LAST")
			}
		})
	}
}

// selectTerms appends the select terms to the joined query.
// Afterward, the term aliases are utilized to order the root query.
func selectTerms(q *sql.Selector, ts []sql.OrderTerm) {
	for _, t := range ts {
		switch t := t.(type) {
		case *sql.OrderFieldTerm:
			if t.As != "" {
				q.AppendSelectAs(q.C(t.Field), t.As)
			} else {
				q.AppendSelect(q.C(t.Field))
			}
		case *sql.OrderExprTerm:
			q.AppendSelectExprAs(t.Expr(q), t.As)
		}
	}
}

// OrderByNeighborTerms appends ordering based on the number of neighbors.
// For example, order users by their number of posts.
func OrderByNeighborTerms(q *sql.Selector, s *Step, opts ...sql.OrderTerm) {
	var (
		join  *sql.Selector
		build = sql.Dialect(q.Dialect())
	)
	switch {
	case s.FromEdgeOwner():
		toT := build.Table(s.To.Table).Schema(s.To.Schema)
		join = build.Select(toT.C(s.To.Column)).
			From(toT)
		selectTerms(join, opts)
		q.LeftJoin(join).
			On(q.C(s.Edge.Columns[0]), join.C(s.To.Column))
	case s.ThroughEdgeTable():
		pk1, pk2 := s.Edge.Columns[1], s.Edge.Columns[0]
		if s.Edge.Inverse {
			pk1, pk2 = pk2, pk1
		}
		toT := build.Table(s.To.Table).Schema(s.To.Schema)
		joinT := build.Table(s.Edge.Table).Schema(s.Edge.Schema)
		join = build.Select(pk2).
			From(toT).
			Join(joinT).
			On(toT.C(s.To.Column), joinT.C(pk1)).
			GroupBy(pk2)
		selectTerms(join, opts)
		q.LeftJoin(join).
			On(q.C(s.From.Column), join.C(pk2))
	case s.ToEdgeOwner():
		toT := build.Table(s.Edge.Table).Schema(s.Edge.Schema)
		join = build.Select(toT.C(s.Edge.Columns[0])).
			From(toT).
			GroupBy(toT.C(s.Edge.Columns[0]))
		selectTerms(join, opts)
		q.LeftJoin(join).
			On(q.C(s.From.Column), join.C(s.Edge.Columns[0]))
	}
	orderTerms(q, join, opts)
}

// NeighborsLimit provides a modifier function that limits the
// number of neighbors (rows) loaded per parent row (node).
type NeighborsLimit struct {
	// RowNumber is the window's output column name.
	RowNumber string
	// DefaultOrderField sets the default ordering for
	// sub-queries in case no order terms were provided.
	DefaultOrderField string
	// Offset skips this many neighbors in each partition.
	Offset int
}

// LimitNeighbors returns a modifier that limits the number of neighbors (rows) loaded per parent
// row (node). The "partitionBy" is the foreign-key column (edge) to partition the window function
// by, the "limit" is the maximum number of rows per parent, and the "orderBy" defines the order of
// how neighbors (connected by the edge) are returned.
//
// This function is useful for non-unique edges, such as O2M and M2M, where the same parent can
// have multiple children.
func LimitNeighbors(partitionBy string, limit int, orderBy ...sql.Querier) func(*sql.Selector) {
	l := &NeighborsLimit{
		RowNumber:         "row_number",
		DefaultOrderField: "id",
	}
	return l.Modifier(partitionBy, limit, orderBy...)
}

// Modifier returns a modifier function that limits the number of rows of the eager load query.
func (l *NeighborsLimit) Modifier(partitionBy string, limit int, orderBy ...sql.Querier) func(s *sql.Selector) {
	return func(s *sql.Selector) {
		builder := sql.Dialect(s.Dialect())
		window := sql.RowNumber().PartitionBy(partitionBy)
		terms := orderBy
		if len(terms) == 0 {
			terms = s.OrderTerms()
		}
		switch {
		case len(terms) > 0:
			window.OrderExpr(terms...)
		case l.DefaultOrderField != "":
			window.OrderBy(sql.Asc(s.C(l.DefaultOrderField)))
		default:
			s.AddError(errors.New("no order terms provided for window function"))
			return
		}
		columns := s.UnqualifiedColumns()
		inner := s.Clone().ClearOrder().SetDistinct(false)
		inner.AppendSelectExprAs(window, l.RowNumber).As("limited_neighbors")
		outer := builder.Select().From(inner).WithContext(s.Context())
		for _, column := range columns {
			outer.AppendSelect(inner.C(column))
		}
		if l.Offset > 0 {
			outer.Where(sql.GT(inner.C(l.RowNumber), l.Offset))
		}
		if limit >= 0 {
			outer.Where(sql.LTE(inner.C(l.RowNumber), l.Offset+limit))
		}
		outer.OrderBy(inner.C(l.RowNumber))
		*s = *outer
	}
}

type (
	// FieldSpec holds the information for updating a field
	// column in the database.
	FieldSpec struct {
		Column string
		Type   field.Type
		Value  driver.Value // value to be stored.
	}

	// EdgeTarget holds the information for the target nodes
	// of an edge.
	EdgeTarget struct {
		Nodes  []driver.Value
		IDSpec *FieldSpec
		// Additional fields can be set on the
		// edge join table. Valid for M2M edges.
		Fields []*FieldSpec
	}

	// EdgeSpec holds the information for updating a field
	// column in the database.
	EdgeSpec struct {
		Rel     Rel
		Inverse bool
		Table   string
		Schema  string
		Columns []string
		Bidi    bool        // bidirectional edge.
		Target  *EdgeTarget // target nodes.
	}

	// EdgeSpecs used for perform common operations on list of edges.
	EdgeSpecs []*EdgeSpec

	// NodeSpec defines the information for querying and
	// decoding nodes in the graph.
	NodeSpec struct {
		Table       string
		Schema      string
		Columns     []string
		ID          *FieldSpec   // primary key.
		CompositeID []*FieldSpec // composite id (edge schema).
		// Unique contains unique column sets besides the primary key, in database
		// column names; generated code fills it.
		Unique [][]string
	}
)

// NewFieldSpec creates a new FieldSpec with its required fields.
func NewFieldSpec(column string, typ field.Type) *FieldSpec {
	return &FieldSpec{Column: column, Type: typ}
}

// AddColumnOnce adds the given column to the spec if it is not already present.
func (n *NodeSpec) AddColumnOnce(column string) *NodeSpec {
	for _, c := range n.Columns {
		if c == column {
			return n
		}
	}
	n.Columns = append(n.Columns, column)
	return n
}

// identity returns the ID column and value of the node, or the columns and values of its composite ID.
func (n *NodeSpec) identity() (columns []string, values []any) {
	switch {
	case n.ID != nil:
		return []string{n.ID.Column}, []any{n.ID.Value}
	case len(n.CompositeID) == 2:
		return []string{n.CompositeID[0].Column, n.CompositeID[1].Column}, []any{n.CompositeID[0].Value, n.CompositeID[1].Value}
	}
	return nil, nil
}

// FieldValues returns the values of additional fields that were set on the join-table.
func (e *EdgeTarget) FieldValues() []any {
	vs := make([]any, len(e.Fields))
	for i, f := range e.Fields {
		vs[i] = f.Value
	}
	return vs
}

// Returning scans each row returned by a mutation. Create callbacks must include
// the ID column and assign CreateSpec.ID.Value before returning for edge writes.
type Returning struct {
	Columns []string
	Scan    func(rows dialect.Rows) error
}

func (r *Returning) scan(rows dialect.Rows) (int, error) {
	defer rows.Close()
	count := 0
	for rows.Next() {
		if err := r.Scan(rows); err != nil {
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, constraintError(err)
	}
	return count, rows.Close()
}

type (
	// CreateSpec holds the information for creating
	// a node in the graph.
	CreateSpec struct {
		Expressions map[string]func(*sql.Builder)
		Skipped     bool
		Returning   *Returning
		Table       string
		Schema      string
		ID          *FieldSpec
		Fields      []*FieldSpec
		Edges       []*EdgeSpec

		// The OnConflict option allows providing on-conflict
		// options to the INSERT statement.
		//
		//	sqlgraph.CreateSpec{
		//		OnConflict: []sql.ConflictOption{
		//			sql.ResolveWithNewValues(),
		//		},
		//	}
		//
		OnConflict []sql.ConflictOption
	}

	// BatchCreateSpec holds the information for creating
	// multiple nodes in the graph.
	BatchCreateSpec struct {
		Nodes []*CreateSpec

		// The OnConflict option allows providing on-conflict
		// options to the INSERT statement.
		//
		//	sqlgraph.CreateSpec{
		//		OnConflict: []sql.ConflictOption{
		//			sql.ResolveWithNewValues(),
		//		},
		//	}
		//
		OnConflict []sql.ConflictOption
	}
)

// NewCreateSpec creates a new node creation spec.
func NewCreateSpec(table string, id *FieldSpec) *CreateSpec {
	return &CreateSpec{Table: table, ID: id}
}

// SetField appends a new field setter to the creation spec.
func (u *CreateSpec) SetField(column string, t field.Type, value driver.Value) {
	u.Fields = append(u.Fields, &FieldSpec{
		Column: column,
		Type:   t,
		Value:  value,
	})
}

// CreateNode applies the CreateSpec on the graph. The operation creates a new
// record in the database, and connects it to other nodes specified in spec.Edges.
func CreateNode(ctx context.Context, drv dialect.Driver, spec *CreateSpec) error {
	if sql.Insert(spec.Table).OnConflict(spec.OnConflict...).ConflictDoSelect() && !drv.Capabilities().ConflictDoSelect {
		return &dialect.UnsupportedError{Feature: "ON CONFLICT DO SELECT", Dialect: drv.Dialect()}
	}
	spec.Skipped = false
	gr := graph{tx: drv, builder: sql.Dialect(drv.Dialect())}
	cr := &creator{CreateSpec: spec, graph: gr}
	return constraintError(cr.node(ctx, drv))
}

// BatchCreate applies the BatchCreateSpec on the graph.
func BatchCreate(ctx context.Context, drv dialect.Driver, spec *BatchCreateSpec) error {
	for _, node := range spec.Nodes {
		options := node.OnConflict
		if len(spec.OnConflict) > 0 {
			options = spec.OnConflict
		}
		if sql.Insert(node.Table).OnConflict(options...).ConflictDoSelect() && !drv.Capabilities().ConflictDoSelect {
			return &dialect.UnsupportedError{Feature: "ON CONFLICT DO SELECT", Dialect: drv.Dialect()}
		}
	}
	gr := graph{tx: drv, builder: sql.Dialect(drv.Dialect())}
	cr := &batchCreator{BatchCreateSpec: spec, graph: gr}
	return constraintError(cr.nodes(ctx, drv))
}

type (
	// EdgeMut defines edge mutations.
	EdgeMut struct {
		Add   []*EdgeSpec
		Clear []*EdgeSpec
	}

	// FieldMut defines field mutations.
	FieldMut struct {
		Set   []*FieldSpec // field = ?
		Add   []*FieldSpec // field = field + ?
		Clear []*FieldSpec // field = NULL
	}

	// UpdateSpec holds the information for updating one
	// or more nodes in the graph.
	UpdateSpec struct {
		Returning *Returning
		Node      *NodeSpec
		Edges     EdgeMut
		Fields    FieldMut
		Predicate func(*sql.Selector)
		Modifiers []func(*sql.UpdateBuilder)

		ScanValues    func(columns []string) ([]any, error)
		Assign        func(columns []string, values []any) error
		OldScanValues func(columns []string) ([]any, error)
		OldAssign     func(columns []string, values []any) error
	}
)

// NewUpdateSpec creates a new node update spec.
func NewUpdateSpec(table string, columns []string, id ...*FieldSpec) *UpdateSpec {
	spec := &UpdateSpec{
		Node: &NodeSpec{Table: table, Columns: columns},
	}
	switch {
	case len(id) == 1:
		spec.Node.ID = id[0]
	case len(id) > 1:
		spec.Node.CompositeID = id
	}
	return spec
}

// AddModifier adds a new statement modifier to the spec.
func (u *UpdateSpec) AddModifier(m func(*sql.UpdateBuilder)) {
	u.Modifiers = append(u.Modifiers, m)
}

// AddModifiers adds a list of statement modifiers to the spec.
func (u *UpdateSpec) AddModifiers(m ...func(*sql.UpdateBuilder)) {
	u.Modifiers = append(u.Modifiers, m...)
}

// SetField appends a new field setter to the update spec.
func (u *UpdateSpec) SetField(column string, t field.Type, value driver.Value) {
	u.Fields.Set = append(u.Fields.Set, &FieldSpec{
		Column: column,
		Type:   t,
		Value:  value,
	})
}

// AddField appends a new field adder to the update spec.
func (u *UpdateSpec) AddField(column string, t field.Type, value driver.Value) {
	u.Fields.Add = append(u.Fields.Add, &FieldSpec{
		Column: column,
		Type:   t,
		Value:  value,
	})
}

// ClearField appends a new field cleaner (set to NULL) to the update spec.
func (u *UpdateSpec) ClearField(column string, t field.Type) {
	u.Fields.Clear = append(u.Fields.Clear, &FieldSpec{
		Column: column,
		Type:   t,
	})
}

// UpdateNode applies the UpdateSpec on one node in the graph.
func UpdateNode(ctx context.Context, drv dialect.Driver, spec *UpdateSpec) error {
	if spec.OldScanValues != nil && !drv.Capabilities().ReturningOld {
		return &dialect.UnsupportedError{Feature: "RETURNING OLD", Dialect: drv.Dialect()}
	}
	tx, err := drv.Tx(ctx)
	if err != nil {
		return err
	}
	gr := graph{tx: tx, builder: sql.Dialect(drv.Dialect())}
	cr := &updater{UpdateSpec: spec, graph: gr}
	if err := cr.node(ctx, tx); err != nil {
		return constraintError(rollback(tx, err))
	}
	return constraintError(tx.Commit())
}

// UpdateNodes applies the UpdateSpec on a set of nodes in the graph.
func UpdateNodes(ctx context.Context, drv dialect.Driver, spec *UpdateSpec) (int, error) {
	gr := graph{tx: drv, builder: sql.Dialect(drv.Dialect())}
	cr := &updater{UpdateSpec: spec, graph: gr}
	count, err := cr.nodes(ctx, drv)
	return count, constraintError(err)
}

// BatchUpdateSpec holds the information for updating multiple
// nodes of one table, each with its own changes.
type BatchUpdateSpec struct {
	Nodes []*UpdateSpec
	// Predicate is the condition every node must match besides its ID.
	Predicate func(*sql.Selector)
	// PostgresTypes maps the ID, field and foreign-key columns
	// to their PostgreSQL types, used to type the row source.
	PostgresTypes map[string]string
}

// BatchUpdate applies the BatchUpdateSpec on the graph with set-based UPDATE ... FROM statements.
// Nodes that do not exist or do not match the predicate fail the whole update with a NotFoundError
// holding their IDs. The predicate is evaluated once, against the rows before the update. If drv is a
// transaction, the statements that already ran stay applied, and the caller must roll back the
// transaction on error.
func BatchUpdate(ctx context.Context, drv dialect.Driver, spec *BatchUpdateSpec) error {
	updater := &batchUpdater{BatchUpdateSpec: spec, graph: graph{builder: sql.Dialect(drv.Dialect())}, postgres: drv.Dialect() == dialect.Postgres}
	return constraintError(updater.nodes(ctx, drv))
}

// NotFoundError returns when trying to update an
// entity, and it was not found in the database.
type NotFoundError struct {
	table string
	id    driver.Value
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("record with id %v not found in table %s", e.id, e.table)
}

// DeleteSpec holds the information for delete one
// or more nodes in the graph.
type DeleteSpec struct {
	Returning *Returning
	Node      *NodeSpec
	Predicate func(*sql.Selector)
}

// NewDeleteSpec creates a new node deletion spec.
func NewDeleteSpec(table string, id *FieldSpec) *DeleteSpec {
	return &DeleteSpec{Node: &NodeSpec{Table: table, ID: id}}
}

// DeleteNodes applies the DeleteSpec on the graph.
func DeleteNodes(ctx context.Context, drv dialect.Driver, spec *DeleteSpec) (int, error) {
	var (
		builder = sql.Dialect(drv.Dialect())
	)
	selector := builder.Select().
		From(builder.Table(spec.Node.Table).Schema(spec.Node.Schema)).
		WithContext(ctx)
	if pred := spec.Predicate; pred != nil {
		pred(selector)
	}
	statement := builder.Delete(spec.Node.Table).Schema(spec.Node.Schema).FromSelect(selector)
	if spec.Returning != nil {
		query, args := statement.Returning(spec.Returning.Columns...).Query()
		rows, err := drv.Query(dialect.WithStatement(ctx, spec.Node.writeStatement(statement)), query, args)
		if err != nil {
			return 0, constraintError(err)
		}
		return spec.Returning.scan(rows)
	}
	query, args := statement.Query()
	res, err := drv.Exec(dialect.WithStatement(ctx, spec.Node.writeStatement(statement)), query, args)
	if err != nil {
		return 0, constraintError(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

// QuerySpec holds the information for querying
// nodes in the graph.
type QuerySpec struct {
	Node *NodeSpec     // Nodes info.
	From *sql.Selector // Optional query source (from path).

	Limit     int
	Offset    int
	Unique    bool
	Order     func(*sql.Selector)
	Predicate func(*sql.Selector)
	Modifiers []func(*sql.Selector)

	ScanValues func(columns []string) ([]any, error)
	Assign     func(columns []string, values []any) error
}

// NewQuerySpec creates a new node query spec.
func NewQuerySpec(table string, columns []string, id *FieldSpec) *QuerySpec {
	return &QuerySpec{
		Node: &NodeSpec{
			ID:      id,
			Table:   table,
			Columns: columns,
		},
	}
}

// QueryNodes queries the nodes in the graph query and scans them to the given values.
func QueryNodes(ctx context.Context, drv dialect.Driver, spec *QuerySpec) error {
	builder := sql.Dialect(drv.Dialect())
	qr := &query{graph: graph{builder: builder}, QuerySpec: spec}
	return qr.nodes(ctx, drv)
}

// CountNodes counts the nodes in the given graph query.
func CountNodes(ctx context.Context, drv dialect.Driver, spec *QuerySpec) (int, error) {
	builder := sql.Dialect(drv.Dialect())
	qr := &query{graph: graph{builder: builder}, QuerySpec: spec}
	return qr.count(ctx, drv)
}

// EdgeQuerySpec holds the information for querying
// edges in the graph.
type EdgeQuerySpec struct {
	Edge       *EdgeSpec
	Predicate  func(*sql.Selector)
	ScanValues func() [2]any
	Assign     func(out, in any) error
}

// QueryEdges queries the edges in the graph and scans the result with the given dest function.
func QueryEdges(ctx context.Context, drv dialect.Driver, spec *EdgeQuerySpec) error {
	if len(spec.Edge.Columns) != 2 {
		return fmt.Errorf("sqlgraph: edge query requires 2 columns (out, in)")
	}
	out, in := spec.Edge.Columns[0], spec.Edge.Columns[1]
	if spec.Edge.Inverse {
		out, in = in, out
	}
	selector := sql.Dialect(drv.Dialect()).
		Select(out, in).
		From(sql.Table(spec.Edge.Table).Schema(spec.Edge.Schema))
	if p := spec.Predicate; p != nil {
		p(selector)
	}
	query, args := selector.Query()
	rows, err := drv.Query(dialect.WithStatement(ctx, readStatement(selector, dialect.QualifiedTable(spec.Edge.Schema, spec.Edge.Table))), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		values := spec.ScanValues()
		if err := rows.Scan(values[0], values[1]); err != nil {
			return err
		}
		if err := spec.Assign(values[0], values[1]); err != nil {
			return err
		}
	}
	return rows.Err()
}

type query struct {
	graph
	*QuerySpec
}

func (q *query) nodes(ctx context.Context, drv dialect.Driver) error {
	selector, err := q.selector(ctx)
	if err != nil {
		return err
	}
	query, args := selector.Query()
	rows, err := drv.Query(dialect.WithStatement(ctx, q.Node.readStatement(selector)), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	for rows.Next() {
		values, err := q.ScanValues(columns)
		if err != nil {
			return err
		}
		for i, v := range values {
			if _, ok := v.(*sql.UnknownType); ok {
				values[i] = sql.ScanTypeOf(rows, i)
			}
		}
		if err := rows.Scan(values...); err != nil {
			return err
		}
		if err := q.Assign(columns, values); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (q *query) count(ctx context.Context, drv dialect.Driver) (int, error) {
	selector, err := q.selector(ctx)
	if err != nil {
		return 0, err
	}
	// Remove any ORDER BY clauses present in the COUNT query as
	// they are not allowed in some databases, such as PostgreSQL.
	if q.Order != nil {
		selector.ClearOrder()
	}
	// If no columns were selected in count,
	// the default selection is by node ids.
	columns := q.Node.Columns
	if len(columns) == 0 && q.Node.ID != nil {
		columns = append(columns, q.Node.ID.Column)
	}
	for i, c := range columns {
		columns[i] = selector.C(c)
	}
	if q.Unique {
		selector.SetDistinct(false)
		selector.Count(sql.Distinct(columns...))
	} else {
		selector.Count(columns...)
	}
	query, args := selector.Query()
	rows, err := drv.Query(dialect.WithStatement(ctx, readStatement(selector, dialect.QualifiedTable(q.Node.Schema, q.Node.Table))), query, args)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	return sql.ScanInt(rows)
}

func (q *query) selector(ctx context.Context) (*sql.Selector, error) {
	selector := q.builder.
		Select().
		From(q.builder.Table(q.Node.Table).Schema(q.Node.Schema)).
		WithContext(ctx)
	if q.From != nil {
		selector = q.From
		selector.WithContext(ctx)
	}
	selector.Select(selector.Columns(q.Node.Columns...)...)
	if order := q.Order; order != nil {
		order(selector)
	}
	if pred := q.Predicate; pred != nil {
		pred(selector)
	}
	if q.Offset != 0 {
		// Limit is mandatory for the offset clause. We start
		// with default value, and override it below if needed.
		selector.Offset(q.Offset).Limit(math.MaxInt32)
	}
	if q.Limit != 0 {
		selector.Limit(q.Limit)
	}
	if q.Unique {
		selector.Distinct()
	}
	for _, m := range q.Modifiers {
		m(selector)
	}
	if err := selector.Err(); err != nil {
		return nil, err
	}
	return selector, nil
}

type updater struct {
	graph
	*UpdateSpec
}

func (u *updater) node(ctx context.Context, tx dialect.ExecQuerier) error {
	var (
		id         driver.Value
		idp        *sql.Predicate
		addEdges   = EdgeSpecs(u.Edges.Add).GroupRel()
		clearEdges = EdgeSpecs(u.Edges.Clear).GroupRel()
	)
	switch {
	// In case it is not an edge schema, the id holds the PK
	// of the node used for linking it with the other nodes.
	case u.Node.ID != nil:
		id = u.Node.ID.Value
		idp = sql.EQ(u.Node.ID.Column, id)
	case len(u.Node.CompositeID) == 2:
		idp = sql.And(
			sql.EQ(u.Node.CompositeID[0].Column, u.Node.CompositeID[0].Value),
			sql.EQ(u.Node.CompositeID[1].Column, u.Node.CompositeID[1].Value),
		)
	case len(u.Node.CompositeID) != 2:
		return fmt.Errorf("sql/sqlgraph: invalid composite id for update table %q", u.Node.Table)
	default:
		return fmt.Errorf("sql/sqlgraph: missing node id for update table %q", u.Node.Table)
	}
	selector := u.builder.Select(u.Node.Columns...).From(u.builder.Table(u.Node.Table).Schema(u.Node.Schema))
	if pred := u.Predicate; pred != nil {
		pred(selector)
	}
	if predicate := selector.P(); predicate != nil {
		selector.SetP(sql.And(idp, predicate))
	} else {
		selector.SetP(idp)
	}
	update := u.builder.Update(u.Node.Table).Schema(u.Node.Schema).FromSelect(selector)
	if err := u.setTableColumns(update, addEdges, clearEdges); err != nil {
		return err
	}
	for _, m := range u.Modifiers {
		m(update)
	}
	if err := update.Err(); err != nil {
		return err
	}
	if update.Empty() && u.ScanValues != nil {
		if u.OldScanValues != nil {
			selector.AppendSelect(u.Node.Columns...)
		}
		query, args := selector.Query()
		rows, err := tx.Query(dialect.WithStatement(ctx, u.Node.readStatement(selector)), query, args)
		if err != nil {
			return err
		}
		if err := u.scan(rows); err != nil {
			return err
		}
	} else if !update.Empty() {
		if u.ScanValues != nil {
			update.Returning(u.Node.Columns...)
			if u.OldScanValues != nil {
				update.ReturningOldNew(u.Node.Columns, u.Node.Columns)
			}
			query, args := update.Query()
			rows, err := tx.Query(dialect.WithStatement(ctx, u.Node.writeStatement(update)), query, args)
			if err != nil {
				return err
			}
			if err := u.scan(rows); err != nil {
				return err
			}
		} else {
			query, args := update.Query()
			result, err := tx.Exec(dialect.WithStatement(ctx, u.Node.writeStatement(update)), query, args)
			if err != nil {
				return err
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if affected == 0 {
				return &NotFoundError{table: u.Node.Table, id: id}
			}
		}
	}
	if id != nil {
		return u.setExternalEdges(ctx, []driver.Value{id}, addEdges, clearEdges)
	}
	return nil
}

func (u *updater) nodes(ctx context.Context, drv dialect.Driver) (int, error) {
	var (
		addEdges   = EdgeSpecs(u.Edges.Add).GroupRel()
		clearEdges = EdgeSpecs(u.Edges.Clear).GroupRel()
		multiple   = hasExternalEdges(addEdges, clearEdges)
		update     = u.builder.Update(u.Node.Table).Schema(u.Node.Schema)
		selector   = u.builder.Select().
				From(u.builder.Table(u.Node.Table).Schema(u.Node.Schema)).
				WithContext(ctx)
	)
	switch {
	// In case it is not an edge schema, the id holds the PK of
	// the returned nodes are used for updating external tables.
	case u.Node.ID != nil:
		selector.Select(u.Node.ID.Column)
	case len(u.Node.CompositeID) == 2:
		// Other edge-schemas (M2M tables) cannot be updated by this operation.
		// Also, in case there is a need to update an external foreign-key, it must
		// be a single value and the user should use the "update by id" API instead.
		if multiple {
			return 0, fmt.Errorf("sql/sqlgraph: update edge schema table %q cannot update external tables", u.Node.Table)
		}
	case len(u.Node.CompositeID) != 2:
		return 0, fmt.Errorf("sql/sqlgraph: invalid composite id for update table %q", u.Node.Table)
	default:
		return 0, fmt.Errorf("sql/sqlgraph: missing node id for update table %q", u.Node.Table)
	}
	if err := u.setTableColumns(update, addEdges, clearEdges); err != nil {
		return 0, err
	}
	if pred := u.Predicate; pred != nil {
		pred(selector)
	}
	// In case of single statement update, avoid opening a transaction manually.
	if !multiple {
		update.FromSelect(selector)
		return u.updateTable(ctx, update)
	}
	tx, err := drv.Tx(ctx)
	if err != nil {
		return 0, err
	}
	u.tx = tx
	affected, err := func() (int, error) {
		var (
			ids         []driver.Value
			query, args = selector.Query()
		)
		rows, err := u.tx.Query(dialect.WithStatement(ctx, u.Node.readStatement(selector)), query, args)
		if err != nil {
			return 0, fmt.Errorf("querying table %s: %w", u.Node.Table, err)
		}
		defer rows.Close()
		if err := sql.ScanSlice(rows, &ids); err != nil {
			return 0, fmt.Errorf("scan node ids: %w", err)
		}
		if err := rows.Close(); err != nil {
			return 0, err
		}
		if len(ids) == 0 {
			return 0, nil
		}
		update.Where(matchID(u.Node.ID.Column, ids))
		// In case of multi statement update, that change can
		// affect more than 1 table, and therefore, we return
		// the list of ids as number of affected records.
		if _, err := u.updateTable(ctx, update); err != nil {
			return 0, err
		}
		if err := u.setExternalEdges(ctx, ids, addEdges, clearEdges); err != nil {
			return 0, err
		}
		return len(ids), nil
	}()
	if err != nil {
		return 0, rollback(tx, err)
	}
	return affected, tx.Commit()
}

func (u *updater) updateTable(ctx context.Context, stmt *sql.UpdateBuilder) (int, error) {
	for _, m := range u.Modifiers {
		m(stmt)
	}
	if err := stmt.Err(); err != nil {
		return 0, err
	}
	if stmt.Empty() && u.Returning != nil && len(u.Returning.Columns) > 0 {
		stmt.Set(u.Returning.Columns[0], sql.ExprFunc(func(builder *sql.Builder) { builder.Ident(u.Returning.Columns[0]) }))
	}
	if stmt.Empty() {
		return 0, nil
	}
	if u.Returning != nil {
		query, args := stmt.Returning(u.Returning.Columns...).Query()
		rows, err := u.tx.Query(dialect.WithStatement(ctx, u.Node.writeStatement(stmt)), query, args)
		if err != nil {
			return 0, constraintError(err)
		}
		return u.Returning.scan(rows)
	}
	var (
		query, args = stmt.Query()
	)
	res, err := u.tx.Exec(dialect.WithStatement(ctx, u.Node.writeStatement(stmt)), query, args)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (u *updater) setExternalEdges(ctx context.Context, ids []driver.Value, addEdges, clearEdges map[Rel][]*EdgeSpec) error {
	if err := u.clearM2MEdges(ctx, ids, clearEdges[M2M]); err != nil {
		return err
	}
	if err := u.addM2MEdges(ctx, ids, addEdges[M2M]); err != nil {
		return err
	}
	if err := u.clearFKEdges(ctx, ids, append(clearEdges[O2M], clearEdges[O2O]...)); err != nil {
		return err
	}
	if err := u.addFKEdges(ctx, ids, append(addEdges[O2M], addEdges[O2O]...)); err != nil {
		return err
	}
	return nil
}

// setTableColumns sets the table columns and foreign_keys used in insert.
func (u *updater) setTableColumns(update *sql.UpdateBuilder, addEdges, clearEdges map[Rel][]*EdgeSpec) error {
	return updateColumns(u.UpdateSpec, addEdges, clearEdges,
		func(column string) { update.SetNull(column) },
		func(column string, value driver.Value) { update.Set(column, value) },
		func(column string, value driver.Value) { update.Add(column, value) },
	)
}

// updateColumns reports the column changes of an update in the order they
// apply: cleared columns, set columns and foreign keys, and added columns.
func updateColumns(spec *UpdateSpec, addEdges, clearEdges map[Rel][]*EdgeSpec, setNull func(column string), set, add func(column string, value driver.Value)) error {
	// Avoid multiple assignments to the same column.
	setEdges := make(map[string]bool)
	for _, e := range addEdges[M2O] {
		setEdges[e.Columns[0]] = true
	}
	for _, e := range addEdges[O2O] {
		if e.Inverse || e.Bidi {
			setEdges[e.Columns[0]] = true
		}
	}
	for _, fi := range spec.Fields.Clear {
		setNull(fi.Column)
	}
	for _, e := range clearEdges[M2O] {
		if col := e.Columns[0]; !setEdges[col] {
			setNull(col)
		}
	}
	for _, e := range clearEdges[O2O] {
		col := e.Columns[0]
		if (e.Inverse || e.Bidi) && !setEdges[col] {
			setNull(col)
		}
	}
	if err := setTableColumns(spec.Fields.Set, addEdges, set); err != nil {
		return err
	}
	for _, fi := range spec.Fields.Add {
		add(fi.Column, fi.Value)
	}
	return nil
}

func (u *updater) scan(rows dialect.Rows) error {
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		if len(u.Node.CompositeID) == 2 {
			return &NotFoundError{table: u.Node.Table, id: []driver.Value{u.Node.CompositeID[0].Value, u.Node.CompositeID[1].Value}}
		}
		return &NotFoundError{table: u.Node.Table, id: u.Node.ID.Value}
	}
	if u.OldScanValues != nil {
		columns = u.Node.Columns
	}
	values, err := u.ScanValues(columns)
	if err != nil {
		return err
	}
	var oldValues []any
	if u.OldScanValues != nil {
		oldValues, err = u.OldScanValues(columns)
		if err != nil {
			return err
		}
		values = append(oldValues, values...)
	}
	for i, v := range values {
		if _, ok := v.(*sql.UnknownType); ok {
			values[i] = sql.ScanTypeOf(rows, i)
		}
	}
	if err := rows.Scan(values...); err != nil {
		return fmt.Errorf("failed scanning rows: %w", err)
	}
	if u.OldScanValues != nil {
		if err := u.OldAssign(columns, values[:len(oldValues)]); err != nil {
			return err
		}
		values = values[len(oldValues):]
	}
	if err := u.Assign(columns, values); err != nil {
		return err
	}
	if rows.Next() {
		return errors.New("sqlgraph: update returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

// The modes of a column in one row of a batch update. The
// row source passes them to the CASE expression of mixed columns.
const (
	batchKeep int16 = iota
	batchSet
	batchNull
	batchAdd
	// batchSetAdd adds the delta to the value the row sets, as UpdateBuilder.Add does after Set.
	batchSetAdd
)

type (
	batchUpdater struct {
		graph
		*BatchUpdateSpec

		postgres bool
		alias    string    // name of the row source.
		node     *NodeSpec // table node shared by the nodes.

		identifiers []string       // ID columns.
		columns     []string       // sorted union of the updated columns.
		modes       []uint8        // bit masks of the modes each column has in the batch.
		sources     []batchSource  // columns of the row source, starting with the ID columns.
		returning   []string       // ID columns and the union of the node columns.
		positions   map[string]int // positions of the returning columns.

		rows map[any]*batchRow // rows by their key.

		predicate *sql.Predicate
		arguments int // number of predicate arguments.
	}

	// batchRow holds the changes of one node of a batch update.
	batchRow struct {
		node    *UpdateSpec
		ids     []any
		key     any // normalized IDs, see batchKey.
		cells   map[string]batchCell
		matched bool // matched the predicate before the batch.
		found   bool
	}

	batchCell struct {
		mode  int16
		value driver.Value
		delta driver.Value // added to the value in batchSetAdd.
	}

	// batchSource is a column of the row source.
	batchSource struct {
		name  string
		typ   string
		value func(*batchRow) any
	}
)

func (u *batchUpdater) nodes(ctx context.Context, drv dialect.Driver) error {
	if len(u.Nodes) == 0 {
		return nil
	}
	u.node = u.Nodes[0].Node
	if u.identifiers, _ = u.node.identity(); len(u.identifiers) == 0 {
		return fmt.Errorf("sqlgraph: batch update of table %q requires node ids", u.node.Table)
	}
	u.returning = slices.Clone(u.identifiers)
	u.positions = make(map[string]int)
	for index, column := range u.identifiers {
		u.positions[column] = index
	}
	u.rows = make(map[any]*batchRow, len(u.Nodes))
	rows := make([]*batchRow, len(u.Nodes))
	used := make(map[string]struct{})
	for index, spec := range u.Nodes {
		columns, ids := spec.Node.identity()
		switch {
		case spec.Node.Table != u.node.Table || spec.Node.Schema != u.node.Schema:
			return errors.New("sqlgraph: batch update nodes must share a table and schema")
		case !slices.Equal(columns, u.identifiers) || slices.Contains(ids, nil):
			return fmt.Errorf("sqlgraph: batch update node %d requires an id of columns %v", index, u.identifiers)
		case len(spec.Modifiers) > 0:
			return errors.New("sqlgraph: batch update does not support statement modifiers")
		case spec.Predicate != nil:
			return errors.New("sqlgraph: batch update does not support node predicates")
		case slices.ContainsFunc(spec.Edges.Add, isExternalEdge) || slices.ContainsFunc(spec.Edges.Clear, isExternalEdge):
			return errors.New("sqlgraph: batch update does not support edges stored in other tables")
		}
		key, err := batchKey(ids)
		if err != nil {
			return err
		}
		if u.rows[key] != nil {
			return fmt.Errorf("sqlgraph: batch update has duplicate id %v", key)
		}
		row := &batchRow{node: spec, ids: ids, key: key, cells: make(map[string]batchCell)}
		err = updateColumns(spec, EdgeSpecs(spec.Edges.Add).GroupRel(), EdgeSpecs(spec.Edges.Clear).GroupRel(),
			func(column string) { row.cells[column] = batchCell{mode: batchNull} },
			func(column string, value driver.Value) { row.cells[column] = batchCell{mode: batchSet, value: value} },
			func(column string, value driver.Value) {
				if cell := row.cells[column]; cell.mode == batchSet {
					row.cells[column] = batchCell{mode: batchSetAdd, value: cell.value, delta: value}
				} else {
					row.cells[column] = batchCell{mode: batchAdd, value: value}
				}
			},
		)
		if err != nil {
			return err
		}
		// Rows are found by their IDs, which must not change.
		for _, column := range u.identifiers {
			if _, ok := row.cells[column]; ok {
				return fmt.Errorf("sqlgraph: batch update node %d changes the id column %q", index, column)
			}
		}
		for column := range row.cells {
			used[column] = struct{}{}
		}
		for _, column := range spec.Node.Columns {
			if _, ok := u.positions[column]; !ok {
				u.positions[column] = len(u.returning)
				u.returning = append(u.returning, column)
			}
		}
		u.rows[key], rows[index] = row, row
	}
	u.columns = keys(used)
	if u.postgres {
		for _, column := range slices.Concat(u.identifiers, u.columns) {
			if u.PostgresTypes[column] == "" {
				return fmt.Errorf("sqlgraph: batch update has no PostgreSQL type for column %q", column)
			}
		}
	}
	u.sources = make([]batchSource, len(u.identifiers))
	for index, column := range u.identifiers {
		u.sources[index] = batchSource{name: fmt.Sprintf("id%d", index+1), typ: u.PostgresTypes[column], value: func(row *batchRow) any { return row.ids[index] }}
	}
	// Columns that change the same way in all rows are assigned directly, and the others through a CASE on their mode.
	u.modes = make([]uint8, len(u.columns))
	for index, column := range u.columns {
		for _, row := range rows {
			u.modes[index] |= 1 << row.cells[column].mode
		}
		if u.modes[index]&(1<<batchSet|1<<batchAdd|1<<batchSetAdd) != 0 {
			u.sources = append(u.sources, batchSource{name: fmt.Sprintf("value%d", index+1), typ: u.PostgresTypes[column], value: func(row *batchRow) any { return row.cells[column].value }})
		}
		if u.modes[index]&(1<<batchSetAdd) != 0 {
			u.sources = append(u.sources, batchSource{name: fmt.Sprintf("delta%d", index+1), typ: u.PostgresTypes[column], value: func(row *batchRow) any { return row.cells[column].delta }})
		}
		if bits.OnesCount8(u.modes[index]) > 1 {
			u.sources = append(u.sources, batchSource{name: fmt.Sprintf("mode%d", index+1), typ: "smallint", value: func(row *batchRow) any { return row.cells[column].mode }})
		}
	}
	selector := u.builder.Select().
		From(u.builder.Table(u.node.Table).Schema(u.node.Schema)).
		WithContext(ctx)
	if u.Predicate != nil {
		u.Predicate(selector)
	}
	// Rendering counts the predicate arguments, which every statement binds.
	_, arguments := selector.Query()
	if err := selector.Err(); err != nil {
		return err
	}
	u.predicate, u.arguments = selector.P(), len(arguments)
	// The row source is named differently from the target table.
	u.alias = "batch"
	if u.node.Table == u.alias {
		u.alias = "batch_rows"
	}
	size, err := u.chunkSize(u.sources)
	if err != nil {
		return err
	}
	// The row sources list their rows by ID, and the chunks cover consecutive ranges of IDs. Plans
	// that loop over the row source lock the rows in this order, unlike hash joins and table scans.
	sorted := slices.SortedStableFunc(slices.Values(rows), func(a, b *batchRow) int { return compareBatchKeys(a.key, b.key) })
	tx, err := drv.Tx(ctx)
	if err != nil {
		return err
	}
	// Each statement would evaluate the predicate against the writes of the previous ones. Instead,
	// the rows are matched once before the batch, and updated without the predicate. A batch that
	// changes no column writes nothing, and selects its rows with the predicate as UpdateNode does.
	statement := u.update
	switch {
	case len(u.columns) == 0:
		statement = u.read
	case u.predicate != nil && len(rows) > size:
		if err := u.match(ctx, tx, sorted); err != nil {
			return rollback(tx, err)
		}
		if err := u.notFound(rows, func(row *batchRow) bool { return row.matched }); err != nil {
			return rollback(tx, err)
		}
		u.predicate, u.arguments = nil, 0
	}
	for chunk := range slices.Chunk(sorted, size) {
		if err := statement(ctx, tx, chunk); err != nil {
			return rollback(tx, err)
		}
	}
	if err := u.notFound(rows, func(row *batchRow) bool { return row.found }); err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// unnest reports if the row source passes its columns as arrays to unnest.
// Arrays cannot be nested in arrays, so array columns use a VALUES list instead.
func (u *batchUpdater) unnest(sources []batchSource) bool {
	return u.postgres && !slices.ContainsFunc(sources, func(source batchSource) bool { return strings.HasSuffix(source.typ, "[]") })
}

// batchValuesRows caps the rows of VALUES, past about 32,400 of which SQLite scans the table per row.
const batchValuesRows = 16384

// chunkSize returns the number of rows that each statement binds in its row source. Every
// statement binds the predicate arguments and one parameter per source: its array in unnest,
// which holds any number of rows, or its value in each row of VALUES, which holds at most
// batchValuesRows rows.
func (u *batchUpdater) chunkSize(sources []batchSource) (int, error) {
	// The maximum number of parameters in one statement.
	limit := 32766
	if u.postgres {
		limit = 65535
	}
	if fixed := len(sources) + u.arguments; fixed > limit {
		return 0, fmt.Errorf("sqlgraph: batch update needs %d parameters per statement, more than the limit of %d", fixed, limit)
	}
	if u.unnest(sources) {
		return math.MaxInt, nil
	}
	return min((limit-u.arguments)/len(sources), batchValuesRows), nil
}

// rowSource renders the rows of one statement as an unnest of one array per source, or as
// a VALUES list in the returned prefix. PostgreSQL infers the types of the other rows of
// VALUES from the first one. On PostgreSQL, the row source also numbers its rows from 1 in
// the column "position", by which the returned rows are found.
func (u *batchUpdater) rowSource(sources []batchSource, chunk []*batchRow) (prefix []sql.Querier, from sql.Querier) {
	names := make([]string, len(sources), len(sources)+1)
	for index, source := range sources {
		names[index] = source.name
	}
	if u.postgres {
		names = append(names, "position")
	}
	if u.unnest(sources) {
		return nil, sql.ExprFunc(func(b *sql.Builder) {
			b.WriteString("unnest(")
			for index, source := range sources {
				values := make(sql.ArrayArgument, len(chunk))
				for position, row := range chunk {
					values[position] = source.value(row)
				}
				if index > 0 {
					b.Comma()
				}
				b.Arg(values).WriteString("::" + source.typ + "[]")
			}
			b.WriteString(") WITH ORDINALITY AS ").Ident(u.alias).Wrap(func(b *sql.Builder) { b.IdentComma(names...) })
		})
	}
	return []sql.Querier{sql.With(u.alias, names...).As(sql.ExprFunc(func(b *sql.Builder) {
		b.WriteString("VALUES ")
		for index, row := range chunk {
			if index > 0 {
				b.Comma()
			}
			b.Wrap(func(b *sql.Builder) {
				for position, source := range sources {
					if position > 0 {
						b.Comma()
					}
					b.Arg(source.value(row))
					if u.postgres && index == 0 {
						b.WriteString("::" + source.typ)
					}
				}
				if u.postgres {
					b.Comma().WriteString(strconv.Itoa(index + 1))
				}
			})
		}
	}))}, sql.ExprFunc(func(b *sql.Builder) { b.Ident(u.alias) })
}

// where matches the target rows to the rows of the row source, and to the predicate.
func (u *batchUpdater) where() *sql.Predicate {
	table, batch := u.builder.Table(u.node.Table), u.builder.Table(u.alias)
	predicates := make([]*sql.Predicate, 0, len(u.identifiers)+1)
	for index, column := range u.identifiers {
		predicates = append(predicates, sql.ColumnsEQ(table.C(column), batch.C(fmt.Sprintf("id%d", index+1))))
	}
	if u.predicate != nil {
		predicates = append(predicates, u.predicate)
	}
	return sql.And(predicates...)
}

// selectChunk selects the columns of the target rows that match one chunk of the row source and
// the predicate. On PostgreSQL, it also selects the position of each row in the row source.
func (u *batchUpdater) selectChunk(sources []batchSource, chunk []*batchRow, columns []string) *sql.Selector {
	selected := u.builder.Table(u.node.Table).Columns(columns...)
	if u.postgres {
		selected = append(selected, u.builder.Table(u.alias).C("position"))
	}
	prefix, from := u.rowSource(sources, chunk)
	return u.builder.Select(selected...).
		Prefix(prefix...).
		From(u.builder.Table(u.node.Table).Schema(u.node.Schema)).
		AppendFromExpr(from).
		Where(u.where())
}

// match marks the rows that match the predicate before the batch, locking them on PostgreSQL.
func (u *batchUpdater) match(ctx context.Context, tx dialect.ExecQuerier, rows []*batchRow) error {
	sources := u.sources[:len(u.identifiers)]
	size, err := u.chunkSize(sources)
	if err != nil {
		return err
	}
	for chunk := range slices.Chunk(rows, size) {
		selector := u.selectChunk(sources, chunk, u.identifiers)
		if u.postgres {
			selector.ForUpdate(sql.WithLockTables(u.node.Table))
		}
		result, err := QuerySelector(ctx, tx, selector)
		if err != nil {
			return err
		}
		err = u.scan(result, u.identifiers, chunk, func(row *batchRow, _ []any) error {
			row.matched = true
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// notFound returns a NotFoundError holding the IDs of the rows that were not found, if any.
func (u *batchUpdater) notFound(rows []*batchRow, found func(*batchRow) bool) error {
	var missing []driver.Value
	for _, row := range rows {
		switch {
		case found(row):
		case len(row.ids) == 1:
			missing = append(missing, row.ids[0])
		default:
			missing = append(missing, row.ids)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return &NotFoundError{table: u.node.Table, id: missing}
}

// update updates the rows of one chunk of the row source.
func (u *batchUpdater) update(ctx context.Context, tx dialect.ExecQuerier, chunk []*batchRow) error {
	table, batch := u.builder.Table(u.node.Table), u.builder.Table(u.alias)
	update := u.builder.Update(u.node.Table).Schema(u.node.Schema)
	for index, column := range u.columns {
		value, delta := batch.C(fmt.Sprintf("value%d", index+1)), batch.C(fmt.Sprintf("delta%d", index+1))
		has := func(mode int16) bool { return u.modes[index]&(1<<mode) != 0 }
		setAdd := func(b *sql.Builder) { b.WriteString("COALESCE(").Ident(value).WriteString(", 0) + ").Ident(delta) }
		switch {
		case bits.OnesCount8(u.modes[index]) > 1:
			update.Set(column, sql.ExprFunc(func(b *sql.Builder) {
				b.WriteString("CASE ").Ident(batch.C(fmt.Sprintf("mode%d", index+1)))
				if has(batchSet) {
					b.WriteString(fmt.Sprintf(" WHEN %d THEN ", batchSet)).Ident(value)
				}
				if has(batchNull) {
					b.WriteString(fmt.Sprintf(" WHEN %d THEN NULL", batchNull))
				}
				if has(batchAdd) {
					b.WriteString(fmt.Sprintf(" WHEN %d THEN COALESCE(", batchAdd)).Ident(table.C(column)).WriteString(", 0) + ").Ident(value)
				}
				if has(batchSetAdd) {
					b.WriteString(fmt.Sprintf(" WHEN %d THEN ", batchSetAdd))
					setAdd(b)
				}
				b.WriteString(" ELSE ").Ident(table.C(column)).WriteString(" END")
			}))
		case has(batchSet):
			update.Set(column, sql.Expr(value))
		case has(batchNull):
			update.SetNull(column)
		case has(batchAdd):
			update.Add(column, sql.Expr(value))
		case has(batchSetAdd):
			update.Set(column, sql.ExprFunc(setAdd))
		}
	}
	// PostgreSQL also resolves RETURNING columns against the row source.
	returning := u.returning
	if u.postgres {
		returning = append(table.Columns(u.returning...), batch.C("position"))
	}
	prefix, from := u.rowSource(u.sources, chunk)
	query, args, err := update.Prefix(prefix...).From(from).Where(u.where()).Returning(returning...).QueryErr()
	if err != nil {
		return err
	}
	var ids []any
	if len(u.identifiers) == 1 {
		ids = make([]any, len(chunk))
		for index, row := range chunk {
			ids[index] = row.ids[0]
		}
	}
	rows, err := tx.Query(dialect.WithStatement(ctx, rowWrite(dialect.QualifiedTable(u.node.Schema, u.node.Table), ids)), query, args)
	if err != nil {
		return err
	}
	return u.scan(rows, u.returning, chunk, u.assign)
}

// read selects the rows of one chunk of the row source in a batch that changes no column.
func (u *batchUpdater) read(ctx context.Context, tx dialect.ExecQuerier, chunk []*batchRow) error {
	selector := u.selectChunk(u.sources, chunk, u.returning)
	query, args := selector.Query()
	if err := selector.Err(); err != nil {
		return err
	}
	rows, err := tx.Query(dialect.WithStatement(ctx, u.node.readStatement(selector)), query, args)
	if err != nil {
		return err
	}
	return u.scan(rows, u.returning, chunk, u.assign)
}

// scan scans each returned row into the scan values of the given columns, which start with the
// ID columns, and visits it with the row of the chunk it belongs to. PostgreSQL may return an ID
// in another form than it was given, such as a canonical UUID, so its rows are found by their
// position in the row source, returned after the columns. SQLite cannot return columns of the
// row source, and returns IDs as given, so its rows are found by their IDs.
func (u *batchUpdater) scan(rows dialect.Rows, columns []string, chunk []*batchRow, visit func(*batchRow, []any) error) error {
	defer rows.Close()
	for rows.Next() {
		values, err := u.Nodes[0].ScanValues(columns)
		if err != nil {
			return err
		}
		for i, v := range values {
			if _, ok := v.(*sql.UnknownType); ok {
				values[i] = sql.ScanTypeOf(rows, i)
			}
		}
		var position int64
		destinations := values
		if u.postgres {
			destinations = append(destinations, &position)
		}
		if err := rows.Scan(destinations...); err != nil {
			return fmt.Errorf("failed scanning rows: %w", err)
		}
		var row *batchRow
		if u.postgres {
			row = chunk[position-1]
		} else {
			key, err := batchKey(values[:len(u.identifiers)])
			if err != nil {
				return err
			}
			if row = u.rows[key]; row == nil {
				return fmt.Errorf("sqlgraph: batch update returned an unexpected row with id %v", key)
			}
		}
		if err := visit(row, values); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

// assign assigns a returned row to its node, narrowed to the columns of the node.
func (u *batchUpdater) assign(row *batchRow, values []any) error {
	row.found = true
	columns := row.node.Node.Columns
	assigned := make([]any, len(columns))
	for index, column := range columns {
		assigned[index] = values[u.positions[column]]
	}
	return row.node.Assign(columns, assigned)
}

// batchKey normalizes the ID values of a node, or the scan destinations of a returned
// row, so that both compare equal. Composite IDs are keyed by their pair of values.
func batchKey(ids []any) (any, error) {
	var key [2]any
	for index, id := range ids {
		value, err := driver.DefaultParameterConverter.ConvertValue(id)
		if err != nil {
			return nil, fmt.Errorf("sqlgraph: batch update id: %w", err)
		}
		if bytes, ok := value.([]byte); ok {
			value = string(bytes)
		}
		key[index] = value
	}
	if len(ids) == 1 {
		return key[0], nil
	}
	return key, nil
}

// compareBatchKeys orders the keys of batchKey by their numbers, strings or times, and
// composite keys lexicographically. Keys of other or different types compare equal.
func compareBatchKeys(a, b any) int {
	switch a := a.(type) {
	case [2]any:
		b := b.([2]any)
		return cmp.Or(compareBatchKeys(a[0], b[0]), compareBatchKeys(a[1], b[1]))
	case int64:
		return compareAs(a, b)
	case float64:
		return compareAs(a, b)
	case string:
		return compareAs(a, b)
	case time.Time:
		if b, ok := b.(time.Time); ok {
			return a.Compare(b)
		}
	}
	return 0
}

// compareAs compares a to b if b has the type of a, and reports them equal otherwise.
func compareAs[T cmp.Ordered](a T, b any) int {
	if b, ok := b.(T); ok {
		return cmp.Compare(a, b)
	}
	return 0
}

type creator struct {
	graph
	*CreateSpec
}

func (c *creator) node(ctx context.Context, drv dialect.Driver) error {
	var (
		edges  = EdgeSpecs(c.Edges).GroupRel()
		insert = c.builder.Insert(c.Table).Schema(c.Schema).Default()
	)
	if err := c.setTableColumns(insert, edges); err != nil {
		return err
	}
	tx, err := c.mayTx(ctx, drv, edges)
	if err != nil {
		return err
	}
	if err := func() error {
		// In case the spec does not contain an ID field, we assume
		// we interact with an edge-schema with composite primary key.
		if c.ID == nil {
			return c.insert(ctx, insert)
		}
		if err := c.insert(ctx, insert); err != nil {
			return err
		}
		if err := c.addM2MEdges(ctx, []driver.Value{c.ID.Value}, edges[M2M]); err != nil {
			return err
		}
		return c.addFKEdges(ctx, []driver.Value{c.ID.Value}, append(edges[O2M], edges[O2O]...))
	}(); err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// mayTx opens a new transaction if the create operation spans across multiple statements.
func (c *creator) mayTx(ctx context.Context, drv dialect.Driver, edges map[Rel][]*EdgeSpec) (dialect.Tx, error) {
	if !hasExternalEdges(edges, nil) {
		return dialect.NopTx(drv), nil
	}
	tx, err := drv.Tx(ctx)
	if err != nil {
		return nil, err
	}
	c.tx = tx
	return tx, nil
}

// setTableColumns sets the table columns and foreign_keys used in insert.
func (c *creator) setTableColumns(insert *sql.InsertBuilder, edges map[Rel][]*EdgeSpec) error {
	err := setTableColumns(c.Fields, edges, func(column string, value driver.Value) {
		if c.Expressions[column] == nil {
			insert.Set(column, value)
		}
	})
	for _, column := range slices.Sorted(maps.Keys(c.Expressions)) {
		insert.Set(column, sql.ExprFunc(c.Expressions[column]))
	}
	return err
}

// insert a node to its table and sets its ID if it was not provided by the user.
func (c *creator) insert(ctx context.Context, insert *sql.InsertBuilder) error {
	c.ensureConflict(insert)
	if c.ID == nil {
		statement := bulkWrite(dialect.QualifiedTable(c.Schema, c.Table))
		if c.Returning != nil {
			query, args, err := insert.Returning(c.Returning.Columns...).QueryErr()
			if err != nil {
				return err
			}
			rows, err := c.tx.Query(dialect.WithStatement(ctx, statement), query, args)
			if err != nil {
				return err
			}
			count, err := c.Returning.scan(rows)
			if err == nil && count == 0 {
				return dialect.ErrNoRows
			}
			return err
		}
		query, args, err := insert.QueryErr()
		if err != nil {
			return err
		}
		_, err = c.tx.Exec(dialect.WithStatement(ctx, statement), query, args)
		return err
	}
	if c.ID.Value != nil && c.Expressions[c.ID.Column] == nil {
		insert.Set(c.ID.Column, c.ID.Value)
	}
	if c.Returning == nil {
		return c.insertID(ctx, insert.Returning(c.ID.Column))
	}
	query, args, err := insert.Returning(c.Returning.Columns...).QueryErr()
	if err != nil {
		return err
	}
	table := dialect.QualifiedTable(c.Schema, c.Table)
	// Keep bulk invalidation until every returned row has been identified.
	statement := bulkWrite(table)
	rows, err := c.tx.Query(dialect.WithStatement(ctx, statement), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return dialect.ErrNoRows
	}
	if err := c.Returning.Scan(rows); err != nil {
		return err
	}
	if c.ID.Value == nil {
		return errors.New("sqlgraph: Returning.Scan must assign the ID value")
	}
	// Publish the assigned ID before Close lets driver wrappers invalidate rows.
	statement.Rows = append(statement.Rows, dialect.RowRef{Table: table, ID: c.ID.Value})
	if rows.Next() {
		return errors.New("sqlgraph: create returned more than one row")
	}
	if err := rows.Err(); err != nil {
		return err
	}
	statement.Bulk = nil
	return nil
}

// ensureConflict ensures the ON CONFLICT is added to the insert statement.
func (c *creator) ensureConflict(insert *sql.InsertBuilder) {
	if opts := c.OnConflict; len(opts) > 0 {
		insert.OnConflict(opts...)
	}
}

type batchCreator struct {
	graph
	*BatchCreateSpec
}

func (c *batchCreator) nodes(ctx context.Context, drv dialect.Driver) error {
	if len(c.Nodes) == 0 {
		return nil
	}
	for index, node := range c.Nodes {
		if index > 0 && (node.Table != c.Nodes[0].Table || node.Schema != c.Nodes[0].Schema) {
			return errors.New("sqlgraph: batch nodes must share a table and schema")
		}
	}
	// Every node is inserted with the union of all columns; columns a node did
	// not set become NULL so that ON CONFLICT ... DO UPDATE with the new values
	// clears them, on both dialects and on both execution strategies below.
	columns := make(map[string]struct{})
	values := make([]map[string]driver.Value, len(c.Nodes))
	for i, node := range c.Nodes {
		values[i] = make(map[string]driver.Value)
		if node.ID != nil && node.ID.Value != nil {
			columns[node.ID.Column] = struct{}{}
			values[i][node.ID.Column] = node.ID.Value
		}
		err := setTableColumns(node.Fields, EdgeSpecs(node.Edges).GroupRel(), func(column string, value driver.Value) {
			columns[column] = struct{}{}
			values[i][column] = value
		})
		for column, render := range node.Expressions {
			columns[column] = struct{}{}
			values[i][column] = sql.ExprFunc(render)
		}
		if err != nil {
			return err
		}
	}
	if len(columns) == 0 && c.Nodes[0].ID != nil {
		columns[c.Nodes[0].ID.Column] = struct{}{}
	}
	sorted := keys(columns)
	// Unordered or skipped RETURNING rows cannot be correlated with input nodes.
	individual := !drv.Capabilities().MultiRowReturningOrdered || len(c.OnConflict) > 0
	for _, node := range c.Nodes {
		node.Skipped = false
		individual = individual || len(node.OnConflict) > 0 || node.ID == nil && len(node.Fields) == 0
	}
	if individual {
		tx, err := drv.Tx(ctx)
		if err != nil {
			return err
		}
		for index, node := range c.Nodes {
			spec := *node
			if len(c.OnConflict) > 0 {
				spec.OnConflict = c.OnConflict
			}
			creator := &creator{graph: graph{tx: tx, builder: c.builder}, CreateSpec: &spec}
			edges := EdgeSpecs(node.Edges).GroupRel()
			insert := c.builder.Insert(node.Table).Schema(node.Schema).Default()
			for _, column := range sorted {
				// The ID column is set by creator.insert when a value was provided.
				if node.ID != nil && column == node.ID.Column && node.Expressions[column] == nil {
					continue
				}
				insert.Set(column, values[index][column])
			}
			node.Skipped = false
			if err := creator.insert(ctx, insert); err != nil {
				if errors.Is(err, dialect.ErrNoRows) {
					node.Skipped = true
					continue
				}
				return rollback(tx, err)
			}
			if node.ID != nil {
				if err := creator.addM2MEdges(ctx, []driver.Value{node.ID.Value}, edges[M2M]); err != nil {
					return rollback(tx, err)
				}
				if err := creator.addFKEdges(ctx, []driver.Value{node.ID.Value}, append(edges[O2M], edges[O2O]...)); err != nil {
					return rollback(tx, err)
				}
			}
		}
		return tx.Commit()
	}
	// Keep each callback's exact column layout, including mixed callback/ID-only nodes.
	var groups [][]int
	for index, node := range c.Nodes {
		group := slices.IndexFunc(groups, func(indexes []int) bool {
			returning := c.Nodes[indexes[0]].Returning
			return returning == nil && node.Returning == nil || returning != nil && node.Returning != nil && slices.Equal(returning.Columns, node.Returning.Columns)
		})
		if group == -1 {
			groups = append(groups, []int{index})
		} else {
			groups[group] = append(groups[group], index)
		}
	}
	var tx dialect.Tx
	var err error
	if len(groups) > 1 {
		tx, err = drv.Tx(ctx)
	} else {
		tx, err = c.mayTx(ctx, drv)
	}
	if err != nil {
		return err
	}
	c.tx = tx
	if err := func() error {
		for _, indexes := range groups {
			nodes := make([]*CreateSpec, 0, len(indexes))
			insert := c.builder.Insert(c.Nodes[0].Table).Schema(c.Nodes[0].Schema).Default().Columns(sorted...)
			for _, index := range indexes {
				nodes = append(nodes, c.Nodes[index])
				arguments := make([]any, len(sorted))
				for position, column := range sorted {
					arguments[position] = values[index][column]
					if node := c.Nodes[index]; node.ID != nil && column == node.ID.Column && node.ID.Value == nil && node.Expressions[column] == nil {
						arguments[position] = sql.Default
					}
				}
				insert.Values(arguments...)
			}
			creator := &batchCreator{graph: c.graph, BatchCreateSpec: &BatchCreateSpec{Nodes: nodes}}
			if err := creator.batchInsert(ctx, tx, insert); err != nil {
				return fmt.Errorf("insert nodes to table %q: %w", c.Nodes[0].Table, err)
			}
		}
		if c.Nodes[0].ID == nil {
			return nil
		}
		if err := c.batchAddM2M(ctx, c.BatchCreateSpec); err != nil {
			return err
		}
		// FKs that exist in different tables can't be updated in batch (using the CASE
		// statement), because we rely on RowsAffected to check if the FK column is NULL.
		for _, node := range c.Nodes {
			edges := EdgeSpecs(node.Edges).GroupRel()
			if err := c.addFKEdges(ctx, []driver.Value{node.ID.Value}, append(edges[O2M], edges[O2O]...)); err != nil {
				return err
			}
		}
		return nil
	}(); err != nil {
		return rollback(tx, err)
	}
	return tx.Commit()
}

// mayTx opens a new transaction if the create operation spans across multiple statements.
func (c *batchCreator) mayTx(ctx context.Context, drv dialect.Driver) (dialect.Tx, error) {
	for _, node := range c.Nodes {
		for _, edge := range node.Edges {
			if isExternalEdge(edge) {
				return drv.Tx(ctx)
			}
		}
	}
	return dialect.NopTx(drv), nil
}

// batchInsert inserts a batch of nodes to their table and sets their ID if it was not provided by the user.
func (c *batchCreator) batchInsert(ctx context.Context, tx dialect.ExecQuerier, insert *sql.InsertBuilder) error {
	c.ensureConflict(insert)
	table := dialect.QualifiedTable(c.Nodes[0].Schema, c.Nodes[0].Table)
	// Keep bulk invalidation until every returned row has been identified.
	statement := bulkWrite(table)
	if c.Nodes[0].Returning == nil {
		if c.Nodes[0].ID != nil {
			return c.insertIDs(ctx, tx, insert.Returning(c.Nodes[0].ID.Column))
		}
		query, args, err := insert.QueryErr()
		if err != nil {
			return err
		}
		_, err = tx.Exec(dialect.WithStatement(ctx, statement), query, args)
		return err
	}
	query, args, err := insert.Returning(c.Nodes[0].Returning.Columns...).QueryErr()
	if err != nil {
		return err
	}
	rows, err := tx.Query(dialect.WithStatement(ctx, statement), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if count >= len(c.Nodes) {
			return errors.New("sqlgraph: too many returned rows")
		}
		node := c.Nodes[count]
		if err := node.Returning.Scan(rows); err != nil {
			return err
		}
		if node.ID != nil {
			if node.ID.Value == nil {
				return errors.New("sqlgraph: Returning.Scan must assign the ID value")
			}
			// Every assigned ID must be visible to driver wrappers before rows.Close.
			statement.Rows = append(statement.Rows, dialect.RowRef{Table: table, ID: node.ID.Value})
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(c.Nodes) {
		return fmt.Errorf("sqlgraph: expected %d returned rows, got %d", len(c.Nodes), count)
	}
	if c.Nodes[0].ID != nil {
		statement.Bulk = nil
	}
	return rows.Close()
}

// ensureConflict ensures the ON CONFLICT is added to the insert statement.
func (c *batchCreator) ensureConflict(insert *sql.InsertBuilder) {
	if opts := c.OnConflict; len(opts) > 0 {
		insert.OnConflict(opts...)
	}
}

// GroupRel groups edges by their relation type.
func (es EdgeSpecs) GroupRel() map[Rel][]*EdgeSpec {
	edges := make(map[Rel][]*EdgeSpec)
	for _, edge := range es {
		edges[edge.Rel] = append(edges[edge.Rel], edge)
	}
	return edges
}

// GroupTable groups edges by their table name.
func (es EdgeSpecs) GroupTable() map[string][]*EdgeSpec {
	edges := make(map[string][]*EdgeSpec)
	for _, edge := range es {
		edges[edge.Table] = append(edges[edge.Table], edge)
	}
	return edges
}

// FilterRel returns edges for the given relation type.
func (es EdgeSpecs) FilterRel(r Rel) EdgeSpecs {
	edges := make([]*EdgeSpec, 0, len(es))
	for _, edge := range es {
		if edge.Rel == r {
			edges = append(edges, edge)
		}
	}
	return edges
}

// The common operations shared between the different builders.
//
// M2M edges reside in join tables and require INSERT and DELETE
// queries for adding or removing edges respectively.
//
// O2M and non-inverse O2O edges also reside in external tables,
// but use UPDATE queries (fk = ?, fk = NULL).
type graph struct {
	tx      dialect.ExecQuerier
	builder *sql.DialectBuilder
}

func (g *graph) clearM2MEdges(ctx context.Context, ids []driver.Value, edges EdgeSpecs) error {
	// Remove all M2M edges from the same type at once.
	// The EdgeSpec is the same for all members in a group.
	tables := edges.GroupTable()
	for _, table := range edgeKeys(tables) {
		edges := tables[table]
		preds := make([]*sql.Predicate, 0, len(edges))
		for _, edge := range edges {
			fromC, toC := edge.Columns[0], edge.Columns[1]
			if edge.Inverse {
				fromC, toC = toC, fromC
			}
			// If there are no specific edges (to target-nodes) to remove,
			// clear all edges that go out (or come in) from the nodes.
			if len(edge.Target.Nodes) == 0 {
				preds = append(preds, matchID(fromC, ids))
				if edge.Bidi {
					preds = append(preds, matchID(toC, ids))
				}
			} else {
				pk1, pk2 := ids, edge.Target.Nodes
				preds = append(preds, matchIDs(fromC, pk1, toC, pk2))
				if edge.Bidi {
					preds = append(preds, matchIDs(toC, pk1, fromC, pk2))
				}
			}
		}
		deleter := g.builder.Delete(table).Where(sql.Or(preds...))
		if edges[0].Schema != "" {
			// If the Schema field was provided to the EdgeSpec (by the
			// generated code), it should be the same for all EdgeSpecs.
			deleter.Schema(edges[0].Schema)
		}
		query, args := deleter.Query()
		statement := bulkWrite(dialect.QualifiedTable(edges[0].Schema, table))
		if _, err := g.tx.Exec(dialect.WithStatement(ctx, statement), query, args); err != nil {
			return fmt.Errorf("remove m2m edge for table %s: %w", table, err)
		}
	}
	return nil
}

func (g *graph) addM2MEdges(ctx context.Context, ids []driver.Value, edges EdgeSpecs) error {
	// Insert all M2M edges from the same type at once.
	// The EdgeSpec is the same for all members in a group.
	tables := edges.GroupTable()
	for _, table := range edgeKeys(tables) {
		var (
			edges   = tables[table]
			columns = edges[0].Columns
			values  = make([]any, 0, len(edges[0].Target.Fields))
		)
		// Additional fields, such as edge-schema fields. Note, we use the first index,
		// because Ent generates the same spec fields for all edges from the same type.
		for _, f := range edges[0].Target.Fields {
			values = append(values, f.Value)
			columns = append(columns, f.Column)
		}
		insert := g.builder.Insert(table).Columns(columns...)
		if edges[0].Schema != "" {
			// If the Schema field was provided to the EdgeSpec (by the
			// generated code), it should be the same for all EdgeSpecs.
			insert.Schema(edges[0].Schema)
		}
		for _, edge := range edges {
			pk1, pk2 := ids, edge.Target.Nodes
			if edge.Inverse {
				pk1, pk2 = pk2, pk1
			}
			for _, pair := range product(pk1, pk2) {
				insert.Values(append([]any{pair[0], pair[1]}, values...)...)
				if edge.Bidi {
					insert.Values(append([]any{pair[1], pair[0]}, values...)...)
				}
			}
		}
		// Ignore conflicts only if edges do not contain extra fields, because these fields
		// can hold different values on different insertions (e.g. time.Now() or uuid.New()).
		if len(edges[0].Target.Fields) == 0 {
			insert.OnConflict(sql.DoNothing())
		}
		query, args := insert.Query()
		statement := bulkWrite(dialect.QualifiedTable(edges[0].Schema, table))
		if _, err := g.tx.Exec(dialect.WithStatement(ctx, statement), query, args); err != nil {
			return fmt.Errorf("add m2m edge for table %s: %w", table, err)
		}
	}
	return nil
}

func (g *graph) batchAddM2M(ctx context.Context, spec *BatchCreateSpec) error {
	tables := make(map[string]*sql.InsertBuilder)
	for _, node := range spec.Nodes {
		edges := EdgeSpecs(node.Edges).FilterRel(M2M)
		for name, edges := range edges.GroupTable() {
			if len(edges) != 1 {
				return fmt.Errorf("expect exactly 1 edge-spec per table, but got %d", len(edges))
			}
			edge := edges[0]
			table := dialect.QualifiedTable(edge.Schema, name)
			insert, ok := tables[table]
			if !ok {
				columns := edge.Columns
				// Additional fields, such as edge-schema fields.
				for _, f := range edge.Target.Fields {
					columns = append(columns, f.Column)
				}
				insert = g.builder.Insert(name).Columns(columns...)
				if edge.Schema != "" {
					// If the Schema field was provided to the EdgeSpec (by the
					// generated code), it should be the same for all EdgeSpecs.
					insert.Schema(edge.Schema)
				}
				// Ignore conflicts only if edges do not contain extra fields, because these fields
				// can hold different values on different insertions (e.g. time.Now() or uuid.New()).
				if len(edge.Target.Fields) == 0 {
					insert.OnConflict(sql.DoNothing())
				}
			}
			tables[table] = insert
			pk1, pk2 := []driver.Value{node.ID.Value}, edge.Target.Nodes
			if edge.Inverse {
				pk1, pk2 = pk2, pk1
			}
			for _, pair := range product(pk1, pk2) {
				insert.Values(append([]any{pair[0], pair[1]}, edge.Target.FieldValues()...)...)
				if edge.Bidi {
					insert.Values(append([]any{pair[1], pair[0]}, edge.Target.FieldValues()...)...)
				}
			}
		}
	}
	for _, table := range insertKeys(tables) {
		query, args := tables[table].Query()
		statement := bulkWrite(table)
		if _, err := g.tx.Exec(dialect.WithStatement(ctx, statement), query, args); err != nil {
			return fmt.Errorf("add m2m edge for table %s: %w", table, err)
		}
	}
	return nil
}

func (g *graph) clearFKEdges(ctx context.Context, ids []driver.Value, edges []*EdgeSpec) error {
	for _, edge := range edges {
		if edge.Rel == O2O && edge.Inverse {
			continue
		}
		// O2O relations can be cleared without
		// passing the target ids.
		pred := matchID(edge.Columns[0], ids)
		if nodes := edge.Target.Nodes; len(nodes) > 0 {
			pred = matchIDs(edge.Target.IDSpec.Column, edge.Target.Nodes, edge.Columns[0], ids)
		}
		query, args := g.builder.Update(edge.Table).
			Schema(edge.Schema).
			SetNull(edge.Columns[0]).
			Where(pred).
			Query()
		if _, err := g.tx.Exec(dialect.WithStatement(ctx, edge.writeStatement()), query, args); err != nil {
			return fmt.Errorf("add %s edge for table %s: %w", edge.Rel, edge.Table, err)
		}
	}
	return nil
}

func (g *graph) addFKEdges(ctx context.Context, ids []driver.Value, edges []*EdgeSpec) error {
	id := ids[0]
	if len(ids) > 1 && len(edges) != 0 {
		// O2M and non-inverse O2O edges are defined by a FK in the "other"
		// table. Therefore, ids[i+1] will override ids[i] which is invalid.
		return fmt.Errorf("unable to link FK edge to more than 1 node: %v", ids)
	}
	for _, edge := range edges {
		if edge.Rel == O2O && edge.Inverse {
			continue
		}
		p := sql.EQ(edge.Target.IDSpec.Column, edge.Target.Nodes[0])
		// Use "IN" predicate instead of list of "OR"
		// in case of more than on nodes to connect.
		if len(edge.Target.Nodes) > 1 {
			p = sql.InValues(edge.Target.IDSpec.Column, edge.Target.Nodes...)
		}
		query, args := g.builder.Update(edge.Table).
			Schema(edge.Schema).
			Set(edge.Columns[0], id).
			Where(sql.And(p, sql.IsNull(edge.Columns[0]))).
			Query()
		res, err := g.tx.Exec(dialect.WithStatement(ctx, edge.writeStatement()), query, args)
		if err != nil {
			return fmt.Errorf("add %s edge for table %s: %w", edge.Rel, edge.Table, err)
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		// Setting the FK value of the "other" table without clearing it before, is not allowed.
		// Including no-op (same id), because we rely on "affected" to determine if the FK set.
		if ids := edge.Target.Nodes; int(affected) < len(ids) {
			return &ConstraintError{msg: fmt.Sprintf("one of %v is already connected to a different %s", ids, edge.Columns[0])}
		}
	}
	return nil
}

func hasExternalEdges(addEdges, clearEdges map[Rel][]*EdgeSpec) bool {
	// M2M edges reside in a join-table, and O2M edges reside
	// in the M2O table (the entity that holds the FK).
	if len(clearEdges[M2M]) > 0 || len(addEdges[M2M]) > 0 ||
		len(clearEdges[O2M]) > 0 || len(addEdges[O2M]) > 0 {
		return true
	}
	for _, edges := range [][]*EdgeSpec{clearEdges[O2O], addEdges[O2O]} {
		for _, e := range edges {
			if !e.Inverse {
				return true
			}
		}
	}
	return false
}

// isExternalEdge reports if the given edge requires an UPDATE
// or an INSERT to other table.
func isExternalEdge(e *EdgeSpec) bool {
	return e.Rel == M2M || e.Rel == O2M || e.Rel == O2O && !e.Inverse
}

// setTableColumns is shared between updater and creator.
func setTableColumns(fields []*FieldSpec, edges map[Rel][]*EdgeSpec, set func(string, driver.Value)) (err error) {
	for _, fi := range fields {
		value := fi.Value
		if fi.Type == field.TypeJSON {
			buf, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("marshal value for column %s: %w", fi.Column, err)
			}
			// If the underlying driver does not support JSON types,
			// driver.DefaultParameterConverter will convert it to uint8.
			value = json.RawMessage(buf)
		}
		set(fi.Column, value)
	}
	for _, e := range edges[M2O] {
		set(e.Columns[0], e.Target.Nodes[0])
	}
	for _, e := range edges[O2O] {
		if e.Inverse || e.Bidi {
			set(e.Columns[0], e.Target.Nodes[0])
		}
	}
	return nil
}

// insertID invokes the insert query on the transaction and returns the returned ID.
func (c *creator) insertID(ctx context.Context, insert *sql.InsertBuilder) error {
	query, args, err := insert.QueryErr()
	if err != nil {
		return err
	}
	table := dialect.QualifiedTable(c.Schema, c.Table)
	// Keep bulk invalidation until every returned row has been identified.
	statement := bulkWrite(table)
	rows, err := c.tx.Query(dialect.WithStatement(ctx, statement), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	switch _, ok := c.ID.Value.(field.ValueScanner); {
	case ok:
		if err := sql.ScanOne(rows, c.ID.Value); err != nil {
			return err
		}
	case c.ID.Type.Numeric():
		id, err := sql.ScanInt64(rows)
		if err != nil {
			return err
		}
		c.ID.Value = id
	default:
		if err := sql.ScanOne(rows, &c.ID.Value); err != nil {
			return err
		}
	}
	// The deferred Close must see the assigned ID before wrappers invalidate rows.
	statement.Rows = append(statement.Rows, dialect.RowRef{Table: table, ID: c.ID.Value})
	statement.Bulk = nil
	return nil
}

// insertIDs invokes the batch insert query on the transaction and returns the returned ID of all entities.
func (c *batchCreator) insertIDs(ctx context.Context, tx dialect.ExecQuerier, insert *sql.InsertBuilder) error {
	query, args, err := insert.QueryErr()
	if err != nil {
		return err
	}
	table := dialect.QualifiedTable(c.Nodes[0].Schema, c.Nodes[0].Table)
	// Keep bulk invalidation until every returned row has been identified.
	statement := bulkWrite(table)
	rows, err := tx.Query(dialect.WithStatement(ctx, statement), query, args)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		if count >= len(c.Nodes) {
			return errors.New("sqlgraph: too many returned IDs")
		}
		node := c.Nodes[count]
		count++
		switch _, ok := node.ID.Value.(field.ValueScanner); {
		case ok:
			if err := rows.Scan(node.ID.Value); err != nil {
				return err
			}
		case node.ID.Type.Numeric():
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			node.ID.Value = id
		default:
			if err := rows.Scan(&node.ID.Value); err != nil {
				return err
			}
		}
		// Publish each assigned ID before the explicit or deferred rows.Close.
		statement.Rows = append(statement.Rows, dialect.RowRef{Table: table, ID: node.ID.Value})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(c.Nodes) {
		return fmt.Errorf("sqlgraph: expected %d returned IDs, got %d; conflict actions must return every row", len(c.Nodes), count)
	}
	statement.Bulk = nil
	return rows.Close()
}

// rollback calls to tx.Rollback and wraps the given error with the rollback error if occurred.
func rollback(tx dialect.Tx, err error) error {
	if rerr := tx.Rollback(); rerr != nil {
		err = fmt.Errorf("%w: %v", err, rerr)
	}
	return err
}

func edgeKeys(m map[string][]*EdgeSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func insertKeys(m map[string]*sql.InsertBuilder) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func keys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func matchID(column string, pk []driver.Value) *sql.Predicate {
	if len(pk) > 1 {
		return sql.InValues(column, pk...)
	}
	return sql.EQ(column, pk[0])
}

func matchIDs(column1 string, pk1 []driver.Value, column2 string, pk2 []driver.Value) *sql.Predicate {
	p := matchID(column1, pk1)
	if len(pk2) > 1 {
		// Use "IN" predicate instead of list of "OR"
		// in case of more than on nodes to connect.
		return sql.And(p, sql.InValues(column2, pk2...))
	}
	return sql.And(p, sql.EQ(column2, pk2[0]))
}

// cartesian product of 2 id sets.
func product(a, b []driver.Value) [][2]driver.Value {
	c := make([][2]driver.Value, 0, len(a)*len(b))
	for i := range a {
		for j := range b {
			c = append(c, [2]driver.Value{a[i], b[j]})
		}
	}
	return c
}
