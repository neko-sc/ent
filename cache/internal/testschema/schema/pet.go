// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

type Pet struct {
	ent.Schema
}

func (Pet) Fields() []ent.Field {
	return []ent.Field{field.String("name")}
}

func (Pet) Edges() []ent.Edge {
	return []ent.Edge{edge.From("owner", User.Type).Ref("pets").Unique()}
}

func (Pet) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Edges("owner").Unique()}
}
