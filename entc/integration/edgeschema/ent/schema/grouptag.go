// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// GroupTag holds the schema definition for the GroupTag entity.
type GroupTag struct {
	ent.Schema
}

// Fields of the GroupTag.
func (GroupTag) Fields() []ent.Field {
	return []ent.Field{
		// An edge schema with the builtin ID
		// field, but without any other field.
		field.Int("tag_id"),
		field.Int("group_id"),
	}
}

// Edges of the GroupTag.
func (GroupTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tag", Tag.Type).
			Unique().
			Required().
			Field("tag_id"),
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id"),
	}
}
