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

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Float("age"),
		field.String("first_name"),
		field.String("last_name"),
		field.Strings("tags").
			Optional(),
	}
}

// Annotations of the User.
func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		// In case schema.ModeInspect is used without a dev-database, unnamed check constraints
		// should be normalized (i.e. identical to their definition in the database). In this
		// case, it is entsql.Check("(`age` > 0)"). See: https://atlasgo.io/concepts/dev-database.
		entsql.Check("age > 0"),

		// Named check constraints are compared by their name.
		// Thus, the definition does not need to be normalized.
		entsql.Checks(map[string]string{
			"first_last_not_empty": "(`first_name` <> '' AND `last_name` <> '')",
		}),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("cards", Card.Type),
	}
}
