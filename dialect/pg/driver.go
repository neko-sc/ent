// Package pg implements the driver boundary with native pgx codecs.
package pg

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
)

type Driver struct {
	pool         *pgxpool.Pool
	databaseOnce sync.Once
	database     *sql.DB
	capabilities dialect.Capabilities
}

type Option func(*Driver)

func WithCapabilities(capabilities dialect.Capabilities) Option {
	return func(d *Driver) { d.capabilities = capabilities }
}

func Open(ctx context.Context, dsn string, options ...Option) (*Driver, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, mapError(err)
	}
	var version int
	if err := pool.QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil {
		pool.Close()
		return nil, mapError(err)
	}
	d := NewDriver(pool)
	d.capabilities.ReturningOld = version >= 180000
	d.capabilities.ConflictDoSelect = version >= 190000
	for _, option := range options {
		option(d)
	}
	return d, nil
}

// NewDriver leaves version-gated capabilities disabled until explicitly configured.
func NewDriver(pool *pgxpool.Pool) *Driver {
	return &Driver{pool: pool, capabilities: dialect.Capabilities{NativeArray: true, MultiRowReturningOrdered: true}}
}

func (d *Driver) Pool() *pgxpool.Pool { return d.pool }

func (d *Driver) DB() *sql.DB {
	d.databaseOnce.Do(func() { d.database = stdlib.OpenDBFromPool(d.pool) })
	return d.database
}

func (d *Driver) Dialect() dialect.Dialect { return dialect.Postgres }

func (d *Driver) Capabilities() dialect.Capabilities { return d.capabilities }

func (d *Driver) Close() error {
	err := d.DB().Close()
	d.pool.Close()
	return err
}

func (d *Driver) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	if len(entsql.VarsFromContext(ctx)) == 0 {
		tag, err := d.pool.Exec(ctx, query, arguments...)
		return result{tag}, mapError(err)
	}
	connection, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	cleanup, err := setVars(ctx, connection, false)
	if err != nil {
		connection.Release()
		return nil, err
	}
	tag, err := connection.Exec(ctx, query, arguments...)
	err = errors.Join(mapError(err), cleanup())
	connection.Release()
	return result{tag}, err
}

func (d *Driver) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	if len(entsql.VarsFromContext(ctx)) == 0 {
		native, err := d.pool.Query(ctx, query, arguments...)
		if err != nil {
			return nil, mapError(err)
		}
		return &rows{Rows: native}, nil
	}
	connection, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	cleanup, err := setVars(ctx, connection, false)
	if err != nil {
		connection.Release()
		return nil, err
	}
	native, err := connection.Query(ctx, query, arguments...)
	if err != nil {
		err = errors.Join(mapError(err), cleanup())
		connection.Release()
		return nil, err
	}
	return &rows{Rows: native, cleanup: func() error {
		err := cleanup()
		connection.Release()
		return err
	}}, nil
}

func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) { return d.BeginTx(ctx, nil) }

func (d *Driver) BeginTx(ctx context.Context, options *dialect.TxOptions) (dialect.Tx, error) {
	transactionOptions := pgx.TxOptions{}
	if options != nil {
		switch options.Isolation {
		case sql.LevelDefault:
		case sql.LevelReadUncommitted:
			transactionOptions.IsoLevel = pgx.ReadUncommitted
		case sql.LevelReadCommitted:
			transactionOptions.IsoLevel = pgx.ReadCommitted
		case sql.LevelRepeatableRead:
			transactionOptions.IsoLevel = pgx.RepeatableRead
		case sql.LevelSerializable:
			transactionOptions.IsoLevel = pgx.Serializable
		default:
			return nil, &dialect.UnsupportedError{Feature: fmt.Sprintf("isolation level %s", options.Isolation), Dialect: dialect.Postgres}
		}
		if options.ReadOnly {
			transactionOptions.AccessMode = pgx.ReadOnly
		}
	}
	native, err := d.pool.BeginTx(ctx, transactionOptions)
	if err != nil {
		return nil, mapError(err)
	}
	return &Tx{native: native, ctx: ctx}, nil
}

type Tx struct {
	native pgx.Tx
	ctx    context.Context
}

func (t *Tx) Exec(ctx context.Context, query string, arguments []any) (dialect.Result, error) {
	if _, err := setVars(ctx, t.native, true); err != nil {
		return nil, err
	}
	tag, err := t.native.Exec(ctx, query, arguments...)
	return result{tag}, mapError(err)
}

func (t *Tx) Query(ctx context.Context, query string, arguments []any) (dialect.Rows, error) {
	if _, err := setVars(ctx, t.native, true); err != nil {
		return nil, err
	}
	native, err := t.native.Query(ctx, query, arguments...)
	if err != nil {
		return nil, mapError(err)
	}
	return &rows{Rows: native}, nil
}

func (t *Tx) Commit() error { return mapError(t.native.Commit(t.ctx)) }

func (t *Tx) Rollback() error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.ctx), 5*time.Second)
	defer cancel()
	return mapError(t.native.Rollback(ctx))
}

type result struct{ pgconn.CommandTag }

func (r result) RowsAffected() (int64, error) { return r.CommandTag.RowsAffected(), nil }

type rows struct {
	pgx.Rows
	cleanup     func() error
	closeError  error
	closed      bool
	targets     []any
	adaptations []scanAdaptation
}

type scanAdaptation struct {
	destinationType reflect.Type
	interfaceValue  bool
	dataType        *pgtype.Type
	value           any
	bytes           pgtype.UndecodedBytes
}

