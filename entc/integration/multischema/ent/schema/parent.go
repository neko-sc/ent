// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Parent holds the schema definition for the Parent entity.
type Parent struct {
	base
}

// Fields of the Parent.
func (Parent) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("by_adoption").
			Default(false),
		field.Int("user_id").
			Immutable(),
		field.Int("parent_id").
			Immutable(),
	}
}

// Edges of the Parent.
func (Parent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("child", User.Type).
			Unique().
			Required().
			Immutable().
			Field("user_id"),
		edge.To("parent", User.Type).
			Unique().
			Required().
			Immutable().
			Field("parent_id"),
	}
}
