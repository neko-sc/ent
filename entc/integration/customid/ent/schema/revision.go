// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

// User holds the schema definition for the User entity.
import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

// Revision holds the schema definition for the Revision entity.
type Revision struct {
	ent.Schema
}

// Fields of the Revision.
func (Revision) Fields() []ent.Field {
	return []ent.Field{
		field.String("id"),
	}
}
