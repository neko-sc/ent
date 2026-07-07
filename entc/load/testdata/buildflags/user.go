// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package buildflags

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
