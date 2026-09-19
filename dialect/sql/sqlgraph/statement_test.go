// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package sqlgraph

import (
	"context"
	databaseDriver "database/sql/driver"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/schema/field"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type capturedStatement struct {
	statement *dialect.Statement
	query     string
	closed    bool
	rows      []dialect.RowRef
	bulk      []string
}

type statementDriver struct {
	dialect.Driver
	calls []*capturedStatement
}

func (d *statementDriver) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	return d.query(ctx, d.Driver, query, arguments)
}

func (d *statementDriver) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	d.calls = append(d.calls, &capturedStatement{statement: dialect.StatementFrom(ctx), query: query})
	return d.Driver.Exec(ctx, query, arguments)
}

func (d *statementDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	transaction, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	return &statementTransaction{Tx: transaction, driver: d}, nil
}

func (d *statementDriver) query(ctx context.Context, executor dialect.ExecQuerier, query string, arguments []any) (dialect.Rows, error) {
	call := &capturedStatement{statement: dialect.StatementFrom(ctx), query: query}
	d.calls = append(d.calls, call)
	rows, err := executor.Query(ctx, query, arguments)
	if err != nil {
		return nil, err
	}
	return &statementRows{Rows: rows, call: call}, nil
}

type statementTransaction struct {
	dialect.Tx
	driver *statementDriver
}

func (t *statementTransaction) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	return t.driver.query(ctx, t.Tx, query, arguments)
}

func (t *statementTransaction) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	t.driver.calls = append(t.driver.calls, &capturedStatement{statement: dialect.StatementFrom(ctx), query: query})
	return t.Tx.Exec(ctx, query, arguments)
}

type statementRows struct {
	dialect.Rows
	call *capturedStatement
}

func (r *statementRows) Close() error {
	if !r.call.closed {
		r.call.closed = true
		if r.call.statement != nil {
			r.call.rows = slices.Clone(r.call.statement.Rows)
			r.call.bulk = slices.Clone(r.call.statement.Bulk)
		}
	}
	return r.Rows.Close()
}

func newStatementDriver(t *testing.T, name dialect.Dialect) (*statementDriver, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	driver := &statementDriver{Driver: sql.OpenDB(name, database)}
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		statements := make(map[*dialect.Statement]bool)
		for _, call := range driver.calls {
			require.NotNil(t, call.statement, call.query)
			require.False(t, statements[call.statement], "statement reused for %s", call.query)
			statements[call.statement] = true
		}
		database.Close()
	})
	return driver, mock
}

