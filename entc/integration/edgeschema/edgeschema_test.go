// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package edgeschema

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/attachedfile"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/file"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/friendship"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/group"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/migrate"
	process "github.com/neko-sc/ent/entc/integration/edgeschema/ent/process"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/relationship"
	relationshipinfo "github.com/neko-sc/ent/entc/integration/edgeschema/ent/relationshipinfo"
	_ "github.com/neko-sc/ent/entc/integration/edgeschema/ent/runtime"
	tag "github.com/neko-sc/ent/entc/integration/edgeschema/ent/tag"
	tweet "github.com/neko-sc/ent/entc/integration/edgeschema/ent/tweet"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/tweetlike"
	"github.com/neko-sc/ent/entc/integration/edgeschema/ent/user"
	usergroup "github.com/neko-sc/ent/entc/integration/edgeschema/ent/usergroup"
	usertweet "github.com/neko-sc/ent/entc/integration/edgeschema/ent/usertweet"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestEdgeSchemaWithID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	// Create one.
	hub, lab := client.Group.Create().Set(group.Name, "GitHub").SaveX(ctx), client.Group.Create().Set(group.Name, "GitLab").SaveX(ctx)
	a8m, nat := client.User.Create().Set(user.Name, "a8m").AddIDs(user.Groups, hub.ID, lab.ID, hub.ID).SaveX(ctx), client.User.Create().Set(user.Name, "nati").AddIDs(user.Groups, hub.ID, hub.ID).SaveX(ctx)
	require.Equal(t, 2, a8m.QueryGroups().CountX(ctx), "should not create duplicates")
	require.Equal(t, 1, nat.QueryGroups().CountX(ctx))

	// Create batch (ignore duplicate groups).
	foobar := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "foo").AddIDs(user.Groups, hub.ID, lab.ID, hub.ID, lab.ID),
		client.User.Create().Set(user.Name, "bar").AddIDs(user.Groups, hub.ID, lab.ID, hub.ID, hub.ID),
	).SaveX(ctx)
	for _, u := range foobar {
		require.Equal(t, 2, u.QueryGroups().CountX(ctx))
		require.Equal(t, 2, u.QueryJoinedGroups().CountX(ctx))
		edges := u.QueryJoinedGroups().AllX(ctx)
		require.False(t, edges[0].JoinedAt.IsZero())
		require.False(t, edges[1].JoinedAt.IsZero())
	}

	err = hub.Update().AddIDs(group.Users, nat.ID).Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "duplicate edge error, because edge exists with a different 'joined_at' value")
	require.EqualError(t, errors.Unwrap(err), "add m2m edge for table user_groups: UNIQUE constraint failed: user_groups.user_id, user_groups.group_id")

	edges := a8m.QueryJoinedGroups().AllX(ctx)
	require.Equal(t, a8m.ID, edges[0].UserID)
	require.Equal(t, hub.ID, edges[0].GroupID)
	require.False(t, edges[0].JoinedAt.IsZero())
	require.Equal(t, a8m.ID, edges[1].UserID)
	require.Equal(t, lab.ID, edges[1].GroupID)
	require.False(t, edges[1].JoinedAt.IsZero())
	require.Equal(t, hub.ID, a8m.QueryJoinedGroups().QueryGroup().FirstIDX(ctx))
	require.Equal(t, lab.ID, a8m.QueryJoinedGroups().QueryGroup().Order(group.ID.Desc()).FirstIDX(ctx))

	edges = nat.QueryJoinedGroups().AllX(ctx)
	require.Equal(t, nat.ID, edges[0].UserID)
	require.Equal(t, hub.ID, edges[0].GroupID)
	require.False(t, edges[0].JoinedAt.IsZero())

	err = nat.Update().AddIDs(user.Groups, hub.ID).Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "unique constraint failed: user_groups.user_id, user_groups.group_id")

	users := client.User.Query().WithJoinedGroups(func(q *ent.UserGroupQuery) { q.WithGroup() }).AllX(ctx)
	require.Equal(t, []int{a8m.ID, nat.ID}, []int{users[0].ID, users[1].ID})
	require.Equal(t, []int{hub.ID, lab.ID}, []int{users[0].Edges.JoinedGroups[0].GroupID, users[0].Edges.JoinedGroups[1].GroupID})
	require.Equal(t, []int{hub.ID, lab.ID}, []int{users[0].Edges.JoinedGroups[0].Edges.Group.ID, users[0].Edges.JoinedGroups[1].Edges.Group.ID})
	require.Equal(t, hub.ID, users[1].Edges.JoinedGroups[0].GroupID)

	// Ignore update as we already have such edge between a8m and hub.
	client.UserGroup.Create().SetEdge(usergroup.User, a8m.ID).SetEdge(usergroup.Group, hub.ID).OnConflict().Ignore().ExecX(ctx)

	t1 := client.Tag.Create().Set(tag.Value, "tag").SaveX(ctx)
	hub.Update().AddIDs(group.Tags, t1.ID).ExecX(ctx)
	require.Equal(t, 1, client.GroupTag.Query().CountX(ctx))
	hub.Update().AddIDs(group.Tags, t1.ID).ExecX(ctx)
	// Adding the same edge should not create duplicate, but also should not fail because the edge does
	// not have extra fields besides the relation tuple and the ID (that is generated by the database).
	require.Equal(t, 1, client.GroupTag.Query().CountX(ctx))
}

