// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// Media holds the schema definition for the Media entity.
type Media struct {
	ent.Schema
}

// Fields of the Media.
func (Media) Fields() []ent.Field {
	return []ent.Field{
		field.String("source").
			Optional(),
		field.String("source_uri").
			Optional().
			Comment("source_ui text").
			Annotations(entsql.WithComments(false)),
		field.Text("text").
			Optional().
			Comment("media text"),
	}
}

// Indexes of the Media.
func (Media) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "source_uri").
			Annotations(entsql.PrefixColumn("source", 100)).
			Unique(),
		index.Fields("text").
			Annotations(entsql.Prefix(100)),
	}
}

// Annotations of the Media.
func (Media) Annotations() []schema.Annotation {
	return []schema.Annotation{
		schema.Comment("Comment that appears in both the schema and the generated code"),
		entsql.WithComments(true),
		entsql.Check("text <> 'boring'"),
		entsql.Checks(map[string]string{
			"boring_check": "source_uri <> 'entgo.io'",
		}),
	}
}
