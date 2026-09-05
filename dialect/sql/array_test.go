package sql

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/dialect"
	"github.com/stretchr/testify/require"
)

type argumentRecorder struct {
	arguments []any
}

func (recorder *argumentRecorder) ExecContext(_ context.Context, _ string, arguments ...any) (sql.Result, error) {
	recorder.arguments = arguments
	return nil, nil
}

func (recorder *argumentRecorder) QueryContext(_ context.Context, _ string, arguments ...any) (*sql.Rows, error) {
	recorder.arguments = arguments
	return nil, nil
}

func TestArrayDialectBoundary(t *testing.T) {
	for _, name := range []dialect.Dialect{dialect.Postgres, dialect.SQLite} {
		t.Run(string(name), func(t *testing.T) {
			recorder := &argumentRecorder{}
			driver := NewDriver(name, Conn{ExecQuerier: recorder})
			arguments := []any{[]string{"one", "two"}}
			_, err := driver.Exec(t.Context(), "INSERT", arguments)
			require.NoError(t, err)
			if name == dialect.SQLite {
				require.Equal(t, []any{`["one","two"]`}, recorder.arguments)
			} else {
				require.Equal(t, arguments, recorder.arguments)
			}
			_, err = driver.Query(t.Context(), "SELECT", arguments)
			require.NoError(t, err)
			if name == dialect.SQLite {
				require.Equal(t, []any{`["one","two"]`}, recorder.arguments)
			} else {
				require.Equal(t, arguments, recorder.arguments)
			}
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"values"}).AddRow(`["one","two"]`))
			rows, err := OpenDB(name, db).Query(t.Context(), "SELECT", nil)
			require.NoError(t, err)
			defer rows.Close()
			require.True(t, rows.Next())
			var values []string
			err = rows.Scan(&values)
			if name == dialect.SQLite {
				require.NoError(t, err)
				require.Equal(t, []string{"one", "two"}, values)
			} else {
				require.Error(t, err, "non-SQLite rows must not JSON-decode slices")
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestArrayArguments(t *testing.T) {
	type strings []string
	input := []any{strings{"a", "b"}, []int{1, 2}, []byte("raw"), []string(nil), []string{}}
	result, err := arrayArguments(input)
	require.NoError(t, err)
	require.Equal(t, []any{`["a","b"]`, `[1,2]`, []byte("raw"), nil, `[]`}, result)
	require.IsType(t, strings{}, input[0])
}

func TestArrayScanner(t *testing.T) {
	type integers []int
	var values *integers
	scanner := arrayScanner{&values}
	require.NoError(t, scanner.Scan(`[1,2]`))
	require.Equal(t, integers{1, 2}, *values)
	require.NoError(t, scanner.Scan([]byte(`[]`)))
	require.NotNil(t, *values)
	require.Empty(t, *values)
	require.NoError(t, scanner.Scan(nil))
	require.Nil(t, values)
	require.Error(t, scanner.Scan(`invalid`))
	require.Error(t, scanner.Scan(42))
}
