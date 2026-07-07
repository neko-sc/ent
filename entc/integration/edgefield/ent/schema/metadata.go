// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Metadata holds the schema definition for the Metadata entity.
type Metadata struct {
	ent.Schema
}

func (Metadata) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.Int("age").
			Default(0),
		field.Int("parent_id").
			Optional(),
	}
}
func (Metadata) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("metadata").
			Unique(),
		edge.To("parent", Metadata.Type).
			Field("parent_id").
			Unique().
			From("children"),
	}
}