func TestQueryStatements(t *testing.T) {
	for _, name := range []dialect.Dialect{dialect.Postgres, dialect.SQLite} {
		t.Run(string(name), func(t *testing.T) {
			for _, test := range []struct {
				name     string
				modify   func(*QuerySpec)
				tables   []string
				point    bool
				ids      []any
				idColumn string
				lock     bool
			}{
				{name: "list", tables: []string{"users"}, idColumn: "id"},
				{name: "equal", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.EQ(selector.C("id"), 1)) }
				}, tables: []string{"users"}, point: true, ids: []any{1}, idColumn: "id"},
				{name: "in", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.In(selector.C("id"), 1, 2)) }
				}, tables: []string{"users"}, point: true, ids: []any{1, 2}, idColumn: "id"},
				{name: "or", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.Or(sql.EQ(selector.C("id"), 1), sql.EQ(selector.C("name"), "a")))
					}
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "neighbors", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						HasNeighbors(selector, NewStep(From("users", "id"), To("pets", "id"), Edge(O2M, false, "pets", "owner_id")))
					}
				}, tables: []string{"users", "pets"}, idColumn: "id"},
				{name: "neighbors with", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						HasNeighborsWith(selector, NewStep(From("users", "id"), To("pets", "id"), Edge(M2M, false, "user_pets", "user_id", "pet_id")), func(neighbors *sql.Selector) { neighbors.Where(sql.EQ(neighbors.C("id"), 1)) })
					}
				}, tables: []string{"users", "user_pets", "pets"}, idColumn: "id"},
				{name: "lock", modify: func(spec *QuerySpec) {
					spec.Modifiers = []func(*sql.Selector){func(selector *sql.Selector) { selector.ForUpdate() }}
				}, tables: []string{"users"}, idColumn: "id", lock: true},
				{name: "schema", modify: func(spec *QuerySpec) { spec.Node.Schema = "tenant" }, tables: []string{"tenant.users"}, idColumn: "id"},
				{name: "projection", modify: func(spec *QuerySpec) {
					spec.Modifiers = []func(*sql.Selector){func(selector *sql.Selector) { selector.Select(selector.C("name")) }}
				}, tables: []string{"users"}},
				{name: "unique", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"tenant_id", "name"}}
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.And(sql.EQ("tenant_id", 1), sql.EQ(selector.Quote("name"), "a")))
					}
				}, tables: []string{"users"}, point: true, idColumn: "id"},
				{name: "unique equality", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"email"}}
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.EQ("email", "a")) }
				}, tables: []string{"users"}, point: true, idColumn: "id"},
				{name: "unique single in", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"email"}}
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.In("email", "a")) }
				}, tables: []string{"users"}, point: true, idColumn: "id"},
				{name: "unique multiple in", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"email"}}
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.In("email", "a", "b")) }
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "composite unique multiple in", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"tenant_id", "email"}}
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.And(sql.EQ("tenant_id", 1), sql.In("email", "a", "b")))
					}
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "self join", modify: func(spec *QuerySpec) {
					spec.Modifiers = []func(*sql.Selector){func(selector *sql.Selector) {
						friends := sql.Dialect(name).Table("users").As("friends")
						selector.Join(friends).On(selector.C("friend_id"), friends.C("id")).Where(sql.EQ(selector.C("id"), 1))
					}}
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "self subquery", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.EQ(selector.C("id"), 1)).Where(sql.In(selector.C("friend_id"), sql.Select("id").From(sql.Table("users"))))
					}
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "self neighbors with", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.EQ(selector.C("id"), 1))
						HasNeighborsWith(selector, NewStep(From("users", "id"), To("users", "id"), Edge(M2M, false, "user_friends", "user_id", "friend_id")), func(neighbors *sql.Selector) { neighbors.Where(sql.EQ(neighbors.C("name"), "a")) })
					}
				}, tables: []string{"users", "user_friends"}, idColumn: "id"},
				{name: "other table subquery", modify: func(spec *QuerySpec) {
					spec.Predicate = func(selector *sql.Selector) {
						selector.Where(sql.EQ(selector.C("id"), 1)).Where(sql.In(selector.C("pet_id"), sql.Select("id").From(sql.Table("pets"))))
					}
				}, tables: []string{"users", "pets"}, point: true, ids: []any{1}, idColumn: "id"},
				{name: "joined primary key", modify: func(spec *QuerySpec) {
					spec.Modifiers = []func(*sql.Selector){func(selector *sql.Selector) {
						pets := sql.Dialect(name).Table("pets").As("companions")
						selector.Join(pets).On(selector.C("id"), pets.C("owner_id")).Where(sql.EQ(pets.C("id"), 1))
					}}
				}, tables: []string{"users", "pets"}, idColumn: "id"},
				{name: "joined unique column", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"name"}}
					spec.Modifiers = []func(*sql.Selector){func(selector *sql.Selector) {
						pets := sql.Dialect(name).Table("pets").As("companions")
						selector.Join(pets).On(selector.C("id"), pets.C("owner_id")).Where(sql.EQ(pets.C("name"), "a"))
					}}
				}, tables: []string{"users", "pets"}, idColumn: "id"},
				{name: "incomplete unique", modify: func(spec *QuerySpec) {
					spec.Node.Unique = [][]string{{"tenant_id", "name"}}
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.EQ("name", "a")) }
				}, tables: []string{"users"}, idColumn: "id"},
				{name: "composite", modify: func(spec *QuerySpec) {
					spec.Node.ID = nil
					spec.Node.CompositeID = []*FieldSpec{NewFieldSpec("tenant_id", field.TypeInt), NewFieldSpec("name", field.TypeString)}
					spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.And(sql.EQ("tenant_id", 1), sql.EQ("name", "a"))) }
				}, tables: []string{"users"}, point: true},
			} {
				t.Run(test.name, func(t *testing.T) {
					if name == dialect.SQLite && test.lock {
						t.Skip("SQLite does not support locking reads")
					}
					driver, mock := newStatementDriver(t, name)
					spec := NewQuerySpec("users", []string{"id", "name"}, NewFieldSpec("id", field.TypeInt))
					if test.modify != nil {
						test.modify(spec)
					}
					mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id", "name"}))
					require.NoError(t, QueryNodes(t.Context(), driver, spec))
					require.Len(t, driver.calls, 1)
					require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: test.tables, PointRead: test.point, RequestedIDs: test.ids, IDColumn: test.idColumn, Lock: test.lock}, driver.calls[0].statement)
					require.True(t, driver.calls[0].closed)
					require.Contains(t, driver.calls[0].query, "SELECT")
				})
			}
		})
	}
}

