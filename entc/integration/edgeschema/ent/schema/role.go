// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Role holds the schema definition for the Role entity.
type Role struct {
	ent.Schema
}

// Fields of the Role.
func (Role) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Unique(),
		field.Time("created_at").
			Default(time.Now),
	}
}

// Edges of the Role.
func (Role) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("roles").
			Through("roles_users", RoleUser.Type),
	}
}
