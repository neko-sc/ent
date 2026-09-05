package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/dialect/sql/sqlgraph"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestGraphReturning(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(ctx, "CREATE TABLE users(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT UNIQUE NOT NULL, age INTEGER DEFAULT 42)", nil)
	require.NoError(t, err)
	nodes := []*sqlgraph.CreateSpec{
		{Table: "users", ID: &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt}, Fields: []*sqlgraph.FieldSpec{{Column: "name", Type: field.TypeString, Value: "first"}}},
		{Table: "users", ID: &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt}, Fields: []*sqlgraph.FieldSpec{{Column: "name", Type: field.TypeString, Value: "second"}, {Column: "age", Type: field.TypeInt, Value: 20}}},
	}
	require.NoError(t, sqlgraph.BatchCreate(ctx, db, &sqlgraph.BatchCreateSpec{Nodes: nodes}))
	require.EqualValues(t, 1, nodes[0].ID.Value)
	require.EqualValues(t, 2, nodes[1].ID.Value)
	created := &sqlgraph.CreateSpec{Table: "users", ID: &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt}, Fields: []*sqlgraph.FieldSpec{{Column: "name", Type: field.TypeString, Value: "third"}}}
	var age int
	created.Returning = &sqlgraph.Returning{Columns: []string{"id", "age"}, Scan: func(rows dialect.Rows) error {
		var id int64
		if err := rows.Scan(&id, &age); err != nil {
			return err
		}
		created.ID.Value = id
		return nil
	}}
	require.NoError(t, sqlgraph.CreateNode(ctx, db, created))
	require.Equal(t, 42, age)
	update := sqlgraph.NewUpdateSpec("users", []string{"age"}, nodes[0].ID)
	update.SetField("age", field.TypeInt, 43)
	update.ScanValues = func([]string) ([]any, error) { return []any{&age}, nil }
	update.Assign = func([]string, []any) error { return nil }
	require.NoError(t, sqlgraph.UpdateNode(ctx, db, update))
	require.Equal(t, 43, age)
	returned := []int{}
	returning := &sqlgraph.Returning{Columns: []string{"age"}, Scan: func(rows dialect.Rows) error {
		var age int
		if err := rows.Scan(&age); err != nil {
			return err
		}
		returned = append(returned, age)
		return nil
	}}
	update = sqlgraph.NewUpdateSpec("users", nil, &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt})
	update.SetField("age", field.TypeInt, 50)
	update.Returning = returning
	count, err := sqlgraph.UpdateNodes(ctx, db, update)
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.Equal(t, []int{50, 50, 50}, returned)
	returned = nil
	count, err = sqlgraph.DeleteNodes(ctx, db, &sqlgraph.DeleteSpec{Node: &sqlgraph.NodeSpec{Table: "users"}, Returning: returning})
	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.Equal(t, []int{50, 50, 50}, returned)
	migration, err := schema.NewMigrate(db)
	require.NoError(t, err)
	table := schema.NewTable("migrated").AddPrimary(&schema.Column{Name: "id", Type: field.TypeInt, Increment: true})
	require.NoError(t, migration.Create(ctx, table))
	rows, err := db.Query(ctx, "INSERT INTO migrated DEFAULT VALUES RETURNING id", nil)
	require.NoError(t, err)
	defer rows.Close()
	require.NoError(t, sql.ScanOne(rows, &age))
	require.Equal(t, 1, age)
}

func TestDriver(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	_, err = db.Exec(ctx, "CREATE TABLE records (id INTEGER PRIMARY KEY, name TEXT UNIQUE NOT NULL, data BLOB, created DATETIME, active BOOLEAN, optional INTEGER, count INTEGER CHECK(count > 0), parent INTEGER REFERENCES records(id))", nil)
	require.NoError(t, err)
	now := time.Date(2026, 9, 4, 12, 34, 56, 123456000, time.UTC)
	rows, err := db.Query(ctx, "INSERT INTO records (name,data,created,active,count) VALUES (?,?,?,?,?) RETURNING id", []any{"first", []byte{0, 1, 255}, now, true, 1})
	require.NoError(t, err)
	id, err := sql.ScanInt(rows)
	require.NoError(t, err)
	require.Equal(t, 1, id)
	require.NoError(t, rows.Close())
	rows, err = db.Query(ctx, "SELECT id,name,data,created,active,optional FROM records", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var name string
	var data []byte
	var created time.Time
	var active bool
	optional := new(int)
	require.NoError(t, rows.Scan(&id, &name, &data, &created, &active, &optional))
	require.Equal(t, "first", name)
	require.Equal(t, []byte{0, 1, 255}, data)
	require.True(t, now.Equal(created))
	require.True(t, active)
	require.Nil(t, optional)
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	for _, commit := range []bool{false, true} {
		tx, err := db.BeginTx(ctx, &dialect.TxOptions{})
		require.NoError(t, err)
		result, err := tx.Exec(ctx, "UPDATE records SET optional = 42", nil)
		require.NoError(t, err)
		affected, err := result.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 1, affected)
		if commit {
			require.NoError(t, tx.Commit())
		} else {
			require.NoError(t, tx.Rollback())
		}
		rows, err := db.Query(ctx, "SELECT optional FROM records", nil)
		require.NoError(t, err)
		require.NoError(t, sql.ScanOne(rows, &optional))
		require.NoError(t, rows.Close())
		if commit {
			require.Equal(t, 42, *optional)
		} else {
			require.Nil(t, optional)
		}
	}
	for _, test := range []struct {
		query string
		kind  dialect.ConstraintKind
	}{
		{"INSERT INTO records(name) VALUES ('first')", dialect.Unique},
		{"INSERT INTO records(id,name) VALUES (1,'other')", dialect.Unique},
		{"INSERT INTO records(name) VALUES (NULL)", dialect.NotNull},
		{"INSERT INTO records(name,count) VALUES ('check',0)", dialect.Check},
		{"INSERT INTO records(name,parent) VALUES ('foreign',999)", dialect.ForeignKey},
	} {
		_, err := db.Exec(ctx, test.query, nil)
		constraint, ok := errors.AsType[*dialect.ConstraintError](err)
		require.True(t, ok, "%v", err)
		require.Equal(t, test.kind, constraint.Kind)
	}
	require.Same(t, db.DB(), db.DB())
	require.Equal(t, dialect.Capabilities{}, db.Capabilities())
}
