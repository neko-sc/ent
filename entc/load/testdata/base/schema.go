// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package base

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

// base schema for sharing fields and edges.
type base struct {
	ent.Schema
}

func (base) Fields() []ent.Field {
	return []ent.Field{
		field.Int("base_field"),
	}
}

// User holds the user schema.
type User struct {
	base
}

func (u User) Fields() []ent.Field {
	return append(
		u.base.Fields(),
		field.String("user_field"),
	)
}
