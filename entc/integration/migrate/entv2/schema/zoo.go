// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema/field"
)

// Zoo holds the schema definition for the Zoo entity.
type Zoo struct {
	ent.Schema
}

// Fields of the Zoo.
func (Zoo) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Annotations(
				entsql.DefaultExprs(map[string]string{
					dialect.SQLite:   "abs(random())",
					dialect.Postgres: "floor(random() * ~(1<<31))",
				}),
			),
	}
}
