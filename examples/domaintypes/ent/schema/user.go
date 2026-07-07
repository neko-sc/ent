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
		field.String("postal_code").
			SchemaType(map[string]string{
				// Set the database column type to "us_postal_code" only in PostgreSQL.
				// In case this schema is used with other databases, it falls back to the
				// default type (e.g., "varchar").
				dialect.Postgres: "us_postal_code",
			}),
	}
}
