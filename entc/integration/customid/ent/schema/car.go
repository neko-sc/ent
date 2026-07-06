// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/mixin"
)

type IDMixin struct {
	mixin.Schema
}

func (IDMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Float("before_id").
			Optional().
			Positive(),
		field.Int("id").
			Positive().
			Immutable(),
		field.Float("after_id").
			Optional().
			Negative(),
	}
}

// Car holds the schema definition for the Car entity.
type Car struct {
	ent.Schema
}

// Mixin of the Car.
func (Car) Mixin() []ent.Mixin {
	return []ent.Mixin{
		IDMixin{},
	}
}

// Fields of the Car.
func (Car) Fields() []ent.Field {
	return []ent.Field{
		field.String("model"),
	}
}

// Edges of the Car.
func (Car) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", Pet.Type).
			Ref("cars").
			Unique(),
	}
}
