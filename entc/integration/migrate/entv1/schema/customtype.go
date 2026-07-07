// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
)

// CustomType holds the schema definition for the CustomType entity.
type CustomType struct {
	ent.Schema
}

// Fields of the CustomType.
func (CustomType) Fields() []ent.Field {
	return []ent.Field{
		field.String("custom").
			Optional().
			SchemaType(map[string]string{
				dialect.Postgres: "customtype",
			}),
	}
}