func (r *rows) Columns() ([]string, error) {
	descriptions := r.FieldDescriptions()
	columns := make([]string, len(descriptions))
	for index, description := range descriptions {
		columns[index] = description.Name
	}
	return columns, nil
}

// Scan adapts named interface pointers and binary codecs unsupported by sql.Scanner.
func (r *rows) Scan(destinations ...any) error {
	descriptions := r.FieldDescriptions()
	if len(destinations) != len(descriptions) {
		return mapError(r.Rows.Scan(destinations...))
	}
	if len(r.targets) != len(destinations) {
		r.targets = make([]any, len(destinations))
		r.adaptations = make([]scanAdaptation, len(destinations))
	}
	for index, destination := range destinations {
		adaptation := &r.adaptations[index]
		if destinationType := reflect.TypeOf(destination); adaptation.destinationType != destinationType {
			*adaptation = scanAdaptation{destinationType: destinationType}
			switch destination.(type) {
			case *driver.Value, *entsql.UnknownType:
				adaptation.interfaceValue = true
			case sql.Scanner:
				description := descriptions[index]
				if description.Format == pgtype.BinaryFormatCode {
					types := r.Conn().TypeMap()
					// Native codec scanners take precedence over sql.Scanner in pgx.
					if dataType, ok := types.TypeForOID(description.DataTypeOID); ok && dataType.Codec.PlanScan(types, description.DataTypeOID, description.Format, destination) == nil {
						adaptation.dataType = dataType
					}
				}
			}
		}
		switch {
		case adaptation.interfaceValue:
			r.targets[index] = &adaptation.value
		case adaptation.dataType != nil:
			r.targets[index] = &adaptation.bytes
		default:
			r.targets[index] = destination
		}
	}
	if err := r.Rows.Scan(r.targets...); err != nil {
		return mapError(err)
	}
	for index, destination := range destinations {
		adaptation := &r.adaptations[index]
		if adaptation.interfaceValue {
			switch pointer := destination.(type) {
			case *driver.Value:
				*pointer = adaptation.value
			case *entsql.UnknownType:
				*pointer = adaptation.value
			}
		} else if adaptation.dataType != nil {
			description := descriptions[index]
			types := r.Conn().TypeMap()
			decoded, err := adaptation.dataType.Codec.DecodeDatabaseSQLValue(types, description.DataTypeOID, description.Format, adaptation.bytes)
			if err != nil {
				return mapError(err)
			}
			// Preserve SQL scalar values, especially bytea and time.Time.
			if _, binary := decoded.([]byte); binary && description.DataTypeOID != pgtype.ByteaOID {
				decoded, err = adaptation.dataType.Codec.DecodeValue(types, description.DataTypeOID, description.Format, adaptation.bytes)
				if err != nil {
					return mapError(err)
				}
				text, err := types.Encode(description.DataTypeOID, pgtype.TextFormatCode, decoded, nil)
				if err != nil {
					return mapError(err)
				}
				decoded = string(text)
			}
			if err := destination.(sql.Scanner).Scan(decoded); err != nil {
				return mapError(err)
			}
		}
	}
	return nil
}

func (r *rows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.Close()
	return false
}

func (r *rows) Err() error { return errors.Join(mapError(r.Rows.Err()), r.closeError) }

func (r *rows) Close() error {
	if !r.closed {
		r.closed = true
		r.Rows.Close()
		if r.cleanup != nil {
			r.closeError = r.cleanup()
		}
	}
	return r.Err()
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func setVars(ctx context.Context, connection executor, local bool) (func() error, error) {
	variables := entsql.VarsFromContext(ctx)
	reset := make([]string, 0, len(variables))
	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var result error
		if len(reset) > 0 {
			_, err := connection.Exec(ctx, strings.Join(reset, "; "), pgx.QueryExecModeSimpleProtocol)
			result = mapError(err)
		}
		if result != nil {
			if acquired, ok := connection.(*pgxpool.Conn); ok {
				result = errors.Join(result, acquired.Conn().Close(ctx))
			}
		}
		return result
	}
	if len(variables) == 0 {
		return cleanup, nil
	}
	var statement strings.Builder
	statement.WriteString("SELECT ")
	arguments := make([]any, 0, 3*len(variables))
	for index, variable := range variables {
		if index > 0 {
			statement.WriteString(", ")
		}
		fmt.Fprintf(&statement, "set_config($%d, $%d, $%d)", 3*index+1, 3*index+2, 3*index+3)
		arguments = append(arguments, variable[0], variable[1], local)
	}
	if _, err := connection.Exec(ctx, statement.String(), arguments...); err != nil {
		return nil, mapError(err)
	}
	if !local {
		for _, variable := range variables {
			reset = append(reset, "RESET "+pgx.Identifier{variable[0]}.Sanitize())
		}
	}
	return cleanup, nil
}

func mapError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return dialect.ErrNoRows
	}
	if postgres, ok := errors.AsType[*pgconn.PgError](err); ok {
		kinds := map[string]dialect.ConstraintKind{"23505": dialect.Unique, "23503": dialect.ForeignKey, "23514": dialect.Check, "23502": dialect.NotNull, "23P01": dialect.Exclusion}
		if kind, ok := kinds[postgres.Code]; ok {
			return &dialect.ConstraintError{Kind: kind, Constraint: postgres.ConstraintName, Table: postgres.TableName, Err: err}
		}
	}
	return err
}

var _ dialect.Driver = (*Driver)(nil)
