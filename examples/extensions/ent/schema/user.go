// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Bytes("location").
			// Ideally, we would use a custom GoType
			// to represent the "geometry" type.
			SchemaType(map[string]string{
				dialect.Postgres: "GEOMETRY(Point, 4326)",
			}),
	}
}
