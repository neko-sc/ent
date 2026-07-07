// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import "github.com/neko-sc/ent"

// Goods holds the schema definition for the Goods entity.
type Goods struct {
	ent.Schema
}

// Fields of the Goods.
func (Goods) Fields() []ent.Field {
	return nil
}

// Edges of the Goods.
func (Goods) Edges() []ent.Edge {
	return nil
}