func TestEdgeSchemaCompositeID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	tweets := client.Tweet.CreateBulk(
		client.Tweet.Create().Set(tweet.Text, "foo"),
		client.Tweet.Create().Set(tweet.Text, "bar"),
		client.Tweet.Create().Set(tweet.Text, "baz"),
	).SaveX(ctx)
	a8m := client.User.Create().Set(user.Name, "a8m").AddIDs(user.LikedTweets, tweets[0].ID, tweets[1].ID).SaveX(ctx)
	nat := client.User.Create().Set(user.Name, "nati").AddIDs(user.LikedTweets, tweets[0].ID).SaveX(ctx)
	likes := a8m.QueryLikes().AllX(ctx)
	require.Len(t, likes, 2)
	require.Equal(t, a8m.ID, likes[0].UserID)
	require.Equal(t, tweets[0].ID, likes[0].TweetID)
	require.Equal(t, a8m.ID, likes[1].UserID)
	require.Equal(t, tweets[1].ID, likes[1].TweetID)
	ts := time.Unix(1653377090, 0)
	like := client.TweetLike.Create().SetEdge(tweetlike.User, a8m.ID).Set(tweetlike.LikedAt, ts).SetEdge(tweetlike.Tweet, tweets[2].ID).SaveX(ctx)
	require.Equal(t, a8m.ID, like.UserID)
	require.Equal(t, tweets[2].ID, like.TweetID)
	require.Equal(t, a8m.ID, like.QueryUser().OnlyIDX(ctx))
	require.Equal(t, tweets[2].ID, like.QueryTweet().OnlyIDX(ctx))
	require.Equal(t, 3, a8m.QueryLikes().CountX(ctx))
	require.Equal(t, []int{tweets[0].ID, tweets[1].ID, tweets[2].ID}, a8m.QueryLikes().QueryTweet().IDsX(ctx))
	for _, k := range []*ent.TweetLike{
		a8m.QueryLikes().Where(tweetlike.LikedAt.EQ(ts)).OnlyX(ctx),
		client.TweetLike.Query().Where(tweetlike.LikedAt.EQ(ts)).OnlyX(ctx),
		client.Tweet.Query().QueryLikes().Where(tweetlike.LikedAt.EQ(ts)).OnlyX(ctx),
		client.Tweet.Query().QueryLikes().Where(tweetlike.LikedAt.EQ(ts), tweetlike.User.HasWith(user.Name.EQ(a8m.Name))).OnlyX(ctx),
		client.User.Query().QueryLikedTweets().QueryLikes().Where(tweetlike.LikedAt.EQ(ts), tweetlike.User.HasWith(user.Name.EQ(a8m.Name))).OnlyX(ctx),
	} {
		require.Equal(t, like.UserID, k.UserID)
		require.Equal(t, like.TweetID, k.TweetID)
		require.Equal(t, like.LikedAt.Unix(), k.LikedAt.Unix())
	}
	nat = nat.Update().AddIDs(user.LikedTweets, like.TweetID).SaveX(ctx)
	require.Equal(t, 2, nat.QueryLikes().CountX(ctx))
	require.Equal(t, 5, client.TweetLike.Query().CountX(ctx))
	require.Equal(t, 3, client.TweetLike.Query().Where(tweetlike.User.HasWith(user.Name.EQ(a8m.Name))).CountX(ctx))
	require.Equal(t, 2, client.TweetLike.Query().Where(tweetlike.User.HasWith(user.Name.EQ(nat.Name))).CountX(ctx))

	var v []struct {
		UserID int `sql:"user_id"`
		Count  int `sql:"count"`
	}
	require.NoError(t, client.TweetLike.Query().GroupBy(tweetlike.UserID).Aggregate(ent.Count()).Scan(ctx, &v))
	require.Equal(t, a8m.ID, v[0].UserID)
	require.Equal(t, 3, v[0].Count)
	require.Equal(t, nat.ID, v[1].UserID)
	require.Equal(t, 2, v[1].Count)

	// Ignore update as we already have such edge between a8m and hub.
	client.TweetLike.Create().Set(tweetlike.UserID, like.UserID).Set(tweetlike.TweetID, like.TweetID).OnConflict().Ignore().ExecX(ctx)
	client.TweetLike.Create().Set(tweetlike.UserID, like.UserID).Set(tweetlike.TweetID, like.TweetID).OnConflict().DoNothing().ExecX(ctx)

	// Clean all tweet likes and create them in batch again.
	client.TweetLike.Delete().ExecX(ctx)
	likes = client.TweetLike.CreateBulk(
		client.TweetLike.Create().Set(tweetlike.UserID, a8m.ID).SetEdge(tweetlike.Tweet, tweets[0].ID),
		client.TweetLike.Create().Set(tweetlike.UserID, a8m.ID).SetEdge(tweetlike.Tweet, tweets[1].ID),
		client.TweetLike.Create().Set(tweetlike.UserID, nat.ID).SetEdge(tweetlike.Tweet, tweets[1].ID),
		client.TweetLike.Create().Set(tweetlike.UserID, nat.ID).SetEdge(tweetlike.Tweet, tweets[2].ID),
	).SaveX(ctx)
	require.Equal(t, likes[0].UserID, a8m.ID)
	require.Equal(t, likes[0].TweetID, tweets[0].ID)
	require.NotZero(t, likes[0].LikedAt)
	require.Equal(t, likes[1].UserID, a8m.ID)
	require.Equal(t, likes[1].TweetID, tweets[1].ID)
	require.NotZero(t, likes[1].LikedAt)

	require.Equal(t, likes[2].UserID, nat.ID)
	require.Equal(t, likes[2].TweetID, tweets[1].ID)
	require.NotZero(t, likes[2].LikedAt)
	require.Equal(t, likes[3].UserID, nat.ID)
	require.Equal(t, likes[3].TweetID, tweets[2].ID)
	require.NotZero(t, likes[3].LikedAt)

	affected, err := client.TweetLike.Update().Set(tweetlike.LikedAt, time.Now()).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, client.TweetLike.Query().CountX(ctx), affected, "should update all edges (table rows)")
}

