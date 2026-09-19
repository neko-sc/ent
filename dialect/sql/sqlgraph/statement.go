// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package sqlgraph

import (
	"context"
	"slices"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
)

func QuerySelector(ctx context.Context, driver dialect.Driver, selector *sql.Selector) (dialect.Rows, error) {
	query, arguments := selector.Query()
	if err := selector.Err(); err != nil {
		return nil, err
	}
	return driver.Query(dialect.WithStatement(ctx, readStatement(selector)), query, arguments)
}

// bulkWrite describes a write to one already-qualified table whose affected rows
// are not identified, so readers must invalidate the table as a whole.
func bulkWrite(table string) *dialect.Statement {
	return &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{table}, Bulk: []string{table}}
}

func readStatement(selector *sql.Selector, tables ...string) *dialect.Statement {
	statement := &dialect.Statement{
		Kind: dialect.StatementRead,
		// NodeSpec supplies the node table first, even for traversals.
		Tables: tables,
		Lock:   selector.Locked(),
	}
	for _, table := range selector.Tables() {
		if !slices.Contains(statement.Tables, table) {
			statement.Tables = append(statement.Tables, table)
		}
	}
	return statement
}

func (n *NodeSpec) readStatement(selector *sql.Selector) *dialect.Statement {
	statement := readStatement(selector, dialect.QualifiedTable(n.Schema, n.Table))
	if n.ID != nil {
		for _, column := range selector.SelectedColumns() {
			if matchesColumn(selector, column, n.ID.Column) {
				statement.IDColumn = n.ID.Column
				break
			}
		}
	}
	// Other rows of the node table can affect self-joins and self-referencing subqueries.
	if !selector.Conjunctive() || selector.TableReferences()[statement.Tables[0]] > 1 {
		return statement
	}
	if n.ID != nil {
		for _, equality := range selector.Equalities() {
			if matchesColumn(selector, equality.Column, n.ID.Column) {
				statement.PointRead = true
				statement.RequestedIDs = append(statement.RequestedIDs, equality.Values...)
			}
		}
		if statement.PointRead {
			return statement
		}
	}
	unique := n.Unique
	if len(n.CompositeID) > 0 {
		columns := make([]string, len(n.CompositeID))
		for index, column := range n.CompositeID {
			columns[index] = column.Column
		}
		unique = append(slices.Clone(unique), columns)
	}
	equalities := selector.Equalities()
	// Unlike primary keys, absent unique values have no RequestedIDs to invalidate,
	// so only a single-valued equality pins a unique column.
	pinned := func(column string) bool {
		return slices.ContainsFunc(equalities, func(equality sql.Equality) bool {
			return len(equality.Values) == 1 && matchesColumn(selector, equality.Column, column)
		})
	}
	for _, columns := range unique {
		if len(columns) > 0 && !slices.ContainsFunc(columns, func(column string) bool { return !pinned(column) }) {
			statement.PointRead = true
			break
		}
	}
	return statement
}

// rowWrite describes a write to the identified rows of one already-qualified
// table, falling back to bulk invalidation when no row was identified.
func rowWrite[T any](table string, ids []T) *dialect.Statement {
	if len(ids) == 0 {
		return bulkWrite(table)
	}
	statement := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{table}}
	for _, id := range ids {
		statement.Rows = append(statement.Rows, dialect.RowRef{Table: table, ID: id})
	}
	return statement
}

func (n *NodeSpec) writeStatement(predicate interface {
	Equalities() []sql.Equality
	Conjunctive() bool
	Dialect() string
}) *dialect.Statement {
	var ids []any
	switch {
	case n.ID == nil:
	case n.ID.Value != nil:
		ids = []any{n.ID.Value}
	case predicate.Conjunctive():
		selector := sql.Dialect(predicate.Dialect()).Select().From(sql.Table(n.Table).Schema(n.Schema))
		for _, equality := range predicate.Equalities() {
			if matchesColumn(selector, equality.Column, n.ID.Column) {
				ids = append(ids, equality.Values...)
			}
		}
	}
	return rowWrite(dialect.QualifiedTable(n.Schema, n.Table), ids)
}

func (e *EdgeSpec) writeStatement() *dialect.Statement {
	return rowWrite(dialect.QualifiedTable(e.Schema, e.Table), e.Target.Nodes)
}

func matchesColumn(selector *sql.Selector, column, name string) bool {
	return column == name || column == selector.Quote(name) || selector.Table() != nil && column == selector.C(name)
}
