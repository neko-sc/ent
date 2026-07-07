// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package failure

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
)

type User struct {
	ent.Schema
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("panic", User{}.Type),
	}
}
