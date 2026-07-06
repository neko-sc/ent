// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Pet holds the schema definition for the Pet entity.
type Pet struct {
	ent.Schema
}

// Fields of the Pet.
func (Pet) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

// Edges of the Pet.
func (Pet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("friends", Pet.Type),
		edge.From("owner", User.Type).
			Ref("pets").
			Unique(),
	}
}
