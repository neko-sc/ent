// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package sql

import (
	"testing"

	"github.com/neko-sc/ent/dialect"
)

func BenchmarkInsertBuilder_Default(b *testing.B) {
	for _, d := range []string{dialect.SQLite, dialect.Postgres} {
		b.Run(d, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				Dialect(d).Insert("users").Default().Returning("id").Query()
			}
		})
	}
}

func BenchmarkInsertBuilder_Small(b *testing.B) {
	for _, d := range []string{dialect.SQLite, dialect.Postgres} {
		b.Run(d, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				Dialect(d).Insert("users").
					Columns("id", "age", "first_name", "last_name", "nickname", "spouse_id", "created_at", "updated_at").
					Values(1, 30, "Ariel", "Mashraki", "a8m", 2, "2009-11-10 23:00:00", "2009-11-10 23:00:00").
					Returning("id").
					Query()
			}
		})
	}
}
