// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/integration/customid/sid"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// IntSID holds the schema definition for the IntSID entity.
type IntSID struct {
	ent.Schema
}

// Fields of the IntSid.
func (IntSID) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			GoType(sid.New()).
			Unique().
			Immutable(),
	}
}

// Edges of the IntSid.
func (IntSID) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("parent", IntSID.Type).
			Unique(),
		edge.From("children", IntSID.Type).
			Ref("parent"),
	}
}
