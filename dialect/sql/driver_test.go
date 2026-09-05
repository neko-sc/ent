// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/neko-sc/ent/dialect"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type postgresError struct {
	Code, Constraint, Table string
}

func (err *postgresError) Error() string    { return "PostgreSQL error " + err.Code }
func (err *postgresError) SQLState() string { return err.Code }

func TestPostgresConstraintError(t *testing.T) {
	for code, kind := range map[string]dialect.ConstraintKind{"23505": dialect.Unique, "23503": dialect.ForeignKey, "23514": dialect.Check, "23502": dialect.NotNull, "23P01": dialect.Exclusion} {
		original := &postgresError{Code: code, Constraint: "users_key", Table: "users"}
		cleanup := errors.New("cleanup failed")
		err := mapError(errors.Join(fmt.Errorf("query: %w", original), cleanup))
		constraint, ok := errors.AsType[*dialect.ConstraintError](err)
		require.True(t, ok)
		require.Equal(t, kind, constraint.Kind)
		require.Equal(t, "users_key", constraint.Constraint)
		require.Equal(t, "users", constraint.Table)
		require.ErrorIs(t, err, original)
		require.ErrorIs(t, err, cleanup)
		wrapped := fmt.Errorf("operation: %w", err)
		require.Same(t, wrapped, mapError(wrapped))
	}
}

func TestWithVars(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	drv := OpenDB(dialect.Postgres, db)
	mock.ExpectExec("SET foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	var rows dialect.Rows
	rows, err = drv.Query(
		WithVar(context.Background(), "foo", "bar"),
		"SELECT 1",
		[]any{},
	)
	require.NoError(t, err)
	require.NoError(t, rows.Close(), "rows should be closed to release the connection")
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectExec("SET foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("SET foo = 'baz'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	rows, err = drv.Query(
		WithVar(WithVar(context.Background(), "foo", "bar"), "foo", "baz"),
		"SELECT 1",
		[]any{},
	)
	require.NoError(t, err)
	require.NoError(t, rows.Close(), "rows should be closed to release the connection")
	require.NoError(t, mock.ExpectationsWereMet())

	mock.ExpectBegin()
	mock.ExpectExec("SET LOCAL foo = 'bar'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectCommit()
	tx, err := drv.Tx(context.Background())
	require.NoError(t, err)
	rows, err = tx.Query(
		WithVar(context.Background(), "foo", "bar"),
		"SELECT 1",
		[]any{},
	)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.NoError(t, mock.ExpectationsWereMet())
	// Rows should not be closed to release the session,
	// as a transaction is always scoped to a single connection.

	mock.ExpectExec("SET foo = 'qux'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO users DEFAULT VALUES").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	_, err = drv.Exec(
		WithVar(context.Background(), "foo", "qux"),
		"INSERT INTO users DEFAULT VALUES",
		[]any{},
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	// No rows are returned, so no need to close them.

	mock.ExpectExec("SET foo = 'foo'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO users DEFAULT VALUES").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("RESET foo").WillReturnResult(sqlmock.NewResult(0, 0))
	_, err = drv.Exec(
		WithVar(context.Background(), "foo", "foo"),
		"INSERT INTO users DEFAULT VALUES",
		[]any{},
	)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	// No rows are returned, so no need to close them.
}
