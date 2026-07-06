// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	schemadir "github.com/neko-sc/ent/entc/integration/ent/schema/dir"
	"github.com/neko-sc/ent/schema/field"
)

// Comment holds the schema definition for the Comment entity.
type Comment struct {
	ent.Schema
}

// Fields of the Comment.
func (Comment) Fields() []ent.Field {
	return []ent.Field{
		field.Int("unique_int").
			Unique(),
		field.Float("unique_float").
			Unique(),
		field.Int("nillable_int").
			Optional().
			Nillable(),
		field.String("table").
			Optional(),
		field.JSON("dir", schemadir.Dir("")).
			Optional(),
		field.String("client").
			Optional(),
	}
}
