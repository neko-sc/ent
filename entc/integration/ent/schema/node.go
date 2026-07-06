// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Node holds the schema definition for the linked-list Node entity.
type Node struct {
	ent.Schema
}

// Fields of the Node.
func (Node) Fields() []ent.Field {
	return []ent.Field{
		field.Int("value").
			Optional(),
		field.Time("updated_at").
			Nillable().
			Optional().
			UpdateDefault(time.Now),
	}
}

// Edges of the Node.
func (Node) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("next", Node.Type).
			StructTag(`gqlgen:"next"`).
			Unique().
			From("prev").
			StructTag(`gqlgen:"prev"`).
			Unique(),
	}
}
