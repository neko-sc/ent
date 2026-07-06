// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/google/uuid"
)

// Car holds the schema definition for the Car entity.
type Car struct {
	ent.Schema
}

// Fields of the Car.
func (Car) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("number").
			Optional(),
	}
}

// Edges of the Car.
func (Car) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("rentals", Rental.Type),
	}
}
