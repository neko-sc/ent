// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/google/uuid"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// Rental holds the schema definition for the Rental entity.
type Rental struct {
	ent.Schema
}

// Fields of the Rental.
func (Rental) Fields() []ent.Field {
	return []ent.Field{
		field.Time("date").
			Default(time.Now),
		field.Int("user_id").
			Immutable(),
		field.UUID("car_id", uuid.UUID{}).
			Immutable(),
	}
}

// Edges of the Rental.
func (Rental) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("rentals").
			Field("user_id").
			Unique().
			Required().
			Immutable(),
		edge.From("car", Car.Type).
			Ref("rentals").
			Field("car_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the Rental.
func (Rental) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("car_id", "user_id").
			Unique(),
	}
}
