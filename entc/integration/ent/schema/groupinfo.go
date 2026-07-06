// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// GroupInfo holds the schema for the group-info entity.
type GroupInfo struct {
	ent.Schema
}

// Fields of the group.
func (GroupInfo) Fields() []ent.Field {
	return []ent.Field{
		field.String("desc"),
		field.Int("max_users").
			Default(1e4),
	}
}

// Edges of the group.
func (GroupInfo) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("groups", Group.Type).
			Ref("info"),
	}
}
