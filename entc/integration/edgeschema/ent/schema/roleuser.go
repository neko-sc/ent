// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// RoleUser holds the schema definition for the RoleUser entity.
type RoleUser struct {
	ent.Schema
}

func (RoleUser) Annotations() []schema.Annotation {
	return []schema.Annotation{
		field.ID("user_id", "role_id"),
	}
}

// Fields of the RoleUser.
func (RoleUser) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now),
		// Edge fields.
		field.Int("role_id"),
		field.Int("user_id"),
	}
}

// Edges of the RoleUser.
func (RoleUser) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("role", Role.Type).
			Unique().
			Required().
			Field("role_id"),
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id"),
	}
}
