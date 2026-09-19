// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

func TestTransactionInvalidation(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(map[bool]string{true: "commit", false: "rollback"}[commit], func(t *testing.T) {
			cached, mock := newTestDriver(t)
			ctx := Cache(readContext(t.Context(), 1))
			expectRead(mock, "SELECT id", 1)
			readIDs(t, cached, ctx, "SELECT id")
			mock.ExpectBegin()
			transaction, err := cached.Tx(t.Context())
			require.NoError(t, err)
			require.IsType(t, &Tx{}, transaction)
			txContext, info := WithInfo(ctx)
			expectRead(mock, "SELECT id", 2)
			require.Equal(t, []int64{2}, readIDs(t, transaction, txContext, "SELECT id"))
			require.Equal(t, BypassTx, info.Bypass)
			executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
			executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
			require.Zero(t, cached.Stats().Snapshot().Bumps["users"])
			require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			if commit {
				mock.ExpectCommit()
				require.NoError(t, transaction.Commit())
				require.Equal(t, uint64(1), cached.Stats().Snapshot().Bumps["users"])
				expectRead(mock, "SELECT id", 3)
				require.Equal(t, []int64{3}, readIDs(t, cached, ctx, "SELECT id"))
			} else {
				mock.ExpectRollback()
				require.NoError(t, transaction.Rollback())
				// A rolled back transaction discards its writes without invalidating.
				require.Zero(t, cached.Stats().Snapshot().Bumps["users"])
				require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
			}
		})
	}
}

func TestTransactionUntilWrite(t *testing.T) {
	cached, mock := newTestDriver(t, TxReads(TxReadsUntilWrite))
	ctx, info := WithInfo(Cache(readContext(t.Context(), 1)))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	mock.ExpectBegin()
	transaction, err := cached.BeginTx(t.Context(), &dialect.TxOptions{})
	require.NoError(t, err)
	readIDs(t, transaction, ctx, "SELECT id")
	require.True(t, info.Hit)
	executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
	expectRead(mock, "SELECT id", 2)
	readIDs(t, transaction, ctx, "SELECT id")
	require.Equal(t, BypassTx, info.Bypass)
	mock.ExpectRollback()
	require.NoError(t, transaction.Rollback())
}

func TestTransactionReturningAndClosedReads(t *testing.T) {
	cached, mock := newTestDriver(t, TxReads(TxReadsUntilWrite))
	ctx, info := WithInfo(Cache(readContext(t.Context(), 1)))
	expectRead(mock, "SELECT id", 1)
	readIDs(t, cached, ctx, "SELECT id")
	mock.ExpectBegin()
	transaction, err := cached.Tx(t.Context())
	require.NoError(t, err)
	readIDs(t, transaction, ctx, "SELECT id")
	require.True(t, info.Hit)
	statement := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}}
	expectRead(mock, "UPDATE users RETURNING id", 1)
	rows, err := transaction.Query(dialect.WithStatement(t.Context(), statement), "UPDATE users RETURNING id", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	statement.Rows = []dialect.RowRef{{Table: "users", ID: id}}
	require.NoError(t, rows.Close())
	require.Zero(t, cached.Stats().Snapshot().Bumps["users"])
	// The returning write marks the transaction dirty, so its reads bypass now.
	expectRead(mock, "SELECT id", 1)
	readIDs(t, transaction, ctx, "SELECT id")
	require.Equal(t, BypassTx, info.Bypass)
	mock.ExpectCommit()
	require.NoError(t, transaction.Commit())
	require.Equal(t, uint64(1), cached.Stats().Snapshot().Bumps["users"])
	expectRead(mock, "SELECT id", 2)
	readIDs(t, cached, ctx, "SELECT id")
	_, err = transaction.Query(ctx, "SELECT id", nil)
	require.Error(t, err)
	mock.ExpectBegin()
	transaction, err = cached.Tx(t.Context())
	require.NoError(t, err)
	mock.ExpectRollback()
	require.NoError(t, transaction.Rollback())
	_, err = transaction.Query(ctx, "SELECT id", nil)
	require.Error(t, err)
}

func TestTransactionUnclosedWriteRows(t *testing.T) {
	for _, commit := range []bool{true, false} {
		t.Run(map[bool]string{true: "commit", false: "rollback"}[commit], func(t *testing.T) {
			cached, mock := newTestDriver(t)
			for _, id := range []int{1, 2} {
				expectRead(mock, "SELECT id", int64(id))
				readIDs(t, cached, Cache(readContext(t.Context(), id)), "SELECT id")
			}
			mock.ExpectBegin()
			transaction, err := cached.Tx(t.Context())
			require.NoError(t, err)
			statement := &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}}
			expectRead(mock, "UPDATE users RETURNING id", 1, 2)
			rows, err := transaction.Query(dialect.WithStatement(t.Context(), statement), "UPDATE users RETURNING id", nil)
			require.NoError(t, err)
			require.True(t, rows.Next())
			var id int64
			require.NoError(t, rows.Scan(&id))
			statement.Rows = []dialect.RowRef{{Table: "users", ID: id}}
			if commit {
				mock.ExpectCommit()
				require.NoError(t, transaction.Commit())
			} else {
				mock.ExpectRollback()
				require.NoError(t, transaction.Rollback())
			}
			// A commit invalidates rows the unread RETURNING stream may have changed.
			resolved := cached.Stats().Snapshot().Bumps["users"]
			require.Equal(t, commit, resolved > 0)
			require.NoError(t, rows.Close())
			require.NoError(t, rows.Close())
			require.Equal(t, resolved, cached.Stats().Snapshot().Bumps["users"])
			for _, id := range []int{1, 2} {
				ctx, info := WithInfo(Cache(readContext(t.Context(), id)))
				if commit {
					expectRead(mock, "SELECT id", int64(id))
				}
				readIDs(t, cached, ctx, "SELECT id")
				require.Equal(t, !commit, info.Hit)
			}
		})
	}
}

func TestTransactionErrors(t *testing.T) {
	cached, mock := newTestDriver(t)
	mock.ExpectBegin().WillReturnError(errors.New("begin"))
	_, err := cached.Tx(t.Context())
	require.ErrorContains(t, err, "begin")
	mock.ExpectBegin()
	transaction, err := cached.Tx(t.Context())
	require.NoError(t, err)
	executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
	mock.ExpectCommit().WillReturnError(errors.New("commit"))
	require.ErrorContains(t, transaction.Commit(), "commit")
	require.Zero(t, cached.Stats().Snapshot().Bumps["users"])
}

func TestTransactionBumpFailure(t *testing.T) {
	var reported error
	cached, mock := newTestDriver(t, Generations(&failingGenerations{GenerationStore: NewMemoryGenerations(), bumpError: errors.New("bump")}), OnError(func(_ context.Context, err error) { reported = err }))
	mock.ExpectBegin()
	transaction, err := cached.Tx(t.Context())
	require.NoError(t, err)
	executeWrite(t, transaction, mock, writeContext(t.Context(), 1))
	mock.ExpectCommit()
	require.NoError(t, transaction.Commit())
	require.ErrorContains(t, reported, "bump")
}
