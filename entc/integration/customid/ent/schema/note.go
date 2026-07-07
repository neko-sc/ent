// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/google/uuid"
)

type NoteID string

// Note holds the schema definition for the Note entity.
type Note struct {
	ent.Schema
}

// Fields of the Note.
func (Note) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			GoType(NoteID("")).
			MaxLen(36).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(func() NoteID {
				return NoteID(uuid.NewString())
			}),
		field.String("text").
			Optional(),
	}
}

// Edges of the Note.
func (Note) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("children", Note.Type).
			From("parent").
			Unique(),
	}
}
