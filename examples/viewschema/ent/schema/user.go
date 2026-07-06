// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("public_info"),
		field.String("private_info"),
	}
}

// CleanUser represents a user without its PII field.
type CleanUser struct {
	ent.View
}

// Annotations of the CleanUser.
func (CleanUser) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.ViewFor(dialect.Postgres, func(s *sql.Selector) {
			s.Select("id", "name", "public_info").From(sql.Table("users"))
		}),
	}
}

// Fields of the CleanUser.
func (CleanUser) Fields() []ent.Field {
	return []ent.Field{
		// Note, unlike real schemas (tables, defined with ent.Schema),
		// the "id" field should be defined manually if needed.
		field.Int("id"),
		field.String("name"),
		field.String("public_info"),
	}
}