func TestQuerySelector(t *testing.T) {
	for _, name := range []dialect.Dialect{dialect.Postgres, dialect.SQLite} {
		t.Run(string(name), func(t *testing.T) {
			driver, mock := newStatementDriver(t, name)
			builder := sql.Dialect(name)
			users := builder.Table("users").Schema("tenant")
			pets := builder.Table("pets").Schema("tenant")
			selector := builder.Select(users.C("id")).From(users).
				Join(pets).On(users.C("id"), pets.C("owner_id")).
				Where(sql.EQ(users.C("id"), 1))
			if name == dialect.Postgres {
				selector.ForUpdate()
			}
			mock.ExpectQuery("SELECT").WithArgs(1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1)).RowsWillBeClosed()
			rows, err := QuerySelector(t.Context(), driver, selector)
			require.NoError(t, err)
			require.True(t, rows.Next())
			var id int
			require.NoError(t, rows.Scan(&id))
			require.Equal(t, 1, id)
			require.NoError(t, rows.Close())
			require.Len(t, driver.calls, 1)
			require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"tenant.users", "tenant.pets"}, Lock: name == dialect.Postgres}, driver.calls[0].statement)
		})
	}
	t.Run("render error", func(t *testing.T) {
		driver, _ := newStatementDriver(t, dialect.SQLite)
		rows, err := QuerySelector(t.Context(), driver, sql.Dialect(dialect.SQLite).Select("id").From(sql.Table("users")).ForUpdate())
		require.Error(t, err)
		require.Nil(t, rows)
		require.Empty(t, driver.calls)
	})
	t.Run("driver error", func(t *testing.T) {
		driver, mock := newStatementDriver(t, dialect.Postgres)
		mock.ExpectQuery("SELECT").WillReturnError(context.Canceled)
		rows, err := QuerySelector(t.Context(), driver, sql.Dialect(dialect.Postgres).Select("COUNT(*)").From(sql.Table("users")))
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, rows)
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}}, driver.calls[0].statement)
	})
}

