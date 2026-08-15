// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/entc/integration/customid/sid"
	"github.com/neko-sc/ent/schema/field"
)

// Other holds the schema definition for the Other entity.
type Other struct {
	ent.Schema
}

// Fields of the Other.
func (Other) Fields() []ent.Field {
	return []ent.Field{
		field.Other[sid.ID]("id").
			SchemaType(map[string]string{
				dialect.Postgres: "bigint",
				dialect.SQLite:   "integer",
			}).
			DefaultFunc(sid.New).
			Immutable(),
	}
}
