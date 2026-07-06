// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			StorageKey("oid"),
		field.Int32("age"),
		field.String("name").
			MaxLen(10),
		field.Text("description").
			Optional(),
		field.String("nickname").
			Unique(),
		field.String("address").
			Optional(),
		field.String("renamed").
			Optional(),
		field.String("old_token").
			DefaultFunc(uuid.NewString),
		field.Bytes("blob").
			Optional().
			MaxLen(255),
		field.Enum("state").
			Optional().
			Values("logged_in", "logged_out").
			Default("logged_in"),
		field.String("status").
			Optional(),
		field.String("workplace").
			MaxLen(30).
			Optional(),
		field.String("drop_optional").
			Optional(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", User.Type).
			From("parent").
			Unique(),
		edge.To("spouse", User.Type).
			Unique(),
		edge.To("car", Car.Type).
			Unique(),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("description").
			Annotations(entsql.Prefix(50)),
		index.Fields("name", "address").
			Unique(),
	}
}

type Car struct {
	ent.Schema
}

// Annotations of the Car.
func (Car) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "Car"},
	}
}

func (Car) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("car").
			Unique(),
	}
}
