// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/require"
)

func TestPostgresType(t *testing.T) {
	for _, test := range []struct {
		name   string
		column *Column
		want   string
	}{
		{name: "int", column: &Column{Type: field.TypeInt}, want: "bigint"},
		{name: "string", column: &Column{Type: field.TypeString, Size: 255}, want: "text"},
		{name: "native enum", column: &Column{Type: field.TypeEnum, SchemaType: map[string]string{dialect.Postgres: "mood"}}, want: "mood"},
		{name: "array", column: &Column{Type: field.TypeArray, SchemaType: map[string]string{dialect.Postgres: "text[]"}}, want: "text[]"},
		{name: "bigserial", column: &Column{Type: field.TypeInt64, SchemaType: map[string]string{dialect.Postgres: "bigserial"}}, want: "bigint"},
		{name: "serial", column: &Column{Type: field.TypeInt, SchemaType: map[string]string{dialect.Postgres: "serial"}}, want: "integer"},
		{name: "parsed type", column: &Column{Type: field.TypeArray, typ: "integer[]"}, want: "integer[]"},
		{name: "varchar", column: &Column{Type: field.TypeString, SchemaType: map[string]string{dialect.Postgres: "varchar(3)"}}, want: "text"},
		{name: "char", column: &Column{Type: field.TypeString, SchemaType: map[string]string{dialect.Postgres: "char(2)"}}, want: "text"},
		{name: "unmodified char", column: &Column{Type: field.TypeString, SchemaType: map[string]string{dialect.Postgres: "character"}}, want: "text"},
		{name: "numeric", column: &Column{Type: field.TypeFloat64, SchemaType: map[string]string{dialect.Postgres: "numeric(10,2)"}}, want: "numeric"},
		{name: "timestamp precision", column: &Column{Type: field.TypeTime, SchemaType: map[string]string{dialect.Postgres: "timestamp(3) with time zone"}}, want: "timestamptz"},
		{name: "bit", column: &Column{Type: field.TypeString, SchemaType: map[string]string{dialect.Postgres: "bit(8)"}}, want: "bit varying"},
		{name: "varchar array", column: &Column{Type: field.TypeArray, SchemaType: map[string]string{dialect.Postgres: "varchar(10)[]"}}, want: "text[]"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := PostgresType(test.column)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
	_, err := PostgresType(&Column{Type: field.TypeOther, typ: "custom"})
	require.EqualError(t, err, `postgres: unsupported type: "custom"`)
}