func TestCountStatement(t *testing.T) {
	driver, mock := newStatementDriver(t, dialect.Postgres)
	spec := NewQuerySpec("users", nil, NewFieldSpec("id", field.TypeInt))
	spec.Predicate = func(selector *sql.Selector) { selector.Where(sql.EQ(selector.C("id"), 1)) }
	mock.ExpectQuery("SELECT COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	count, err := CountNodes(t.Context(), driver, spec)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}}, driver.calls[0].statement)
}

func TestCreateStatements(t *testing.T) {
	for _, edges := range []bool{false, true} {
		for _, returning := range []bool{false, true} {
			t.Run(fmt.Sprintf("edges=%t/returning=%t", edges, returning), func(t *testing.T) {
				driver, mock := newStatementDriver(t, dialect.Postgres)
				spec := NewCreateSpec("users", NewFieldSpec("id", field.TypeInt))
				spec.Schema = "tenant"
				if returning {
					spec.Returning = &Returning{Columns: []string{"id"}, Scan: func(rows dialect.Rows) error { return rows.Scan(&spec.ID.Value) }}
				}
				if edges {
					spec.Edges = []*EdgeSpec{{Rel: M2M, Table: "user_groups", Schema: "tenant", Columns: []string{"user_id", "group_id"}, Target: &EdgeTarget{Nodes: []databaseDriver.Value{2}}}}
					mock.ExpectBegin()
				}
				mock.ExpectQuery(escape(`INSERT INTO "tenant"."users" DEFAULT VALUES RETURNING "id"`)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10))
				if edges {
					mock.ExpectExec(escape(`INSERT INTO "tenant"."user_groups" ("user_id", "group_id") VALUES ($1, $2) ON CONFLICT DO NOTHING`)).WithArgs(10, 2).WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectCommit()
				}
				require.NoError(t, CreateNode(t.Context(), driver, spec))
				require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"tenant.users"}, Rows: []dialect.RowRef{{Table: "tenant.users", ID: int64(10)}}}, driver.calls[0].statement)
				require.Equal(t, driver.calls[0].statement.Rows, driver.calls[0].rows)
				require.True(t, driver.calls[0].closed)
				if edges {
					require.Len(t, driver.calls, 2)
					require.NotSame(t, driver.calls[0].statement, driver.calls[1].statement)
					require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"tenant.user_groups"}, Bulk: []string{"tenant.user_groups"}}, driver.calls[1].statement)
				}
			})
		}
	}
}

func TestMutationStatements(t *testing.T) {
	for _, returning := range []bool{false, true} {
		t.Run(fmt.Sprintf("returning=%t", returning), func(t *testing.T) {
			t.Run("update node targets a single row", func(t *testing.T) {
				driver, mock := newStatementDriver(t, dialect.Postgres)
				spec := NewUpdateSpec("users", []string{"id"}, &FieldSpec{Column: "id", Type: field.TypeInt, Value: 1})
				spec.SetField("name", field.TypeString, "a")
				mock.ExpectBegin()
				if returning {
					spec.ScanValues = func([]string) ([]any, error) { return []any{new(int64)}, nil }
					spec.Assign = func([]string, []any) error { return nil }
					mock.ExpectQuery("UPDATE").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
				} else {
					mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
				}
				mock.ExpectCommit()
				require.NoError(t, UpdateNode(t.Context(), driver, spec))
				require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}, Rows: []dialect.RowRef{{Table: "users", ID: 1}}}, driver.calls[0].statement)
			})

			for _, point := range []bool{false, true} {
				for _, deletion := range []bool{false, true} {
					t.Run(fmt.Sprintf("point=%t/delete=%t", point, deletion), func(t *testing.T) {
						driver, mock := newStatementDriver(t, dialect.Postgres)
						node := &NodeSpec{Table: "users", Schema: "tenant", ID: NewFieldSpec("id", field.TypeInt)}
						predicate := func(selector *sql.Selector) { selector.Where(sql.GT(selector.C("age"), 18)) }
						// A range predicate can touch any row, so the statement
						// falls back to a table-wide bulk reference.
						expected := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"tenant.users"}, Bulk: []string{"tenant.users"}}
						if point {
							predicate = func(selector *sql.Selector) { selector.Where(sql.In(selector.C("id"), 1, 2)) }
							expected.Bulk = nil
							expected.Rows = []dialect.RowRef{{Table: "tenant.users", ID: 1}, {Table: "tenant.users", ID: 2}}
						}
						var scanner *Returning
						if returning {
							scanner = &Returning{Columns: []string{"id"}, Scan: func(rows dialect.Rows) error { var id int; return rows.Scan(&id) }}
							mock.ExpectQuery("UPDATE|DELETE").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
						} else {
							mock.ExpectExec("UPDATE|DELETE").WillReturnResult(sqlmock.NewResult(0, 2))
						}
						var count int
						var err error
						if deletion {
							count, err = DeleteNodes(t.Context(), driver, &DeleteSpec{Node: node, Predicate: predicate, Returning: scanner})
						} else {
							spec := &UpdateSpec{Node: node, Predicate: predicate, Returning: scanner}
							spec.SetField("name", field.TypeString, "a")
							count, err = UpdateNodes(t.Context(), driver, spec)
						}
						require.NoError(t, err)
						require.Equal(t, 2, count)
						require.Equal(t, expected, driver.calls[0].statement)
					})
				}
			}
		})
	}
}

