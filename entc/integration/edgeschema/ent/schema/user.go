// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/integration/privacy/ent/privacy"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			Default("Unknown"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("groups", Group.Type).
			Through("joined_groups", UserGroup.Type),
		edge.To("friends", User.Type).
			Through("friendships", Friendship.Type),
		edge.To("relatives", User.Type).
			Through("relationship", Relationship.Type),
		edge.To("liked_tweets", Tweet.Type).
			Through("likes", TweetLike.Type),
		edge.To("tweets", Tweet.Type).
			Through("user_tweets", UserTweet.Type),
		edge.To("roles", Role.Type).
			Through("roles_users", RoleUser.Type),
	}
}

// Policy defines the privacy policy of the User.
func (User) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
