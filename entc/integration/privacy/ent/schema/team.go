// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/integration/privacy/ent/privacy"
	"github.com/neko-sc/ent/entc/integration/privacy/rule"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/mixin"
)

// Team defines the schema of a team.
type Team struct {
	ent.Schema
}

// Mixin list of schemas to the team.
func (Team) Mixin() []ent.Mixin {
	return []ent.Mixin{
		BaseMixin{},
	}
}

// Fields of the team.
func (Team) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty(),
	}
}

// Edges of the team.
func (Team) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tasks", Task.Type).
			Ref("teams"),
		edge.From("users", User.Type).
			Ref("teams"),
	}
}

// Policy of the team.
func (Team) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			rule.DenyIfNotAdmin(),
			rule.DenyUpdateRule(),
		},
	}
}

// TeamMixin shared between task and user.
type TeamMixin struct {
	mixin.Schema
}

// Edges of the team-mixin.
func (TeamMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("teams", Team.Type),
	}
}

// Policy of the team-mixin.
func (TeamMixin) Policy() ent.Policy {
	return privacy.Policy{
		Query: privacy.QueryPolicy{
			rule.FilterTeamRule(),
		},
	}
}
