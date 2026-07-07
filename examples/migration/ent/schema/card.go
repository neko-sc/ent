// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Card holds the schema definition for the Card entity.
type Card struct {
	ent.Schema
}

// Fields of the Card.
func (Card) Fields() []ent.Field {
	return []ent.Field{
		field.String("type").
			Default("unknown"),
		field.String("number_hash"),
		field.String("cvv_hash"),
		field.Time("expires_at").
			Optional(),
		field.Int("owner_id").
			Default(0),
	}
}

// Edges of the Card.
func (Card) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("cards").
			Unique().
			Required().
			Field("owner_id"),
		edge.To("payments", Payment.Type),
	}
}

// Annotations of the Card.
func (Card) Annotations() []schema.Annotation {
	return []schema.Annotation{
		// Named check constraints are compared by their name.
		// Thus, the definition does not need to be normalized.
		entsql.Checks(map[string]string{
			"number_hash_length": "(LENGTH(`number_hash`) = 16)",
		}),
	}
}
