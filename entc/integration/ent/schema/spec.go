// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
)

type Spec struct {
	ent.Schema
}

// Edges of the Spec.
func (Spec) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("card", Card.Type),
	}
}
