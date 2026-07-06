// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// Tweet holds the schema definition for the Tweet entity.
type Tweet struct {
	ent.Schema
}

// Fields of the Tweet.
func (Tweet) Fields() []ent.Field {
	return []ent.Field{
		field.Text("text"),
	}
}

// Edges of the Tweet.
func (Tweet) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("liked_users", User.Type).
			Ref("liked_tweets").
			Through("likes", TweetLike.Type),
		edge.From("user", User.Type).
			Ref("tweets").
			Through("tweet_user", UserTweet.Type).
			Comment("The uniqueness is enforced on the edge schema"),
		edge.From("tags", Tag.Type).
			Ref("tweets").
			Through("tweet_tags", TweetTag.Type),
	}
}
