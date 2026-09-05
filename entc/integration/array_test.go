package integration

import (
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/user"
	"github.com/stretchr/testify/require"
	"testing"
)

func Arrays(t *testing.T, client *ent.Client) {
	ctx := t.Context()
	created := client.User.Create().Set(user.Name, "arrays").Set(user.Age, 20).Set(user.Tags, []string{"go", "sql"}).Set(user.Scores, []int{1, 2, 3}).SaveX(ctx)
	loaded := client.User.GetX(ctx, created.ID)
	require.Equal(t, []string{"go", "sql"}, loaded.Tags)
	require.Equal(t, []int{1, 2, 3}, loaded.Scores)
	require.Equal(t, 1, client.User.Query().Where(user.Tags.Contains("go"), user.Tags.HasAny("sql", "missing"), user.Tags.HasAll("go", "sql"), user.Tags.LenGT(1), user.Scores.Contains(2)).CountX(ctx))
	created.Update().Set(user.Tags, []string{"updated"}).Set(user.Scores, []int{4, 5}).SaveX(ctx)
	loaded = client.User.GetX(ctx, created.ID)
	require.Equal(t, []string{"updated"}, loaded.Tags)
	require.Equal(t, []int{4, 5}, loaded.Scores)
	created.Update().Set(user.Tags, []string{}).SaveX(ctx)
	require.Equal(t, []string{}, client.User.GetX(ctx, created.ID).Tags)
	created.Update().Clear(user.Tags).SaveX(ctx)
	require.Nil(t, client.User.GetX(ctx, created.ID).Tags)
	unset := client.User.Create().Set(user.Name, "unset arrays").Set(user.Age, 20).SaveX(ctx)
	require.Nil(t, client.User.GetX(ctx, unset.ID).Scores)
}
