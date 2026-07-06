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
		field.Int("priority").
			GoType(task.Priority(0)).
			Default(int(task.PriorityMid)),
		field.JSON("priorities", map[string]task.Priority{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
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
