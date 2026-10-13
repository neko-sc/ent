package pg

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/dialect/sql/sqlgraph"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func postgresDSN(t *testing.T) string {
	t.Helper()
	if dsn := os.Getenv("PG_DSN"); dsn != "" {
		return dsn
	}
	connection, err := net.DialTimeout("tcp", "localhost:5438", time.Second)
	if err != nil {
		t.Skip("set PG_DSN or start PostgreSQL on localhost:5438")
	}
	require.NoError(t, connection.Close())
	return "host=localhost port=5438 user=postgres dbname=test password=pass sslmode=disable"
}

func postgresDriver(t *testing.T) *Driver {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	driver, err := Open(ctx, postgresDSN(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, driver.Close()) })
	return driver
}

func TestNewDriverCapabilities(t *testing.T) {
	require.Equal(t, dialect.Capabilities{NativeArray: true, MultiRowReturningOrdered: true}, NewDriver(nil).Capabilities())
	require.Equal(t, dialect.Capabilities{ReturningOld: true}, NewDriver(nil, WithCapabilities(dialect.Capabilities{ReturningOld: true})).Capabilities())
}

func TestOpenConfig(t *testing.T) {
	dsn := postgresDSN(t)
	config, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	config.MaxConns = 3
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	driver, err := OpenConfig(ctx, config, WithCapabilities(dialect.Capabilities{ConflictDoSelect: true}))
	require.NoError(t, err)
	defer func() { require.NoError(t, driver.Close()) }()
	require.Equal(t, int32(3), driver.Pool().Config().MaxConns)
	require.Equal(t, dialect.Capabilities{ConflictDoSelect: true}, driver.Capabilities(), "options must override detected capabilities")

	config, err = pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	detected, err := OpenConfig(ctx, config)
	require.NoError(t, err)
	defer func() { require.NoError(t, detected.Close()) }()
	var version int
	require.NoError(t, detected.Pool().QueryRow(ctx, "SELECT current_setting('server_version_num')::int").Scan(&version))
	require.Equal(t, dialect.Capabilities{ReturningOld: version >= 180000, ConflictDoSelect: version >= 190000, NativeArray: true, MultiRowReturningOrdered: true}, detected.Capabilities())
}

type textArray []string

func (values *textArray) Scan(value any) error {
	if value == nil {
		*values = nil
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return fmt.Errorf("text array scanner received %T, want string", value)
	}
	return pgtype.NewMap().Scan(pgtype.TextArrayOID, pgtype.TextFormatCode, []byte(text), (*[]string)(values))
}

func (values textArray) Value() (driver.Value, error) {
	text, err := pgtype.NewMap().Encode(pgtype.TextArrayOID, pgtype.TextFormatCode, []string(values), nil)
	return string(text), err
}

func TestRowsScan(t *testing.T) {
	driver := postgresDriver(t)
	var (
		array      textArray
		identifier uuid.UUID
		number     int
		null       *int
		bytes      []byte
		message    json.RawMessage
		timestamp  time.Time
		nullTime   sql.NullTime
		native     pgtype.Int8
		text       pgtype.Text
	)
	rows, err := driver.Query(t.Context(), `SELECT $1::text[], $2::uuid, 42::int8, NULL::int8,
		'{"answer":42}'::jsonb, '{"answer":42}'::jsonb, $3::timestamptz, $3::timestamptz,
		43::int8, 'native'::text`, []any{textArray{"foo", "bar,baz", "quoted\""}, uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"), time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC)})
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	require.Equal(t, int16(pgtype.BinaryFormatCode), rows.(interface {
		FieldDescriptions() []pgconn.FieldDescription
	}).FieldDescriptions()[0].Format)
	require.NoError(t, rows.Scan(&array, &identifier, &number, &null, &bytes, &message, &timestamp, &nullTime, &native, &text))
	require.Equal(t, textArray{"foo", "bar,baz", "quoted\""}, array)
	require.Equal(t, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", identifier.String())
	require.Equal(t, 42, number)
	require.Nil(t, null)
	require.JSONEq(t, `{"answer":42}`, string(bytes))
	require.JSONEq(t, `{"answer":42}`, string(message))
	require.True(t, timestamp.Equal(time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC)))
	require.True(t, nullTime.Valid)
	require.True(t, nullTime.Time.Equal(timestamp))
	require.Equal(t, pgtype.Int8{Int64: 43, Valid: true}, native)
	require.Equal(t, pgtype.Text{String: "native", Valid: true}, text)
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())

	rows, err = driver.Query(t.Context(), "SELECT NULL::text[], 7::int8", nil)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&array, &null))
	require.Nil(t, array)
	require.Equal(t, 7, *null)
}

