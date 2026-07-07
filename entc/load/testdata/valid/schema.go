// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package valid

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the user schema.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("age"),
		field.String("name"),
	}
}

// Group holds the group schema.
type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.Time("expired_at"),
		field.String("organization"),
	}
}

// Tag holds the tag schema.
type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.String("text"),
	}
}
