package sql

import (
	"database/sql"
	"errors"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	sqlite "github.com/ncruces/go-sqlite3"
	"github.com/neko-sc/ent/dialect"
)

func mapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return dialect.ErrNoRows
	}
	if _, ok := errors.AsType[*dialect.ConstraintError](err); ok {
		return err
	}
	if postgres, ok := errors.AsType[interface {
		error
		SQLState() string
	}](err); ok {
		kinds := map[string]dialect.ConstraintKind{"23505": dialect.Unique, "23503": dialect.ForeignKey, "23514": dialect.Check, "23502": dialect.NotNull, "23P01": dialect.Exclusion}
		if kind, ok := kinds[postgres.SQLState()]; ok {
			constraint := &dialect.ConstraintError{Kind: kind, Err: err}
			if native, ok := postgres.(*pgconn.PgError); ok {
				constraint.Constraint, constraint.Table = native.ConstraintName, native.TableName
			} else if value := reflect.Indirect(reflect.ValueOf(postgres)); value.IsValid() && value.Kind() == reflect.Struct {
				// database/sql PostgreSQL drivers expose diagnostic fields under
				// different names; lib/pq uses Constraint and Table.
				if name := value.FieldByName("Constraint"); name.IsValid() && name.Kind() == reflect.String {
					constraint.Constraint = name.String()
				}
				if table := value.FieldByName("Table"); table.IsValid() && table.Kind() == reflect.String {
					constraint.Table = table.String()
				}
			}
			return constraint
		}
	}
	var code int
	if native, ok := errors.AsType[*sqlite.Error](err); ok {
		code = int(native.ExtendedCode())
	}
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		value := reflect.ValueOf(cause)
		if value.Kind() == reflect.Struct && value.Type().PkgPath() == "github.com/mattn/go-sqlite3" {
			if extended := value.FieldByName("ExtendedCode"); extended.IsValid() && extended.CanInt() {
				code = int(extended.Int())
			}
		}
	}
	kinds := map[int]dialect.ConstraintKind{2067: dialect.Unique, 1555: dialect.Unique, 787: dialect.ForeignKey, 275: dialect.Check, 1299: dialect.NotNull}
	if kind, ok := kinds[code]; ok {
		constraint := &dialect.ConstraintError{Kind: kind, Err: err}
		if _, name, found := strings.Cut(err.Error(), "constraint failed: "); found {
			constraint.Constraint = name
			if table, _, found := strings.Cut(name, "."); found {
				constraint.Table = table
			}
		}
		return constraint
	}
	return err
}

func (r *Rows) Err() error { return mapError(r.ColumnScanner.Err()) }

func (r *Rows) Close() error { return mapError(r.ColumnScanner.Close()) }