func TestEdgeSchemaDefaultID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	tweet1 := client.Tweet.Create().Set(tweet.Text, "foo").SaveX(ctx)
	tag1 := client.Tag.Create().Set(tag.Value, "1").SaveX(ctx)
	tweet1.Update().AddIDs(tweet.Tags, tag1.ID).SaveX(ctx)
	require.Equal(t, tag1.ID, tweet1.QueryTags().OnlyIDX(ctx))
	require.NotEqual(t, uuid.Nil, tweet1.QueryTweetTags().OnlyIDX(ctx))

	tweet2 := client.Tweet.Create().Set(tweet.Text, "bar").AddIDs(tweet.Tags, tag1.ID).SaveX(ctx)
	require.Equal(t, tag1.ID, tweet2.QueryTags().OnlyIDX(ctx))
	require.NotEqual(t, uuid.Nil, tweet2.QueryTweetTags().OnlyIDX(ctx))
}

func TestEdgeSchemaBidiWithID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	nat := client.User.Create().Set(user.Name, "nati").SaveX(ctx)
	a8m := client.User.Create().Set(user.Name, "a8m").AddIDs(user.Friends, nat.ID).SaveX(ctx)
	for _, f1 := range []*ent.Friendship{
		a8m.QueryFriendships().OnlyX(ctx),
		nat.QueryFriendships().QueryFriend().QueryFriendships().OnlyX(ctx),
		client.Friendship.Query().Where(friendship.Friend.HasWith(user.Name.EQ(nat.Name))).OnlyX(ctx),
	} {
		require.Equal(t, friendship.DefaultWeight, f1.Weight)
		require.False(t, f1.CreatedAt.IsZero())
		require.Equal(t, a8m.ID, f1.UserID)
		require.Equal(t, nat.ID, f1.FriendID)
	}
	require.Equal(t, 2, client.Friendship.Query().CountX(ctx), "bidirectional edges create 2 records in the join table")
}

