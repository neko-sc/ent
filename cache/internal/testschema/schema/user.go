// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

type Profile struct {
	Biography string   `json:"biography"`
	Links     []string `json:"links"`
}

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
		field.String("email").Unique(),
		field.Int("age"),
		field.String("nickname").Optional().Nillable(),
		field.Enum("role").Values("admin", "member"),
		field.Array[[]string]("tags"),
		field.JSON[Profile]("profile"),
		field.Time("created_at").DefaultFunc(time.Now),
		field.Bytes("avatar").Optional(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pets", Pet.Type),
		edge.From("groups", Group.Type).Ref("users"),
	}
}