func TestBatchCreateStatements(t *testing.T) {
	for _, name := range []dialect.Dialect{dialect.Postgres, dialect.SQLite} {
		for _, returning := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/returning=%t", name, returning), func(t *testing.T) {
				driver, mock := newStatementDriver(t, name)
				nodes := []*CreateSpec{NewCreateSpec("users", NewFieldSpec("id", field.TypeInt)), NewCreateSpec("users", NewFieldSpec("id", field.TypeInt))}
				for _, node := range nodes {
					node.SetField("name", field.TypeString, "a")
					node.Edges = []*EdgeSpec{{Rel: M2M, Table: "user_groups", Schema: "tenant", Columns: []string{"user_id", "group_id"}, Target: &EdgeTarget{Nodes: []databaseDriver.Value{2}}}}
					if returning {
						node.Returning = &Returning{Columns: []string{"id"}, Scan: func(rows dialect.Rows) error { return rows.Scan(&node.ID.Value) }}
					}
				}
				mock.ExpectBegin()
				if name == dialect.Postgres {
					mock.ExpectQuery("INSERT INTO").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10).AddRow(11))
					mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(0, 2))
				} else {
					for index := range nodes {
						mock.ExpectQuery("INSERT INTO").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(10 + index))
						mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(0, 1))
					}
				}
				mock.ExpectCommit()
				require.NoError(t, BatchCreate(t.Context(), driver, &BatchCreateSpec{Nodes: nodes}))
				var rows []dialect.RowRef
				for _, call := range driver.calls {
					require.Equal(t, dialect.StatementWrite, call.statement.Kind)
					if call.closed {
						require.Equal(t, call.statement.Rows, call.rows)
						rows = append(rows, call.rows...)
					} else {
						require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"tenant.user_groups"}, Bulk: []string{"tenant.user_groups"}}, call.statement)
					}
				}
				require.Equal(t, []dialect.RowRef{{Table: "users", ID: int64(10)}, {Table: "users", ID: int64(11)}}, rows)
			})
		}
	}
}

