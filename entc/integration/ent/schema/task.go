// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"

	"github.com/neko-sc/ent/entc/integration/ent/schema/task"
)

// Task holds the schema definition for the Task entity.
type Task struct {
	ent.Schema
}

// Fields of the Task.
func (Task) Fields() []ent.Field {
	return []ent.Field{
		field.IntAs[task.Priority]("priority").
			Default(task.PriorityMid),
		field.JSON[map[string]task.Priority]("priorities").
			Optional(),
		field.Time("created_at").
			DefaultFunc(time.Now).
			Immutable().
			Nillable(),
		field.String("name").
			Optional().
			Deprecated(),
		field.String("owner").
			Optional(),
		field.Int("order").
			Optional(),
		field.Int("order_option").
			Optional(),
		field.String("op").
			MaxLen(45).
			Default(""),
	}
}

// Indexes of the Task.
func (Task) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "owner").
			Unique(),
	}
}
