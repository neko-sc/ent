package integration

import (
	"testing"
	"time"

	entgo "github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/entity"
	"github.com/neko-sc/ent/entc/integration/ent/group"
	"github.com/neko-sc/ent/entc/integration/ent/groupinfo"
	"github.com/neko-sc/ent/entc/integration/ent/pet"
	"github.com/neko-sc/ent/entc/integration/ent/user"
	"github.com/stretchr/testify/require"
)

func ReadPath(t *testing.T, client *ent.Client) {
	ctx := t.Context()
	parents := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "alpha").Set(user.Age, 10),
		client.User.Create().Set(user.Name, "beta").Set(user.Age, 20),
		client.User.Create().Set(user.Name, "empty").Set(user.Age, 30),
	).SaveX(ctx)
	for _, parent := range parents[:2] {
		for index, name := range []string{"a", "b", "c", "d"} {
			client.Pet.Create().Set(pet.Name, name).Set(pet.Age, float64(index)).SetEdge(pet.Owner, parent.ID).ExecX(ctx)
		}
	}
	info := client.GroupInfo.Create().Set(groupinfo.Desc, "read path").SaveX(ctx)
	groups := client.Group.CreateBulk(
		client.Group.Create().Set(group.Name, "Alpha").Set(group.Expire, time.Now()).SetEdge(group.Info, info.ID).AddIDs(group.Users, parents[0].ID),
		client.Group.Create().Set(group.Name, "Bravo").Set(group.Expire, time.Now()).SetEdge(group.Info, info.ID).AddIDs(group.Users, parents[0].ID, parents[1].ID),
		client.Group.Create().Set(group.Name, "Charlie").Set(group.Expire, time.Now()).SetEdge(group.Info, info.ID).AddIDs(group.Users, parents[0].ID, parents[1].ID),
		client.Group.Create().Set(group.Name, "Delta").Set(group.Expire, time.Now()).SetEdge(group.Info, info.ID).AddIDs(group.Users, parents[1].ID),
	).SaveX(ctx)

	t.Run("PerParent", func(t *testing.T) {
		query := client.User.Query().Order(user.ID.Asc()).WithCount(user.Pets).WithCount(user.Groups).
			WithPets(func(query *ent.PetQuery) { query.Order(pet.Name.Desc()).Offset(1).Limit(2) }).
			WithGroups(func(query *ent.GroupQuery) { query.Order(group.Name.Asc()).Offset(1).Limit(2) })
		for range 2 {
			loaded := query.AllX(ctx)
			for index, parent := range loaded {
				require.True(t, parent.Edges.Loaded(user.Pets))
				require.True(t, parent.Edges.Loaded(user.Groups))
				require.False(t, parent.Edges.Loaded(user.Friends))
				count, present := parent.Edges.Count(user.Pets)
				require.True(t, present)
				groupCount, present := parent.Edges.Count(user.Groups)
				require.True(t, present)
				if index == 2 {
					require.Zero(t, count)
					require.Zero(t, groupCount)
					require.Empty(t, parent.Edges.Pets)
					require.Empty(t, parent.Edges.Groups)
					continue
				}
				require.Equal(t, 4, count)
				require.Equal(t, 3, groupCount)
				require.Equal(t, []string{"c", "b"}, []string{parent.Edges.Pets[0].Name, parent.Edges.Pets[1].Name})
				require.Equal(t, []int{groups[index+1].ID, groups[index+2].ID}, []int{parent.Edges.Groups[0].ID, parent.Edges.Groups[1].ID})
			}
		}
		loaded := client.User.Query().Order(user.ID.Asc()).WithPets(func(query *ent.PetQuery) { query.Limit(0) }).AllX(ctx)
		for _, parent := range loaded {
			require.Empty(t, parent.Edges.Pets)
		}
		loaded = client.User.Query().Order(user.ID.Asc()).WithPets(func(query *ent.PetQuery) { query.Offset(3) }).AllX(ctx)
		require.Len(t, loaded[0].Edges.Pets, 1)
		require.Len(t, loaded[1].Edges.Pets, 1)
		owners := client.Pet.Query().WithOwner(func(query *ent.UserQuery) { query.Limit(1) }).AllX(ctx)
		for _, child := range owners {
			require.NotNil(t, child.Edges.Owner)
		}
		nested := client.User.Query().Where(user.ID.EQ(parents[0].ID)).
			WithGroups(func(query *ent.GroupQuery) {
				query.WithNamedUsers("limited", func(query *ent.UserQuery) { query.Order(user.ID.Desc()).Limit(1) })
			}).Clone().OnlyX(ctx)
		for _, loaded := range nested.Edges.Groups {
			users, err := loaded.NamedUsers("limited")
			require.NoError(t, err)
			require.Len(t, users, 1)
		}
		var mocked ent.UserEdges
		mocked.SetLoaded(user.Pets, true)
		require.True(t, mocked.Loaded(user.Pets))
		mocked.SetLoaded(user.Pets, false)
		require.False(t, mocked.Loaded(user.Pets))
	})

	t.Run("Projection", func(t *testing.T) {
		ownerID := entgo.Column[entity.Pet, int]{ColumnRef: entgo.ColumnRef{Table: pet.Table, Name: pet.OwnerColumn}}
		childName := pet.Name.As("child_name").Nullable()
		rows, err := client.User.Query().LeftJoin(pet.Table, user.ID.EQColumn(ownerID)).Order(user.ID.Asc()).Select(user.Name, childName).Rows(ctx)
		require.NoError(t, err)
		require.Len(t, rows, 9)
		_, present := ent.GetNullable(rows[8], childName)
		require.False(t, present)
		require.Equal(t, "empty", ent.Get(rows[8], user.Name))
		row, err := client.User.Query().Aggregate(ent.Count(), ent.Sum(user.Age), ent.Avg(user.Age)).Row(ctx)
		require.NoError(t, err)
		require.Equal(t, 3, ent.Get(row, ent.Count()))
		require.Equal(t, 60, ent.Get(row, ent.Sum(user.Age)))
		require.Equal(t, float64(20), ent.Get(row, ent.Avg(user.Age)))
		expression := ent.ExprSelection(entgo.Add(user.Age.Expr(), ent.Literal(7)), "adjusted")
		values, err := ent.Values(ctx, client.User.Query().Where(user.Age.GT(10)).Order(user.ID.Asc()).Select(expression), expression)
		require.NoError(t, err)
		require.Equal(t, []int{27, 37}, values)
		_, err = client.User.Query().Select(user.Name).Row(ctx)
		require.True(t, ent.IsNotSingular(err))
		_, err = client.User.Query().Where(user.ID.EQ(-1)).Select(user.Name).Row(ctx)
		require.True(t, ent.IsNotFound(err))
		_, err = client.User.Query().Select(user.Name).Modify(func(selector *sql.Selector) { selector.AppendSelect(selector.C(user.FieldAge)) }).Rows(ctx)
		require.ErrorContains(t, err, "column count")
		grouped, err := client.User.Query().GroupBy(user.Age).Aggregate(ent.Count()).Rows(ctx)
		require.NoError(t, err)
		require.Len(t, grouped, 3)
		for _, row := range grouped {
			require.Equal(t, 1, ent.Get(row, ent.Count()))
		}
	})
}