func TestEdgeSchemaBidiCompositeID(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	u1 := client.User.Create().Set(user.Name, "u1").SaveX(ctx)
	u2 := client.User.Create().Set(user.Name, "u2").AddIDs(user.Relatives, u1.ID).SaveX(ctx)
	u3 := client.User.Create().Set(user.Name, "u3").AddIDs(user.Relatives, u2.ID).SaveX(ctx)

	err = u1.Update().AddIDs(user.Relatives, u2.ID).Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "duplicate edge error, because edge may contain a different 'weight' value in the database")
	require.EqualError(t, errors.Unwrap(err), "add m2m edge for table relationships: UNIQUE constraint failed: relationships.user_id, relationships.relative_id")

	u4u5 := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "u4").AddIDs(user.Relatives, u1.ID, u2.ID, u3.ID),
		client.User.Create().Set(user.Name, "u5").AddIDs(user.Relatives, u1.ID, u2.ID, u3.ID),
	).SaveX(ctx)
	for _, u := range u4u5 {
		require.Equal(t, 3, u.QueryRelatives().CountX(ctx))
		edges := u.QueryRelationship().AllX(ctx)
		require.Len(t, edges, 3)
		require.NotZero(t, edges[0].Weight)
		require.NotZero(t, edges[1].Weight)
		require.NotZero(t, edges[2].Weight)

		err := u.Update().AddIDs(user.Relatives, u3.ID).Exec(ctx)
		require.True(t, ent.IsConstraintError(err), "duplicate edge error, because edge may contain a different 'weight' value in the database")
		require.EqualError(t, errors.Unwrap(err), "add m2m edge for table relationships: UNIQUE constraint failed: relationships.user_id, relationships.relative_id")

		// Currently, the foreign-key action is configured as "NO ACTION" rather than "CASCADE", because
		// we do not clear edge-schema records when nodes are deleted, as they are treated as real nodes
		// (with additional fields) and not just as connections. Therefore, these we clear these edges
		// before deleting the record to avoid getting constraint violation.
		u.Update().ClearEdge(user.Relatives).ExecX(ctx)
		client.User.DeleteOne(u).ExecX(ctx)
	}

	var v []struct {
		UserID int `sql:"user_id"`
		Count  int `sql:"count"`
	}
	require.NoError(t, client.Relationship.Query().GroupBy(relationship.UserID).Aggregate(ent.Count()).Scan(ctx, &v))
	require.EqualValues(
		t,
		[]struct{ UserID, Count int }{{u1.ID, 1}, {u2.ID, 2}, {u3.ID, 1}},
		v,
	)
	for _, r := range []int{
		u2.QueryRelationship().Where(relationship.RelativeID.EQ(u3.ID)).QueryRelative().OnlyIDX(ctx),
		u1.QueryRelatives().QueryRelationship().Where(relationship.RelativeID.NEQ(u1.ID)).QueryRelative().OnlyIDX(ctx),
		client.User.Query().Where(user.ID.EQ(u1.ID)).QueryRelatives().QueryRelationship().Where(relationship.RelativeID.NEQ(u1.ID)).QueryRelative().OnlyIDX(ctx),
	} {
		require.Equal(t, u3.ID, r)
	}

	info := client.RelationshipInfo.Create().Set(relationshipinfo.Text, "u1->u2").SaveX(ctx)
	r1 := u1.QueryRelationship().OnlyX(ctx)
	r1.Update().SetEdge(relationship.Info, info.ID).ExecX(ctx)
	r2 := client.User.Query().QueryRelationship().Where(relationship.Info.Has()).WithInfo().OnlyX(ctx)
	require.Equal(t, r1.UserID, r2.UserID)
	require.Equal(t, r1.RelativeID, r2.RelativeID)
	require.Equal(t, info.ID, r2.Edges.Info.ID)
}

