// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package cascadelete

import (
	"context"
	"testing"

	"github.com/neko-sc/ent/entc/integration/cascadelete/ent"
	comment "github.com/neko-sc/ent/entc/integration/cascadelete/ent/comment"
	post "github.com/neko-sc/ent/entc/integration/cascadelete/ent/post"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestCascadeDelete(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	ctx := context.Background()
	require.NoError(t, client.Schema.Create(ctx))

	author := client.User.Create().SaveX(ctx)
	posts := client.Post.CreateBulk(
		client.Post.Create(),
		client.Post.Create().SetEdge(post.Author, author.ID),
		client.Post.Create().SetEdge(post.Author, author.ID),
	).SaveX(ctx)
	comments := client.Comment.CreateBulk(
		client.Comment.Create().Set(comment.Text, "Go").SetEdge(comment.Post, posts[0].ID),
		client.Comment.Create().Set(comment.Text, "Ent").SetEdge(comment.Post, posts[1].ID),
		client.Comment.Create().Set(comment.Text, "GraphQL").SetEdge(comment.Post, posts[1].ID),
	).SaveX(ctx)

	t.Log("Delete the author with its 2 posts and their comments")
	client.User.DeleteOne(author).ExecX(ctx)
	require.Zero(t, client.User.Query().CountX(ctx))
	require.Equal(t, posts[0].ID, client.Post.Query().OnlyIDX(ctx))
	require.Equal(t, comments[0].ID, client.Comment.Query().OnlyIDX(ctx))
}
