// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
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
		field.Int8("int8_to_string").
			Optional(),
		field.Uint8("uint8_to_string").
			Optional(),
		field.Int16("int16_to_string").
			Optional(),
		field.Uint16("uint16_to_string").
			Optional(),
		field.Int32("int32_to_string").
			Optional(),
		field.Uint32("uint32_to_string").
			Optional(),
		field.Int64("int64_to_string").
			Optional(),
		field.Uint64("uint64_to_string").
			Optional(),
	}
}