func TestEdgeSchemaForO2M(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	t1 := client.Tweet.Create().Set(tweet.Text, "Hello Edge Schema").SaveX(ctx)
	a8m := client.User.Create().Set(user.Name, "a8m").AddIDs(user.Tweets, t1.ID).SaveX(ctx)
	require.Equal(t, t1.ID, a8m.QueryTweets().OnlyIDX(ctx))
	_, err = client.User.Create().Set(user.Name, "nati").AddIDs(user.Tweets, t1.ID).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "Tweet can have only one author")

	nat := client.User.Create().Set(user.Name, "nati").SaveX(ctx)
	err = nat.Update().AddIDs(user.Tweets, t1.ID).Exec(ctx)
	require.True(t, ent.IsConstraintError(err))
	err = client.UserTweet.Create().SetEdge(usertweet.User, nat.ID).SetEdge(usertweet.Tweet, t1.ID).Exec(ctx)
	require.True(t, ent.IsConstraintError(err))

	tweets := client.Tweet.CreateBulk(
		client.Tweet.Create().Set(tweet.Text, "t1"),
		client.Tweet.Create().Set(tweet.Text, "t2"),
	).SaveX(ctx)
	nat.Update().AddIDs(user.Tweets, tweets[0].ID, tweets[1].ID).ExecX(ctx)
}

func TestEdgeSchemaTypeMatching(t *testing.T) {
	client, err := ent.Open(dialect.SQLite, "file:ent?mode=memory&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)))

	files := client.File.CreateBulk(
		client.File.Create().Set(file.Name, "a"),
		client.File.Create().Set(file.Name, "b"),
		client.File.Create().Set(file.Name, "c"),
	).SaveX(ctx)
	proc := client.Process.Create().AddIDs(process.Files, files[0].ID, files[1].ID).SaveX(ctx)

	// Assoc query.
	fileNames, projectionError := ent.Values(ctx, proc.QueryFiles().Order(file.Name.Asc()).Select(file.Name), file.Name)
	require.NoError(t, projectionError)
	require.Equal(t, []string{"a", "b"}, fileNames)
	attachedIDs, projectionError := ent.Values(ctx, proc.QueryAttachedFiles().Order(attachedfile.FID.Asc()).Select(attachedfile.FID), attachedfile.FID)
	require.NoError(t, projectionError)
	require.Equal(t, []int{files[0].ID, files[1].ID}, attachedIDs)
	require.Equal(t, []int{files[0].ID, files[1].ID}, proc.QueryAttachedFiles().QueryFi().IDsX(ctx))
	require.Equal(t, proc.ID, client.Process.Query().QueryAttachedFiles().QueryProc().OnlyIDX(ctx))

	// Inverse query.
	require.Equal(t, proc.ID, files[0].QueryProcesses().OnlyIDX(ctx))
	require.Equal(t, proc.ID, files[1].QueryProcesses().OnlyIDX(ctx))
	require.False(t, files[2].QueryProcesses().ExistX(ctx))
	require.Equal(t, []int{files[0].ID, files[1].ID}, client.File.Query().QueryProcesses().QueryFiles().IDsX(ctx))
	require.Equal(t, []int{files[0].ID, files[1].ID}, client.File.Query().QueryProcesses().QueryAttachedFiles().QueryFi().IDsX(ctx))

	// Edge schema query and mutation.
	require.Equal(t, []int{files[0].ID, files[1].ID}, client.AttachedFile.Query().QueryFi().IDsX(ctx))
	require.Equal(t, []int{files[0].ID, files[1].ID}, client.AttachedFile.Query().QueryProc().QueryFiles().IDsX(ctx))
	require.Equal(t, proc.ID, client.AttachedFile.Query().QueryProc().OnlyIDX(ctx))
	af := client.AttachedFile.Create().SetEdge(attachedfile.Fi, files[2].ID).SetEdge(attachedfile.Proc, proc.ID).SaveX(ctx)
	require.Equal(t, proc.ID, af.QueryProc().OnlyIDX(ctx))
	require.Equal(t, files[2].ID, af.QueryFi().OnlyIDX(ctx))
	require.Equal(t, []int{files[0].ID, files[1].ID, files[2].ID}, client.AttachedFile.Query().QueryFi().IDsX(ctx))
	require.Equal(t, files[2].ID, client.AttachedFile.Query().QueryProc().QueryFiles().Where(file.Name.EQ(files[2].Name)).OnlyIDX(ctx))
}
