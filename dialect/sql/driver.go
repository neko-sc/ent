// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neko-sc/ent/dialect"
)

// Driver is a dialect.Driver implementation for SQL based databases.
type Driver struct {
	Conn
	capabilities *dialect.Capabilities
}

type Option func(*Driver)

func WithCapabilities(capabilities dialect.Capabilities) Option {
	return func(d *Driver) { d.capabilities = &capabilities }
}

func (d *Driver) Capabilities() dialect.Capabilities {
	if d.capabilities != nil {
		return *d.capabilities
	}
	if d.Dialect() == dialect.Postgres {
		return dialect.Capabilities{NativeArray: true, MultiRowReturningOrdered: true}
	}
	return dialect.Capabilities{}
}

// NewDriver creates a new Driver with the given Conn and dialect.
func NewDriver(name dialect.Dialect, c Conn, opts ...Option) *Driver {
	c.dialect = name
	d := &Driver{Conn: c}
	d.Conn.dialect = d.Dialect()
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Open wraps the database/sql.Open method and returns a dialect.Driver that implements the an ent/dialect.Driver interface.
func Open(name dialect.Dialect, source string, opts ...Option) (*Driver, error) {
	db, err := sql.Open(string(name), source)
	if err != nil {
		return nil, err
	}
	return NewDriver(name, Conn{db, name}, opts...), nil
}

// OpenDB wraps the given database/sql.DB method with a Driver.
func OpenDB(name dialect.Dialect, db *sql.DB, opts ...Option) *Driver {
	return NewDriver(name, Conn{db, name}, opts...)
}

// DB returns the underlying *sql.DB instance.
func (d Driver) DB() *sql.DB {
	db, _ := d.ExecQuerier.(*sql.DB)
	return db
}

func (d Driver) Dialect() dialect.Dialect {
	// If the underlying driver is wrapped with a telemetry driver.
	for _, name := range []dialect.Dialect{dialect.SQLite, dialect.Postgres} {
		if strings.HasPrefix(string(d.dialect), string(name)) {
			return name
		}
	}
	return d.dialect
}

// Tx starts and returns a transaction.
func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) {
	return d.BeginTx(ctx, nil)
}

// BeginTx starts a transaction with options.
func (d *Driver) BeginTx(ctx context.Context, opts *TxOptions) (dialect.Tx, error) {
	tx, err := d.DB().BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{
		Conn: Conn{tx, d.dialect},
		Tx:   tx,
	}, nil
}

// Close closes the underlying connection.
func (d *Driver) Close() error { return d.DB().Close() }

// Tx implements dialect.Tx interface.
type Tx struct {
	Conn
	driver.Tx
}

func (t *Tx) Commit() error { return mapError(t.Tx.Commit()) }

func (t *Tx) Rollback() error { return mapError(t.Tx.Rollback()) }

// ctyVarsKey is the key used for attaching and reading the context variables.
type ctxVarsKey struct{}

// sessionVars holds sessions/transactions variables to set before every statement.
type sessionVars struct {
	vars []struct{ k, v string }
}

// WithVar returns a new context that holds the session variable to be executed before every query.
func WithVar(ctx context.Context, name, value string) context.Context {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	sv.vars = append(append(sv.vars[:0:0], sv.vars...), struct {
		k, v string
	}{
		k: name,
		v: value,
	})
	return context.WithValue(ctx, ctxVarsKey{}, sv)
}

// VarFromContext returns the session variable value from the context.
func VarFromContext(ctx context.Context, name string) (string, bool) {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	for _, s := range sv.vars {
		if s.k == name {
			return s.v, true
		}
	}
	return "", false
}

// VarsFromContext returns session variables in their application order.
func VarsFromContext(ctx context.Context) [][2]string {
	sv, _ := ctx.Value(ctxVarsKey{}).(sessionVars)
	values := make([][2]string, len(sv.vars))
	for i, variable := range sv.vars {
		values[i] = [2]string{variable.k, variable.v}
	}
	return values
}

// WithIntVar calls WithVar with the string representation of the value.
func WithIntVar(ctx context.Context, name string, value int) context.Context {
	return WithVar(ctx, name, strconv.Itoa(value))
}

// ExecQuerier wraps the standard Exec and Query methods.
type ExecQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Conn implements dialect.ExecQuerier given ExecQuerier.
type Conn struct {
	ExecQuerier
	dialect dialect.Dialect
}

func (c Conn) Exec(ctx context.Context, query string, args []any) (result dialect.Result, rerr error) {
	if c.dialect == dialect.SQLite {
		converted, err := arrayArguments(args)
		if err != nil {
			return nil, err
		}
		args = converted
	}
	ex, cf, err := c.maySetVars(ctx)
	if err != nil {
		return nil, err
	}
	if cf != nil {
		defer func() { rerr = errors.Join(rerr, cf()) }()
	}
	result, err = ex.ExecContext(ctx, query, args...)
	return result, mapError(err)
}

