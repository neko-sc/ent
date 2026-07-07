// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema/field"
)

// Conversion holds the schema definition for the Conversion entity.
type Conversion struct {
	ent.Schema
}

// Fields of the Conversion.
func (Conversion) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Optional(),
		// Convert integer fields to string
		// Postgres uses the same type for int8 and int16
		// Postgres loses unsigned so we have assume value is signed
		field.String("int8_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 6}),
		field.String("uint8_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 6}),
		field.String("int16_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 6}),
		field.String("uint16_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 6}),
		field.String("int32_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 12}),
		field.String("uint32_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 12}),
		field.String("int64_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 21}),
		field.String("uint64_to_string").
			Optional().
			Annotations(entsql.Annotation{Size: 21}),
	}
}
