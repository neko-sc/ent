// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// File holds the schema definition for the File entity.
type File struct {
	ent.Schema
}

// Fields of the File.
func (File) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.Bool("deleted").
			Default(false),
		field.Int("parent_id").
			Optional(),
	}
}

// Edges of the File.
func (File) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", File.Type).
			From("parent").
			Unique().
			Field("parent_id"),
	}
}
