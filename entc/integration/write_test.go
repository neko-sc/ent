package integration

import (
	"context"
	"encoding/json"
	"testing"

	runtime "github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/card"
	"github.com/neko-sc/ent/entc/integration/ent/pet"
	"github.com/neko-sc/ent/entc/integration/ent/user"
	"github.com/stretchr/testify/require"
)

func WritePath(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	_, err := client.User.Create().Set(user.Age, 0).Save(ctx)
	require.True(t, ent.IsValidationError(err))
	first := client.User.Insert(ent.UserInsert{Name: "first", Age: 0}).SaveX(ctx)
	require.Equal(t, "unknown", first.Last)
	require.Equal(t, "static", first.Address)
	require.Zero(t, first.Age)
	require.Equal(t, "first", client.User.GetX(ctx, first.ID).Name)

	second := client.User.Create().SetExpr(user.Name, ent.Literal("second")).SetExpr(user.Age, ent.Literal(20)).SaveX(ctx)
	require.Equal(t, "second", second.Name)
	require.Equal(t, 20, second.Age)
	second = second.Update().Set(user.Age, 5).SetExpr(user.Age, runtime.Add(user.Age.Expr(), ent.Literal(2))).SaveX(ctx)
	require.Equal(t, 22, second.Age)
	second = second.Update().SetExpr(user.Age, ent.Literal(100)).Set(user.Age, 10).SaveX(ctx)
	require.Equal(t, 10, second.Age)

	var patch ent.UserPatch
	require.NoError(t, json.Unmarshal([]byte(`{"nickname":null,"age":0}`), &patch))
	require.True(t, patch.Nickname.IsNull())
	require.True(t, patch.Name.IsUnset())
	first = first.Update().Apply(patch).SaveX(ctx)
	require.Zero(t, first.Age)
	require.Empty(t, first.Nickname)
	_, err = first.Update().Apply(ent.UserPatch{Name: ent.Null[string]()}).Save(ctx)
	require.True(t, ent.IsValidationError(err))
	_, err = first.Update().Add(user.ID, 1).Save(ctx)
	require.True(t, ent.IsValidationError(err))
	_, err = first.Update().Append(user.Name, "invalid").Save(ctx)
	require.True(t, ent.IsValidationError(err))
	_, err = first.Update().Clear(user.Age).Save(ctx)
	require.True(t, ent.IsValidationError(err))
	_, err = client.Card.Create().Set(card.Number, "immutable").SaveX(ctx).Update().Set(card.Number, "changed").Save(ctx)
	require.True(t, ent.IsValidationError(err))

	first = first.Update().Apply(ent.UserPatch{AgeAdd: ent.Some(2)}).
		Apply(ent.UserPatch{AgeAdd: ent.Some(3)}).SaveX(ctx)
	require.Equal(t, 5, first.Age)
	first = first.Update().Set(user.Age, 3).Add(user.Age, 2).SaveX(ctx)
	require.Equal(t, 5, first.Age)
	first = first.Update().Clear(user.Nickname).SetExpr(user.Nickname, ent.Literal("expression")).SaveX(ctx)
	require.Equal(t, "expression", first.Nickname)
	animal := client.Pet.Insert(ent.PetInsert{Name: "pet"}).SetEdge(pet.Owner, first.ID).SaveX(ctx)
	require.Equal(t, first.ID, animal.QueryOwner().OnlyIDX(ctx))
	first.Update().Apply(ent.UserPatch{Pets: ent.RelationPatch[int]{Clear: true}}).ExecX(ctx)
	require.False(t, animal.QueryOwner().ExistX(ctx))

	returned, err := client.User.Update().Where(user.ID.In(first.ID, second.ID)).Add(user.Age, 1).Returning(ctx)
	require.NoError(t, err)
	require.Len(t, returned, 2)
	require.Equal(t, 6, client.User.GetX(ctx, first.ID).Age)
	if client.Driver().Capabilities().ReturningOld {
		old, updated, err := first.Update().Add(user.Age, 1).SaveOld(ctx)
		require.NoError(t, err)
		require.Equal(t, 6, old.Age)
		require.Equal(t, 7, updated.Age)
	} else {
		_, _, err := first.Update().Add(user.Age, 1).SaveOld(ctx)
		require.ErrorAs(t, err, new(*dialect.UnsupportedError))
		require.Equal(t, 6, client.User.GetX(ctx, first.ID).Age)
	}
	removed, err := client.User.Delete().Where(user.ID.EQ(second.ID)).Returning(ctx)
	require.NoError(t, err)
	require.Len(t, removed, 1)
	require.Equal(t, second.ID, removed[0].ID)

	existing := client.User.Insert(ent.UserInsert{Name: "existing", Age: 1, Phone: ent.Some("write-conflict")}).SaveX(ctx)
	skipped, err := client.User.Insert(ent.UserInsert{Name: "ignored", Age: 9, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).DoNothing().Save(ctx)
	require.Nil(t, skipped)
	require.ErrorIs(t, err, ent.ErrConflict)
	require.NoError(t, client.User.Insert(ent.UserInsert{Name: "ignored", Age: 9, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).DoNothing().Exec(ctx))
	updated, err := client.User.Insert(ent.UserInsert{Name: "replacement", Age: 2, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).
		DoUpdate(func(update *ent.UserUpsert) {
			update.SetExpr(user.Age, runtime.Add(user.Age.Expr(), ent.Excluded(user.Age)))
		}).Save(ctx)
	require.NoError(t, err)
	require.Equal(t, existing.ID, updated.ID)
	require.Equal(t, "existing", updated.Name)
	require.Equal(t, 3, updated.Age)
	_, err = client.User.Insert(ent.UserInsert{Name: "invalid", Age: 0, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).
		DoUpdate(func(update *ent.UserUpsert) { update.Clear(user.Age) }).Save(ctx)
	require.True(t, ent.IsValidationError(err))
	filtered, err := client.User.Insert(ent.UserInsert{Name: "filtered", Age: 9, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).
		Where(user.Phone.NotNull()).UpdateWhere(user.Age.EQ(3)).UpdateNewValues().Save(ctx)
	require.NoError(t, err)
	require.Equal(t, 9, filtered.Age)
	_, err = client.User.Insert(ent.UserInsert{Name: "select", Age: 0, Phone: ent.Some("write-conflict")}).OnConflict(user.Phone).DoSelect().Save(ctx)
	require.ErrorAs(t, err, new(*dialect.UnsupportedError))

	bulk, err := client.User.InsertBulk(
		ent.UserInsert{Name: "ignored", Age: 0, Phone: ent.Some("write-conflict")},
		ent.UserInsert{Name: "inserted", Age: 0, Phone: ent.Some("write-new")},
	).OnConflict(user.Phone).DoNothing().Save(ctx)
	require.NoError(t, err)
	require.Len(t, bulk, 1)
	require.Equal(t, "inserted", bulk[0].Name)
	require.Equal(t, "static", bulk[0].Address)
	require.True(t, client.User.Query().Where(user.Phone.EQ("write-new")).ExistX(ctx))
	expressionBulk := client.User.CreateBulk(
		client.User.Create().SetExpr(user.Name, ent.Literal("bulk-expression")).Set(user.Age, 3),
		client.User.Create().Set(user.Name, "bulk-value").SetExpr(user.Age, ent.Literal(4)),
	).SaveX(ctx)
	require.Equal(t, "bulk-expression", expressionBulk[0].Name)
	require.Equal(t, 4, expressionBulk[1].Age)
}