func TestConstraintMutations(t *testing.T) {
	native := postgresDriver(t)
	for name, driver := range map[string]dialect.Driver{"native": native, "stdlib": entsql.OpenDB(dialect.Postgres, native.DB())} {
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			_, err := driver.Exec(ctx, `CREATE TABLE ent_pg_constraint_parent (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, name text UNIQUE);
				CREATE TABLE ent_pg_constraint_child (parent_id bigint REFERENCES ent_pg_constraint_parent(id));
				INSERT INTO ent_pg_constraint_parent (name) VALUES ('first'), ('second');
				INSERT INTO ent_pg_constraint_child VALUES (1)`, nil)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := driver.Exec(context.Background(), "DROP TABLE ent_pg_constraint_child, ent_pg_constraint_parent", nil)
				require.NoError(t, err)
			})
			for _, returning := range []bool{false, true} {
				t.Run(fmt.Sprint("returning=", returning), func(t *testing.T) {
					create := sqlgraph.NewCreateSpec("ent_pg_constraint_parent", sqlgraph.NewFieldSpec("id", field.TypeInt64))
					create.SetField("name", field.TypeString, "first")
					if returning {
						create.Returning = &sqlgraph.Returning{Columns: []string{"id"}, Scan: func(rows dialect.Rows) error { return rows.Scan(new(int64)) }}
					}
					assertConstraint(t, sqlgraph.CreateNode(ctx, driver, create), dialect.Unique)
					assertConstraint(t, sqlgraph.BatchCreate(ctx, driver, &sqlgraph.BatchCreateSpec{Nodes: []*sqlgraph.CreateSpec{create, create}}), dialect.Unique)
					update := sqlgraph.NewUpdateSpec("ent_pg_constraint_parent", []string{"id"}, &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt64, Value: int64(2)})
					update.SetField("name", field.TypeString, "first")
					update.Returning = create.Returning
					assertConstraint(t, sqlgraph.UpdateNode(ctx, driver, update), dialect.Unique)
					update.Predicate = func(selector *entsql.Selector) { selector.Where(entsql.EQ("id", 2)) }
					_, err := sqlgraph.UpdateNodes(ctx, driver, update)
					assertConstraint(t, err, dialect.Unique)
					deletion := sqlgraph.NewDeleteSpec("ent_pg_constraint_parent", sqlgraph.NewFieldSpec("id", field.TypeInt64))
					deletion.Returning = create.Returning
					_, err = sqlgraph.DeleteNodes(ctx, driver, deletion)
					assertConstraint(t, err, dialect.ForeignKey)
				})
			}
		})
	}
}

func assertConstraint(t *testing.T, err error, kind dialect.ConstraintKind) {
	t.Helper()
	require.True(t, sqlgraph.IsConstraintError(err), "%T: %v", err, err)
	constraint, ok := errors.AsType[*dialect.ConstraintError](err)
	require.True(t, ok)
	require.Equal(t, kind, constraint.Kind)
	_, ok = errors.AsType[*pgconn.PgError](err)
	require.True(t, ok)
}

type mood string

