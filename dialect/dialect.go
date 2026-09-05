// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package dialect

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/google/uuid"
)

type Dialect string

const (
	Postgres = "postgres"
	SQLite   = "sqlite3"
)

type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Columns() ([]string, error)
	Close() error
	Err() error
}

type Result interface{ RowsAffected() (int64, error) }

type ExecQuerier interface {
	Exec(ctx context.Context, query string, args []any) (Result, error)
	Query(ctx context.Context, query string, args []any) (Rows, error)
}

type TxOptions = sql.TxOptions

type Tx interface {
	ExecQuerier
	Commit() error
	Rollback() error
}

type Capabilities struct {
	ReturningOld             bool
	ConflictDoSelect         bool
	NativeArray              bool
	MultiRowReturningOrdered bool
}

type Driver interface {
	ExecQuerier
	Tx(ctx context.Context) (Tx, error)
	BeginTx(ctx context.Context, opts *TxOptions) (Tx, error)
	Close() error
	Dialect() Dialect
	Capabilities() Capabilities
}

type nopTx struct{ Driver }

func (nopTx) Commit() error { return nil }

func (nopTx) Rollback() error { return nil }

func NopTx(d Driver) Tx { return nopTx{d} }

type DebugDriver struct {
	Driver
	log func(context.Context, ...any)
}

func Debug(d Driver, logger ...func(...any)) Driver {
	logf := log.Println
	if len(logger) == 1 {
		logf = logger[0]
	}
	return &DebugDriver{d, func(_ context.Context, values ...any) { logf(values...) }}
}

func DebugWithContext(d Driver, logger func(context.Context, ...any)) Driver {
	return &DebugDriver{d, logger}
}

func (d *DebugDriver) Exec(ctx context.Context, query string, args []any) (Result, error) {
	d.log(ctx, fmt.Sprintf("driver.Exec: query=%v args=%v", query, args))
	return d.Driver.Exec(ctx, query, args)
}

func (d *DebugDriver) Query(ctx context.Context, query string, args []any) (Rows, error) {
	d.log(ctx, fmt.Sprintf("driver.Query: query=%v args=%v", query, args))
	return d.Driver.Query(ctx, query, args)
}

func (d *DebugDriver) Tx(ctx context.Context) (Tx, error) { return d.BeginTx(ctx, nil) }

func (d *DebugDriver) BeginTx(ctx context.Context, opts *TxOptions) (Tx, error) {
	tx, err := d.Driver.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	d.log(ctx, fmt.Sprintf("driver.BeginTx(%s): started", id))
	return &DebugTx{tx, id, d.log, ctx}, nil
}

// DB exposes the migration connection when supported by the wrapped driver.
func (d *DebugDriver) DB() *sql.DB {
	if driver, ok := d.Driver.(interface{ DB() *sql.DB }); ok {
		return driver.DB()
	}
	return nil
}

type DebugTx struct {
	Tx
	id  string
	log func(context.Context, ...any)
	ctx context.Context
}

func (d *DebugTx) Exec(ctx context.Context, query string, args []any) (Result, error) {
	d.log(ctx, fmt.Sprintf("Tx(%s).Exec: query=%v args=%v", d.id, query, args))
	return d.Tx.Exec(ctx, query, args)
}

func (d *DebugTx) Query(ctx context.Context, query string, args []any) (Rows, error) {
	d.log(ctx, fmt.Sprintf("Tx(%s).Query: query=%v args=%v", d.id, query, args))
	return d.Tx.Query(ctx, query, args)
}

func (d *DebugTx) Commit() error {
	d.log(d.ctx, fmt.Sprintf("Tx(%s): committed", d.id))
	return d.Tx.Commit()
}

func (d *DebugTx) Rollback() error {
	d.log(d.ctx, fmt.Sprintf("Tx(%s): rolled back", d.id))
	return d.Tx.Rollback()
}
