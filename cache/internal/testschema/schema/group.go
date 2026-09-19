// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

type Group struct {
	ent.Schema
}

func (Group) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Group) Edges() []ent.Edge {
	return []ent.Edge{edge.To("users", User.Type)}
}
