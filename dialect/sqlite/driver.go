// Package sqlite uses ncruces SQLite through database/sql. SQLite has no native
// wire-codec advantage; database/sql provides concrete-pointer and NULL scanning.
// One shared connection serializes operations and keeps :memory: migrations and
// application queries on the same database. Close rows before starting a query.
package sqlite

import (
	"context"

	sqlite3 "github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
)

type Driver struct{ *sql.Driver }

type Option func(*Driver)

func Open(ctx context.Context, dsn string, opts ...Option) (*Driver, error) {
	db, err := driver.Open(dsn, func(conn *sqlite3.Conn) error {
		return conn.Exec("PRAGMA foreign_keys = ON")
	})
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	d := &Driver{sql.OpenDB(dialect.SQLite, db)}
	for _, opt := range opts {
		opt(d)
	}
	return d, nil
}

var _ dialect.Driver = (*Driver)(nil)