// TestUnnestAnyArrays pins the encoding contract of batch updates: one ArrayArgument per column with NULL elements.
func TestUnnestAnyArrays(t *testing.T) {
	native := postgresDriver(t)
	_, err := native.Exec(t.Context(), "CREATE TYPE ent_pg_unnest_mood AS ENUM ('happy', 'sad')", nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := native.Exec(context.Background(), "DROP TYPE ent_pg_unnest_mood", nil)
		require.NoError(t, err)
	})
	tx, err := native.Tx(t.Context())
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback()) }()
	identifier := uuid.MustParse("a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11")
	timestamp := time.Date(2026, 9, 4, 12, 34, 56, 0, time.UTC)
	for name, querier := range map[string]dialect.ExecQuerier{"driver": native, "tx": tx} {
		t.Run(name, func(t *testing.T) {
			rows, err := querier.Query(t.Context(), `SELECT * FROM unnest($1::bigint[], $2::character varying[], $3::jsonb[], $4::uuid[],
				$5::timestamp with time zone[], $6::ent_pg_unnest_mood[], $7::ent_pg_unnest_mood[], $8::smallint[])`, []any{
				entsql.ArrayArgument{int64(1), nil},
				entsql.ArrayArgument{nil, "text"},
				entsql.ArrayArgument{json.RawMessage(`{"answer":42}`), nil},
				entsql.ArrayArgument{nil, identifier},
				entsql.ArrayArgument{timestamp, nil},
				entsql.ArrayArgument{nil, mood("sad")},
				entsql.ArrayArgument{nil, nil},
				[]int16{1, 3},
			})
			require.NoError(t, err)
			defer rows.Close()
			var (
				number    *int64
				text      *string
				message   []byte
				uuidValue *uuid.UUID
				moment    *time.Time
				enum      *string
				null      *string
				mode      int16
			)
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(&number, &text, &message, &uuidValue, &moment, &enum, &null, &mode))
			require.Equal(t, int64(1), *number)
			require.Nil(t, text)
			require.JSONEq(t, `{"answer":42}`, string(message))
			require.Nil(t, uuidValue)
			require.True(t, moment.Equal(timestamp))
			require.Nil(t, enum)
			require.Nil(t, null)
			require.Equal(t, int16(1), mode)
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(&number, &text, &message, &uuidValue, &moment, &enum, &null, &mode))
			require.Nil(t, number)
			require.Equal(t, "text", *text)
			require.Nil(t, message)
			require.Equal(t, identifier, *uuidValue)
			require.Nil(t, moment)
			require.Equal(t, "sad", *enum)
			require.Nil(t, null)
			require.Equal(t, int16(3), mode)
			require.False(t, rows.Next())
			require.NoError(t, rows.Err())
		})
	}
}

// TestSliceArgument checks that plain []any arguments reach pgx, which encodes them as JSON.
func TestSliceArgument(t *testing.T) {
	native := postgresDriver(t)
	tx, err := native.Tx(t.Context())
	require.NoError(t, err)
	defer func() { require.NoError(t, tx.Rollback()) }()
	for name, querier := range map[string]dialect.ExecQuerier{"driver": native, "tx": tx} {
		t.Run(name, func(t *testing.T) {
			rows, err := querier.Query(t.Context(), "SELECT $1::jsonb", []any{[]any{"a", 1, map[string]any{"b": true}}})
			require.NoError(t, err)
			defer rows.Close()
			var document []byte
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(&document))
			require.JSONEq(t, `["a", 1, {"b": true}]`, string(document))
			require.NoError(t, rows.Err())
		})
	}
}

func TestBatchUpdate(t *testing.T) {
	driver := postgresDriver(t)
	ctx := t.Context()
	_, err := driver.Exec(ctx, `CREATE TYPE ent_pg_batch_mood AS ENUM ('happy', 'sad');
		CREATE TABLE ent_pg_batch (id bigint PRIMARY KEY, mood ent_pg_batch_mood, tags text[]);
		INSERT INTO ent_pg_batch (id, mood) VALUES (1, 'happy'), (2, 'happy')`, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := driver.Exec(context.Background(), "DROP TABLE ent_pg_batch; DROP TYPE ent_pg_batch_mood", nil)
		require.NoError(t, err)
	})
	returned := make(map[int64][]any)
	node := func(id int) *sqlgraph.UpdateSpec {
		spec := sqlgraph.NewUpdateSpec("ent_pg_batch", []string{"id", "mood", "tags"}, &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt, Value: id})
		spec.ScanValues = func([]string) ([]any, error) { return []any{new(int64), new(sql.NullString), new(textArray)}, nil }
		spec.Assign = func(_ []string, values []any) error {
			returned[*values[0].(*int64)] = []any{*values[1].(*sql.NullString), *values[2].(*textArray)}
			return nil
		}
		return spec
	}
	// Array columns cannot be unnested and use a VALUES list, which types the native enum by its first row.
	first, second := node(1), node(2)
	first.SetField("tags", field.TypeArray, textArray{"a", "b"})
	second.SetField("tags", field.TypeArray, textArray{"c"})
	second.SetField("mood", field.TypeEnum, mood("sad"))
	types := map[string]string{"id": "bigint", "mood": "ent_pg_batch_mood", "tags": "text[]"}
	require.NoError(t, sqlgraph.BatchUpdate(ctx, driver, &sqlgraph.BatchUpdateSpec{Nodes: []*sqlgraph.UpdateSpec{first, second}, PostgresTypes: types}))
	require.Equal(t, map[int64][]any{
		1: {sql.NullString{String: "happy", Valid: true}, textArray{"a", "b"}},
		2: {sql.NullString{String: "sad", Valid: true}, textArray{"c"}},
	}, returned)
}

