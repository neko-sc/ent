// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"math"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// File holds the schema definition for the File entity.
type File struct {
	ent.Schema
}

func (File) Annotations() []schema.Annotation {
	return []schema.Annotation{
		edge.Annotation{
			StructTag: `json:"file_edges"`,
		},
	}
}

// Fields of the File.
func (File) Fields() []ent.Field {
	return []ent.Field{
		field.Int("set_id").
			Max(10).
			Optional(),
		field.Int("size").
			StorageKey("fsize").
			Default(math.MaxInt32).
			Positive(),
		field.String("name"),
		field.String("user").
			Optional().
			Nillable(),
		field.String("group").
			Optional(),
		field.Bool("op").
			Optional(),
		// Skip generating the "FieldID" predicate
		// as it conflicts with the "FieldID" constant.
		field.Int("field_id").
			Optional(),
		field.Time("create_time").
			Optional().
			Unique(),
	}
}

// Edges of the File.
func (File) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("files").
			Unique(),
		edge.From("type", FileType.Type).
			Ref("files").
			Unique(),
		edge.To("field", FieldType.Type),
	}
}

// Indexes of a file.
func (File) Indexes() []ent.Index {
	return []ent.Index{
		// non-unique index should not prevent duplicates.
		index.Fields("name", "size").
			StorageKey("file_name_size"),
		// unique index prevents duplicates records.
		index.Fields("name", "user").
			Unique(),
		// index on edges only.
		index.Edges("owner", "type"),
		// unique index under the "owner" sub-tree.
		// user/owner can't have files with duplicate names.
		index.Fields("name").
			Edges("owner", "type").
			Unique(),
		index.Fields("name").
			Edges("owner"),
	}
}
