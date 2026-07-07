// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// UserTweet holds the schema definition for the UserTweet entity.
type UserTweet struct {
	ent.Schema
}

// Fields of the UserTweet.
func (UserTweet) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Default(time.Now),
		field.Int("user_id"),
		field.Int("tweet_id"),
	}
}

// Edges of the UserTweet.
func (UserTweet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required().
			Field("user_id"),
		edge.To("tweet", Tweet.Type).
			Unique().
			Required().
			Field("tweet_id"),
	}
}

// Indexes of the UserTweet.
func (UserTweet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tweet_id").
			Unique(),
	}
}