// TestBatchUpdateTable updates a table named like the row source, with a column named like a row source column.
func TestBatchUpdateTable(t *testing.T) {
	driver := postgresDriver(t)
	ctx := t.Context()
	_, err := driver.Exec(ctx, `CREATE SCHEMA ent_pg_batch_table;
		CREATE TABLE ent_pg_batch_table.batch (id bigint PRIMARY KEY, code varchar(3), value1 bigint);
		INSERT INTO ent_pg_batch_table.batch (id, code, value1) VALUES (1, 'a', 10), (2, 'b', 20)`, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := driver.Exec(context.Background(), "DROP SCHEMA ent_pg_batch_table CASCADE", nil)
		require.NoError(t, err)
	})
	code, err := schema.PostgresType(&schema.Column{Type: field.TypeString, SchemaType: map[string]string{dialect.Postgres: "varchar(3)"}})
	require.NoError(t, err)
	types := map[string]string{"id": "bigint", "code": code, "value1": "bigint"}
	returned := make(map[int64][]any)
	node := func(id int) *sqlgraph.UpdateSpec {
		spec := sqlgraph.NewUpdateSpec("batch", []string{"id", "code", "value1"}, &sqlgraph.FieldSpec{Column: "id", Type: field.TypeInt, Value: id})
		spec.Node.Schema = "ent_pg_batch_table"
		spec.ScanValues = func([]string) ([]any, error) { return []any{new(int64), new(string), new(int64)}, nil }
		spec.Assign = func(_ []string, values []any) error {
			returned[*values[0].(*int64)] = []any{*values[1].(*string), *values[2].(*int64)}
			return nil
		}
		return spec
	}
	first, second := node(1), node(2)
	first.SetField("code", field.TypeString, "abc")
	first.AddField("value1", field.TypeInt, 1)
	second.AddField("value1", field.TypeInt, 2)
	require.NoError(t, sqlgraph.BatchUpdate(ctx, driver, &sqlgraph.BatchUpdateSpec{Nodes: []*sqlgraph.UpdateSpec{first, second}, PostgresTypes: types}))
	require.Equal(t, map[int64][]any{1: {"abc", int64(11)}, 2: {"b", int64(22)}}, returned)

	// The row source must not truncate values that the column rejects.
	first = node(1)
	first.SetField("code", field.TypeString, "abcdef")
	err = sqlgraph.BatchUpdate(ctx, driver, &sqlgraph.BatchUpdateSpec{Nodes: []*sqlgraph.UpdateSpec{first}, PostgresTypes: types})
	require.ErrorContains(t, err, "value too long for type character varying(3)")
	var stored string
	require.NoError(t, driver.Pool().QueryRow(ctx, "SELECT code FROM ent_pg_batch_table.batch WHERE id = 1").Scan(&stored))
	require.Equal(t, "abc", stored)
}

func TestAnyArrayValue(t *testing.T) {
	_, err := anyArray{"sad", int64(1)}.Value()
	require.EqualError(t, err, "pg: array element 1 has type int64, want string")
}

type recordingExecutor struct {
	queries   []string
	arguments [][]any
	err       error
}

func (executor *recordingExecutor) Exec(_ context.Context, query string, arguments ...any) (pgconn.CommandTag, error) {
	executor.queries = append(executor.queries, query)
	executor.arguments = append(executor.arguments, arguments)
	return pgconn.CommandTag{}, executor.err
}

