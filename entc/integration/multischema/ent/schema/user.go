// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	base
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Default("unknown"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pets", Pet.Type),
		edge.From("groups", Group.Type).
			Ref("users"),
		edge.To("friends", User.Type).
			Through("friendships", Friendship.Type),
		edge.To("children", User.Type).
			Through("parent_hood", Parent.Type).
			From("parents"),
	}
}

// CleanUser represents a user without its PII field.
type CleanUser struct {
	ent.View
}

// Annotations of the CleanUser.
func (CleanUser) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.View("SELECT id, name FROM users"),
		entsql.Schema("db1"),
	}
}

// Fields of the CleanUser.
func (CleanUser) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}
