// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// An example for an ent/schema that uses struct embedding to define default table schema (in the database).

// Group holds the schema definition for the Group entity.
type Group struct {
	base
}

// Fields of the Group.
func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Default("unknown"),
	}
}

// Edges of the Group.
func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		// The edge schema (join table) is defaults to the edge owner schema (Group).
		edge.To("users", User.Type),
	}
}
