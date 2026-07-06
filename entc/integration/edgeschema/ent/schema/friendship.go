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

// Friendship holds the edge schema definition of the Friendship relationship.
type Friendship struct {
	ent.Schema
}

// Fields of the Friendship.
func (Friendship) Fields() []ent.Field {
	return []ent.Field{
		field.Int("weight").
			Default(1),
		field.Time("created_at").
			Default(time.Now),
		field.Int("user_id").
			Immutable(),
		field.Int("friend_id").
			Immutable(),
	}
}

// Edges of the Friendship.
func (Friendship) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique().
			Required().
			Immutable().
			Field("user_id"),
		edge.To("friend", User.Type).
			Unique().
			Required().
			Immutable().
			Field("friend_id"),
	}
}

// Indexes of the Friendship.
func (Friendship) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		// By default, Ent generates a unique index named <T>_<FK1>_<FK2>
		// for edge-schemas with an ID field to enforce the uniqueness of
		// the edges reside in the join table. However, in this case it is
		// skipped because we define it explicitly in the definition below.
		index.Fields("user_id", "friend_id").
			Unique().
			StorageKey("friendships_edge"),
	}
}
