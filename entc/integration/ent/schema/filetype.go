// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// FileType holds the schema definition for the FileType entity.
type FileType struct {
	ent.Schema
}

// Fields of the FileType.
func (FileType) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Unique(),
		field.Enum("type").
			NamedValues(
				"PNG", "png",
				"SVG", "svg",
				"JPG", "jpg",
			).
			Default("png"),
		field.Enum("state").
			NamedValues(
				"On", "ON",
				"Off", "OFF",
			).
			Default("ON"),
	}
}

// Edges of the FileType.
func (FileType) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("files", File.Type),
	}
}