func TestInterruptedCreateStatements(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, returning := range []bool{false, true} {
			for _, failure := range []string{"scan", "iteration", "missing", "extra"} {
				t.Run(fmt.Sprintf("batch=%t/returning=%t/%s", batch, returning, failure), func(t *testing.T) {
					driver, mock := newStatementDriver(t, dialect.Postgres)
					nodes := []*CreateSpec{NewCreateSpec("users", NewFieldSpec("id", field.TypeInt))}
					if batch {
						nodes = append(nodes, NewCreateSpec("users", NewFieldSpec("id", field.TypeInt)))
					}
					for _, node := range nodes {
						node.Schema = "tenant"
						if returning {
							node.Returning = &Returning{Columns: []string{"id"}, Scan: func(rows dialect.Rows) error {
								var id int64
								if err := rows.Scan(&id); err != nil {
									return err
								}
								node.ID.Value = id
								return nil
							}}
						}
					}
					rows := sqlmock.NewRows([]string{"id"})
					switch failure {
					case "scan":
						if batch {
							rows.AddRow(10)
						}
						rows.AddRow("invalid ID")
					case "iteration":
						rows.AddRow(10).AddRow(11).RowError(1, errors.New("interrupted returning"))
					case "missing":
						if batch {
							rows.AddRow(10)
						}
					case "extra":
						for index := range len(nodes) + 1 {
							rows.AddRow(10 + index)
						}
					}
					mock.ExpectQuery("INSERT INTO").WillReturnRows(rows).RowsWillBeClosed()
					if batch {
						require.Error(t, BatchCreate(t.Context(), driver, &BatchCreateSpec{Nodes: nodes}))
					} else {
						require.Error(t, CreateNode(t.Context(), driver, nodes[0]))
					}
					require.Len(t, driver.calls, 1)
					require.True(t, driver.calls[0].closed)
					require.Equal(t, []string{"tenant.users"}, driver.calls[0].statement.Bulk)
					// Capture Bulk inside Close: setting it afterwards cannot protect cached reads.
					require.Equal(t, []string{"tenant.users"}, driver.calls[0].bulk)
					if batch && failure == "scan" {
						require.Equal(t, []dialect.RowRef{{Table: "tenant.users", ID: int64(10)}}, driver.calls[0].rows)
					}
				})
			}
		}
	}
}

func TestCreateWithoutIDStatements(t *testing.T) {
	for _, batch := range []bool{false, true} {
		for _, returning := range []bool{false, true} {
			t.Run(fmt.Sprintf("batch=%t/returning=%t", batch, returning), func(t *testing.T) {
				driver, mock := newStatementDriver(t, dialect.Postgres)
				spec := NewCreateSpec("user_groups", nil)
				spec.Schema = "tenant"
				spec.SetField("user_id", field.TypeInt, 1)
				if returning {
					spec.Returning = &Returning{Columns: []string{"user_id"}, Scan: func(rows dialect.Rows) error { var id int; return rows.Scan(&id) }}
					mock.ExpectQuery("INSERT INTO").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(1))
				} else {
					mock.ExpectExec("INSERT INTO").WillReturnResult(sqlmock.NewResult(0, 1))
				}
				if batch {
					require.NoError(t, BatchCreate(t.Context(), driver, &BatchCreateSpec{Nodes: []*CreateSpec{spec}}))
				} else {
					require.NoError(t, CreateNode(t.Context(), driver, spec))
				}
				require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"tenant.user_groups"}, Bulk: []string{"tenant.user_groups"}}, driver.calls[0].statement)
			})
		}
	}
}

