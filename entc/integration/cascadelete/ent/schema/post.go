// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Post holds the schema definition for the Post entity.
type Post struct {
	ent.Schema
}

// Fields of the Post.
func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.String("text").
			Default("What's on your mind?"),
		field.Int("author_id").
			Optional(),
	}
}

// Edges of the Post.
func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("author", User.Type).
			Ref("posts").
			Field("author_id").
			Unique(),
		edge.To("comments", Comment.Type).
			Annotations(entsql.Annotation{
				OnDelete: entsql.Cascade,
			}),
	}
}
