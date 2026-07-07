// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/mixin"
)

var (
	incrementalDisabled = false
)

type Mixin struct {
	mixin.Schema
}

// Annotations of the Mixin schema.
func (Mixin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Charset: "utf8mb4"},
	}
}

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User schema.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			StorageKey("user_id").
			Annotations(entsql.Annotation{
				Incremental: &incrementalDisabled,
			}),
		field.String("name").
			Optional().
			Annotations(entsql.Annotation{
				Size: 128,
			}).Comment(`Name of the user.
Comment line1
Comment line2`),
		field.String("label").
			Optional(),
	}
}

// Mixin of the User schema.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		Mixin{},
	}
}

// Annotations of the User schema.
func (User) Annotations() []schema.Annotation {
	incremental := false
	return []schema.Annotation{
		entsql.Annotation{
			Table:       "Users",
			Incremental: &incremental,
		},
	}
}
