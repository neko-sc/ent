// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import "github.com/neko-sc/ent"

// Builder holds the schema definition for the Builder entity.
type Builder struct {
	ent.Schema
}

// Fields of the Builder.
func (Builder) Fields() []ent.Field {
	return nil
}

// Edges of the Builder.
func (Builder) Edges() []ent.Edge {
	return nil
}
