// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

// RelationshipInfo holds the schema definition for the RelationshipInfo entity.
type RelationshipInfo struct {
	ent.Schema
}

// Fields of the RelationshipInfo.
func (RelationshipInfo) Fields() []ent.Field {
	return []ent.Field{
		field.String("text"),
	}
}
