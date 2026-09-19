// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package dialect

import "context"

type StatementKind uint8

const (
	StatementRead StatementKind = iota + 1
	StatementWrite
)

type RowRef struct {
	Table string
	ID    any
}

// Statement carries the structural metadata sqlgraph knows about a statement
// it is about to execute. Driver wrappers (caches, tracers) read it from the
// context. It travels in the context rather than through the Driver interface
// so that intermediate wrappers unaware of it cannot drop it.
//
// Table names are schema-qualified ("schema.table") when a schema is set.
type Statement struct {
	Kind   StatementKind
	Tables []string
	Lock   bool
	// IDColumn names the primary key column of the queried node when the
	// statement selects it. Empty otherwise.
	IDColumn string
	// PointRead reports that the WHERE clause is a conjunction that pins the
	// result to specific rows of the node table (Tables[0]) by primary key or
	// unique columns. Rows in other tables may still affect the result.
	PointRead bool
	// RequestedIDs holds the primary key values a point read asked for. A
	// cache uses them to invalidate negative results when such rows appear.
	RequestedIDs []any
	// Rows lists the rows a write touched. sqlgraph fills it after the
	// statement executes, once RETURNING has been scanned, so readers must
	// consult it only after the rows are closed or Exec returned.
	Rows []RowRef
	// Bulk lists tables a write touched without identifying the rows.
	Bulk []string
}

type statementKey struct{}

func WithStatement(ctx context.Context, statement *Statement) context.Context {
	return context.WithValue(ctx, statementKey{}, statement)
}

func StatementFrom(ctx context.Context) *Statement {
	statement, _ := ctx.Value(statementKey{}).(*Statement)
	return statement
}

// QualifiedTable returns the schema-qualified table name used in Statement.
func QualifiedTable(schema, table string) string {
	if schema == "" {
		return table
	}
	return schema + "." + table
}
