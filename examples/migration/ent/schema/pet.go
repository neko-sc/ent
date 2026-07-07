// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
	"github.com/google/uuid"
)

// Pet holds the schema definition for the Pet entity.
type Pet struct {
	ent.Schema
}

// Fields of the Pet.
func (Pet) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.Nil).
			Default(uuid.New),
		field.String("name"),
		field.Float("age"),
		field.Float("weight"),
		field.UUID("best_friend_id", uuid.Nil).
			Annotations(
				entsql.Default(uuid.Nil.String()),
			),
		field.Int("owner_id").
			Default(0),
	}
}

// Edges of the Pet.
func (Pet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("best_friend", Pet.Type).
			Unique().
			Required().
			Field("best_friend_id"),
		edge.To("owner", User.Type).
			Unique().
			Required().
			Field("owner_id"),
	}
}

// Indexes of the Pet.
func (Pet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "owner_id").
			Unique(),
	}
}
