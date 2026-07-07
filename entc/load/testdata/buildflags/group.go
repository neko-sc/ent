//go:build !hidegroups

// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package buildflags

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

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