func TestSetVars(t *testing.T) {
	for _, local := range []bool{false, true} {
		t.Run(fmt.Sprint(local), func(t *testing.T) {
			executor := &recordingExecutor{}
			cleanup, err := setVars(entsql.WithVar(entsql.WithVar(t.Context(), "app.user", "1"), "app.role", "admin"), executor, local)
			require.NoError(t, err)
			require.Equal(t, []string{"SELECT set_config($1, $2, $3), set_config($4, $5, $6)"}, executor.queries)
			require.Equal(t, []any{"app.user", "1", local, "app.role", "admin", local}, executor.arguments[0])
			require.NoError(t, cleanup())
			if local {
				require.Len(t, executor.queries, 1)
			} else {
				require.Equal(t, `RESET "app.user"; RESET "app.role"`, executor.queries[1])
				require.Equal(t, []any{pgx.QueryExecModeSimpleProtocol}, executor.arguments[1])
			}
		})
	}
	executor := &recordingExecutor{err: errors.New("set failed")}
	_, err := setVars(entsql.WithVar(t.Context(), "app.user", "1"), executor, false)
	require.ErrorIs(t, err, executor.err)
	require.Len(t, executor.queries, 1)
}

func TestSessionVariables(t *testing.T) {
	driver := postgresDriver(t)
	connection, err := driver.Pool().Acquire(t.Context())
	require.NoError(t, err)
	defer connection.Release()
	cleanup, err := setVars(entsql.WithVar(entsql.WithVar(t.Context(), "app.user", "one"), "app.role", "admin"), connection, false)
	require.NoError(t, err)
	var user, role string
	require.NoError(t, connection.QueryRow(t.Context(), "SELECT current_setting('app.user'), current_setting('app.role')").Scan(&user, &role))
	require.Equal(t, "one", user)
	require.Equal(t, "admin", role)
	require.NoError(t, cleanup())
	require.NoError(t, connection.QueryRow(t.Context(), "SELECT current_setting('app.user'), current_setting('app.role')").Scan(&user, &role))
	require.Empty(t, user)
	require.Empty(t, role)
	_, err = setVars(entsql.WithVar(entsql.WithVar(t.Context(), "app.user", "one"), "invalid_setting", "value"), connection, false)
	require.Error(t, err)
	require.NoError(t, connection.QueryRow(t.Context(), "SELECT current_setting('app.user')").Scan(&user))
	require.Empty(t, user, "a failed batched statement must roll back every setting")
}

func TestRowsScanReuse(t *testing.T) {
	nativeDriver := postgresDriver(t)
	result, err := nativeDriver.Query(t.Context(), `SELECT ARRAY['one', 'two']::text[], 42::int8 FROM generate_series(1, 4)`, nil)
	require.NoError(t, err)
	defer result.Close()
	var first textArray
	var value driver.Value
	require.True(t, result.Next())
	require.NoError(t, result.Scan(&first, &value))
	require.Equal(t, textArray{"one", "two"}, first)
	require.Equal(t, int64(42), value)
	scratch := &result.(*rows).targets[0]
	var second textArray
	var secondValue driver.Value
	require.True(t, result.Next())
	require.NoError(t, result.Scan(&second, &secondValue))
	require.Same(t, scratch, &result.(*rows).targets[0])
	require.Equal(t, first, second)
	require.Equal(t, value, secondValue)
	var native []string
	var unknown entsql.UnknownType
	require.True(t, result.Next())
	require.NoError(t, result.Scan(&native, &unknown))
	require.Equal(t, []string{"one", "two"}, native)
	require.Equal(t, int64(42), unknown)
	require.True(t, result.Next())
	require.NoError(t, result.Scan(&second, &value))
	require.Equal(t, textArray{"one", "two"}, second)
	require.False(t, result.Next())
	require.NoError(t, result.Err())
}

func TestMapError(t *testing.T) {
	for code, kind := range map[string]dialect.ConstraintKind{"23505": dialect.Unique, "23503": dialect.ForeignKey, "23514": dialect.Check, "23502": dialect.NotNull, "23P01": dialect.Exclusion} {
		t.Run(code, func(t *testing.T) {
			original := &pgconn.PgError{Code: code, ConstraintName: "users_email_key", TableName: "users"}
			err := mapError(fmt.Errorf("query: %w", original))
			constraint, ok := errors.AsType[*dialect.ConstraintError](err)
			require.True(t, ok)
			require.Equal(t, kind, constraint.Kind)
			require.Equal(t, "users_email_key", constraint.Constraint)
			require.Equal(t, "users", constraint.Table)
			require.ErrorIs(t, err, original)
		})
	}
	require.ErrorIs(t, mapError(pgx.ErrNoRows), dialect.ErrNoRows)
	original := errors.New("connection failed")
	require.Same(t, original, mapError(original))
	require.NoError(t, mapError(nil))
}
