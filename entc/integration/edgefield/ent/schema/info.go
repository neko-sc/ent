// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"encoding/json"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Info holds the schema definition for the Info entity.
type Info struct {
	ent.Schema
}

func (Info) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.JSON("content", json.RawMessage{}),
	}
}

func (Info) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			StorageKey(edge.Column("id")),
	}
}
