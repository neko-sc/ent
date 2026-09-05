package schema

import (
	"github.com/neko-sc/atlas/sql/postgres"
	atlas "github.com/neko-sc/atlas/sql/schema"
	"github.com/neko-sc/atlas/sql/sqlite"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestArrayColumnType(t *testing.T) {
	column := &Column{Name: "tags", Type: field.TypeArray, SchemaType: map[string]string{dialect.Postgres: "text[]", dialect.SQLite: "json"}}
	postgresColumn := &atlas.Column{Type: &atlas.ColumnType{}}
	require.NoError(t, (&Postgres{}).atTypeC(column, postgresColumn))
	postgresType, err := postgres.FormatType(postgresColumn.Type.Type)
	require.NoError(t, err)
	require.Equal(t, "text[]", postgresType)
	sqliteColumn := &atlas.Column{Type: &atlas.ColumnType{}}
	require.NoError(t, (&SQLite{}).atTypeC(column, sqliteColumn))
	sqliteType, err := sqlite.FormatType(sqliteColumn.Type.Type)
	require.NoError(t, err)
	require.Equal(t, "json", sqliteType)
}
