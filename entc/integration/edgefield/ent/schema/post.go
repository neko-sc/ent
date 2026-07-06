// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// Post holds the schema definition for the Post entity.
type Post struct {
	ent.Schema
}

// Fields of the Post.
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("text"),
		field.Int("author_id").
			Optional().
			Nillable(),
	}
}

// Edges of the Post.
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("author", User.Type).
			Field("author_id").
			Unique(),
	}
}

// Indexes of the Post.
func (Post) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("author_id", "text"),
	}
}