func TestEdgeStatements(t *testing.T) {
	// fkEdge builds a fresh O2M edge spec; UpdateNodes consumes the spec, so
	// each mutation needs its own copy.
	fkEdge := func(nodes ...databaseDriver.Value) *EdgeSpec {
		return &EdgeSpec{
			Rel: O2M, Table: "pets", Schema: "tenant", Columns: []string{"owner_id"},
			Target: &EdgeTarget{IDSpec: NewFieldSpec("id", field.TypeInt), Nodes: nodes},
		}
	}
	// updateWithEdges runs a node update whose only side effect is the given
	// edge mutation, and returns the statements the driver observed.
	updateWithEdges := func(t *testing.T, mutate func(*EdgeMut), edgeStatements int) []*capturedStatement {
		t.Helper()
		driver, mock := newStatementDriver(t, dialect.Postgres)
		spec := NewUpdateSpec("users", []string{"id"}, NewFieldSpec("id", field.TypeInt))
		spec.SetField("name", field.TypeString, "a")
		mutate(&spec.Edges)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 1))
		for range edgeStatements {
			mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 2))
		}
		mock.ExpectCommit()
		count, err := UpdateNodes(t.Context(), driver, spec)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		return driver.calls[2:]
	}

	t.Run("query", func(t *testing.T) {
		driver, mock := newStatementDriver(t, dialect.Postgres)
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"user_id", "group_id"}))
		require.NoError(t, QueryEdges(t.Context(), driver, &EdgeQuerySpec{Edge: &EdgeSpec{Table: "user_groups", Schema: "tenant", Columns: []string{"user_id", "group_id"}}}))
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"tenant.user_groups"}}, driver.calls[0].statement)
	})

	t.Run("foreign key edges reference the targeted rows", func(t *testing.T) {
		expected := &dialect.Statement{
			Kind: dialect.StatementWrite, Tables: []string{"tenant.pets"},
			Rows: []dialect.RowRef{{Table: "tenant.pets", ID: 2}, {Table: "tenant.pets", ID: 3}},
		}
		t.Run("add", func(t *testing.T) {
			calls := updateWithEdges(t, func(edges *EdgeMut) { edges.Add = []*EdgeSpec{fkEdge(2, 3)} }, 1)
			require.Len(t, calls, 1)
			require.Equal(t, expected, calls[0].statement)
		})
		t.Run("clear", func(t *testing.T) {
			calls := updateWithEdges(t, func(edges *EdgeMut) { edges.Clear = []*EdgeSpec{fkEdge(2, 3)} }, 1)
			require.Len(t, calls, 1)
			require.Equal(t, expected, calls[0].statement)
		})
	})

	t.Run("clearing every foreign key falls back to a bulk reference", func(t *testing.T) {
		calls := updateWithEdges(t, func(edges *EdgeMut) { edges.Clear = []*EdgeSpec{fkEdge()} }, 1)
		require.Len(t, calls, 1)
		require.Equal(t, &dialect.Statement{
			Kind: dialect.StatementWrite, Tables: []string{"tenant.pets"}, Bulk: []string{"tenant.pets"},
		}, calls[0].statement)
	})
}

func TestUpdateReadStatements(t *testing.T) {
	t.Run("unchanged node", func(t *testing.T) {
		driver, mock := newStatementDriver(t, dialect.Postgres)
		spec := NewUpdateSpec("users", []string{"id"}, &FieldSpec{Column: "id", Type: field.TypeInt, Value: 1})
		spec.ScanValues = func([]string) ([]any, error) { return []any{new(int)}, nil }
		spec.Assign = func([]string, []any) error { return nil }
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		mock.ExpectCommit()
		require.NoError(t, UpdateNode(t.Context(), driver, spec))
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, IDColumn: "id", PointRead: true, RequestedIDs: []any{1}}, driver.calls[0].statement)
	})
	t.Run("select nodes for edge updates", func(t *testing.T) {
		driver, mock := newStatementDriver(t, dialect.Postgres)
		spec := NewUpdateSpec("users", []string{"id"}, NewFieldSpec("id", field.TypeInt))
		spec.SetField("name", field.TypeString, "a")
		spec.Edges.Clear = []*EdgeSpec{{Rel: M2M, Table: "user_groups", Columns: []string{"user_id", "group_id"}, Target: &EdgeTarget{}}}
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1).AddRow(2))
		mock.ExpectExec("UPDATE").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()
		count, err := UpdateNodes(t.Context(), driver, spec)
		require.NoError(t, err)
		require.Equal(t, 2, count)
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, IDColumn: "id"}, driver.calls[0].statement)
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}, Rows: []dialect.RowRef{{Table: "users", ID: int64(1)}, {Table: "users", ID: int64(2)}}}, driver.calls[1].statement)
		require.Equal(t, &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"user_groups"}, Bulk: []string{"user_groups"}}, driver.calls[2].statement)
	})
}