func (c Conn) Query(ctx context.Context, query string, args []any) (dialect.Rows, error) {
	if c.dialect == dialect.SQLite {
		converted, err := arrayArguments(args)
		if err != nil {
			return nil, err
		}
		args = converted
	}
	ex, cf, err := c.maySetVars(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := ex.QueryContext(ctx, query, args...)
	if err != nil {
		if cf != nil {
			err = errors.Join(err, cf())
		}
		return nil, mapError(err)
	}
	result := &Rows{ColumnScanner: rows, dialect: c.dialect}
	if cf != nil {
		result.ColumnScanner = &rowsWithCloser{ColumnScanner: rows, closer: cf}
	}
	return result, nil
}

// maySetVars sets the session variables before executing a query.
func (c Conn) maySetVars(ctx context.Context) (ExecQuerier, func() error, error) {
	variables := VarsFromContext(ctx)
	if len(variables) == 0 {
		return c.ExecQuerier, nil, nil
	}
	if c.dialect != dialect.Postgres {
		return nil, nil, &dialect.UnsupportedError{Feature: "session variables", Dialect: c.dialect}
	}
	var executor ExecQuerier
	var closeConnection func() error
	local := false
	switch connection := c.ExecQuerier.(type) {
	case *sql.Tx:
		executor, local = connection, true
	case *sql.DB:
		acquired, err := connection.Conn(ctx)
		if err != nil {
			return nil, nil, err
		}
		executor, closeConnection = acquired, acquired.Close
	default:
		return nil, nil, errors.New("dialect/sql: session variables require a database or transaction")
	}
	reset := make([]string, 0, len(variables))
	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var err error
		for _, query := range reset {
			_, resetError := executor.ExecContext(ctx, query)
			err = errors.Join(err, resetError)
		}
		if err != nil {
			if connection, ok := executor.(*sql.Conn); ok {
				connection.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		return errors.Join(err, closeConnection())
	}
	seen := make(map[string]bool, len(variables))
	for _, variable := range variables {
		name := variable[0]
		for _, character := range name {
			if character != '_' && character != '.' && !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9') {
				if closeConnection != nil {
					cleanup()
				}
				return nil, nil, fmt.Errorf("dialect/sql: invalid session variable %q", name)
			}
		}
		if name == "" {
			if closeConnection != nil {
				cleanup()
			}
			return nil, nil, errors.New("dialect/sql: empty session variable")
		}
		prefix := "SET "
		if local {
			prefix = "SET LOCAL "
		}
		if _, err := executor.ExecContext(ctx, prefix+name+" = '"+strings.ReplaceAll(variable[1], "'", "''")+"'"); err != nil {
			if closeConnection != nil {
				err = errors.Join(err, cleanup())
			}
			return nil, nil, mapError(err)
		}
		if !local && !seen[name] {
			reset = append(reset, "RESET "+name)
			seen[name] = true
		}
	}
	if closeConnection != nil {
		return executor, cleanup, nil
	}
	return executor, nil, nil
}

var _ dialect.Driver = (*Driver)(nil)

type (
	// Rows wraps the sql.Rows to avoid locks copy.
	Rows struct {
		ColumnScanner
		dialect dialect.Dialect
	}
	// Result is an alias to sql.Result.
	Result = sql.Result
	// NullBool is an alias to sql.NullBool.
	NullBool = sql.NullBool
	// NullInt64 is an alias to sql.NullInt64.
	NullInt64 = sql.NullInt64
	// NullString is an alias to sql.NullString.
	NullString = sql.NullString
	// NullFloat64 is an alias to sql.NullFloat64.
	NullFloat64 = sql.NullFloat64
	// NullTime represents a time.Time that may be null.
	NullTime = sql.NullTime
	// TxOptions holds the transaction options to be used in DB.BeginTx.
	TxOptions = sql.TxOptions
)

// NullScanner implements the sql.Scanner interface such that it
// can be used as a scan destination, similar to the types above.
type NullScanner struct {
	S     sql.Scanner
	Valid bool // Valid is true if the Scan value is not NULL.
}

// Scan implements the Scanner interface.
func (n *NullScanner) Scan(value any) error {
	n.Valid = value != nil
	if n.Valid {
		return n.S.Scan(value)
	}
	return nil
}

// ColumnScanner is the interface that wraps the standard
// sql.Rows methods used for scanning database rows.
type ColumnScanner interface {
	Close() error
	ColumnTypes() ([]*sql.ColumnType, error)
	Columns() ([]string, error)
	Err() error
	Next() bool
	NextResultSet() bool
	Scan(dest ...any) error
}

// rowsWithCloser wraps the ColumnScanner interface with a custom Close hook.
type rowsWithCloser struct {
	ColumnScanner
	closer     func() error
	once       sync.Once
	closeError error
}

func (r *rowsWithCloser) Next() bool {
	if r.ColumnScanner.Next() {
		return true
	}
	r.Close()
	return false
}

func (r *rowsWithCloser) Err() error { return errors.Join(r.ColumnScanner.Err(), r.closeError) }

// Close releases session state and its connection exactly once.
func (r *rowsWithCloser) Close() error {
	r.once.Do(func() { r.closeError = errors.Join(r.ColumnScanner.Close(), r.closer()) })
	return r.closeError
}
