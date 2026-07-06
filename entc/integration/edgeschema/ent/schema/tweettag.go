// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/google/uuid"
)

// TweetTag holds the schema definition for the TweetTag entity.
type TweetTag struct {
	ent.Schema
}

// Fields of the TweetTag.
func (TweetTag) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.Time("added_at").
			Default(time.Now),
		field.Int("tag_id"),
		field.Int("tweet_id"),
	}
}

// Edges of the TweetTag.
func (TweetTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("tag", Tag.Type).
			Unique().
			Required().
			Field("tag_id"),
		edge.To("tweet", Tweet.Type).
			Unique().
			Required().
			Field("tweet_id"),
	}
}
