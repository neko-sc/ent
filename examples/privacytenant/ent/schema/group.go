// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/examples/privacytenant/ent/privacy"
	"github.com/neko-sc/ent/examples/privacytenant/rule"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Group holds the schema definition for the Group entity.
type Group struct {
	ent.Schema
}

// Mixin of the User schema.
func (Group) Mixin() []ent.Mixin {
	return []ent.Mixin{
		BaseMixin{},
		TenantMixin{},
	}
}

// Fields of the User.
func (Group) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Default("Unknown"),
	}
}

// Edges of the Group.
func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("users", User.Type).
			Ref("groups"),
	}
}

// Policy defines the privacy policy of the Group.
func (Group) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			// Limit DenyMismatchedTenants only for
			// Create operations
			privacy.OnMutationOperation(
				rule.DenyMismatchedTenants(),
				ent.OpCreate,
			),
		},
	}
}
