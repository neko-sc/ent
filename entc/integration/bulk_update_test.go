// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"net"
	"testing"
	"time"

	entgo "github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/fieldtype"
	"github.com/neko-sc/ent/entc/integration/ent/pet"
	"github.com/neko-sc/ent/entc/integration/ent/role"
	"github.com/neko-sc/ent/entc/integration/ent/schema"
	"github.com/neko-sc/ent/entc/integration/ent/schema/task"
	enttask "github.com/neko-sc/ent/entc/integration/ent/task"
	"github.com/neko-sc/ent/entc/integration/ent/user"

	"github.com/stretchr/testify/require"
)

func UpdateBulk(t *testing.T, client *ent.Client) {
	ctx := t.Context()
	createUsers := func(names ...string) []*ent.User {
		builders := make([]*ent.UserCreate, len(names))
		for index, name := range names {
			builders[index] = client.User.Create().Set(user.Name, name).Set(user.Age, 10*(index+1)).
				Set(user.Nickname, name+"-nickname").Set(user.OptionalInt, index+1).Set(user.Tags, []string{name})
		}
		return client.User.CreateBulk(builders...).SaveX(ctx)
	}

	t.Run("SetNullAdd", func(t *testing.T) {
		users := createUsers("set-a", "set-b", "set-c")
		updated := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Set(user.Name, "set-a2").Set(user.Age, 11).Set(user.Role, user.RoleAdmin).
				SetOptional(user.Nickname, ent.Some("set-a2-nickname")),
			client.User.UpdateOne(users[1]).Add(user.Age, 5).Clear(user.Nickname).Set(user.Employment, user.EmploymentContract),
			client.User.UpdateOne(users[2]).Set(user.Name, "set-c2").Set(user.Role, user.RoleFreeUser).
				SetOptional(user.OptionalInt, ent.Null[int]()).Add(user.FilesCount, 3),
		).SaveX(ctx)
		require.Len(t, updated, 3)
		require.Equal(t, users[0].ID, updated[0].ID)
		require.Equal(t, "set-a2", updated[0].Name)
		require.Equal(t, 11, updated[0].Age)
		require.Equal(t, user.RoleAdmin, updated[0].Role)
		require.Equal(t, "set-a2-nickname", updated[0].Nickname)
		require.Equal(t, 1, updated[0].OptionalInt)
		require.Equal(t, users[1].ID, updated[1].ID)
		require.Equal(t, "set-b", updated[1].Name)
		require.Equal(t, 25, updated[1].Age)
		require.Empty(t, updated[1].Nickname)
		require.Equal(t, user.EmploymentContract, updated[1].Employment)
		require.Equal(t, user.RoleUser, updated[1].Role)
		require.Equal(t, users[2].ID, updated[2].ID)
		require.Equal(t, "set-c2", updated[2].Name)
		require.Equal(t, 30, updated[2].Age)
		require.Equal(t, user.RoleFreeUser, updated[2].Role)
		require.Equal(t, "set-c-nickname", updated[2].Nickname)
		require.Zero(t, updated[2].OptionalInt)
		require.Equal(t, 3, updated[2].FilesCount)
		for _, entity := range updated {
			require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
		}
		require.Zero(t, client.User.Query().Where(user.ID.EQ(users[1].ID), user.Nickname.NotNull()).CountX(ctx))
		require.Zero(t, client.User.Query().Where(user.ID.EQ(users[2].ID), user.OptionalInt.NotNull()).CountX(ctx))
	})

	t.Run("Arrays", func(t *testing.T) {
		users := createUsers("array-a", "array-b", "array-c")
		updated := client.User.UpdateBulk(
			client.User.UpdateOne(users[2]).Set(user.Tags, []string{}).Set(user.Scores, []int{7, 8}),
			client.User.UpdateOne(users[1]).Clear(user.Tags).Add(user.Age, 1),
			client.User.UpdateOne(users[0]).Set(user.Tags, []string{"x", "y"}).Set(user.Scores, []int{1}),
		).SaveX(ctx)
		require.Equal(t, []int{users[2].ID, users[1].ID, users[0].ID}, entityIDs(updated), "results follow the order of the builders")
		require.Equal(t, []string{}, updated[0].Tags)
		require.Equal(t, []int{7, 8}, updated[0].Scores)
		require.Nil(t, updated[1].Tags)
		require.Nil(t, updated[1].Scores)
		require.Equal(t, 21, updated[1].Age)
		require.Equal(t, []string{"x", "y"}, updated[2].Tags)
		require.Equal(t, []int{1}, updated[2].Scores)
		for _, entity := range updated {
			require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
		}
	})

	t.Run("FieldTypes", func(t *testing.T) {
		start := time.Now().Truncate(time.Second)
		mac, err := net.ParseMAC("3b:b3:6b:3c:10:79")
		require.NoError(t, err)
		fields := client.FieldType.CreateBulk(
			client.FieldType.Create().Set(fieldtype.Int, 1).Set(fieldtype.Int8, 8).Set(fieldtype.Int16, 16).Set(fieldtype.Int32, 32).Set(fieldtype.Int64, 64).
				Set(fieldtype.NillableInt, 1),
			client.FieldType.Create().Set(fieldtype.Int, 2).Set(fieldtype.Int8, 8).Set(fieldtype.Int16, 16).Set(fieldtype.Int32, 32).Set(fieldtype.Int64, 64).
				Set(fieldtype.NillableInt, 2).Set(fieldtype.Strings, []string{"initial"}),
		).SaveX(ctx)
		updated := client.FieldType.UpdateBulk(
			client.FieldType.UpdateOne(fields[0]).Set(fieldtype.Int, 10).Set(fieldtype.NillableInt, 5).Set(fieldtype.State, fieldtype.StateOn).
				Set(fieldtype.Strings, []string{"a", "b"}).Set(fieldtype.Duration, time.Hour).Set(fieldtype.MAC, schema.MAC{HardwareAddr: mac}),
			client.FieldType.UpdateOne(fields[1]).Add(fieldtype.Int, 3).Clear(fieldtype.NillableInt).Set(fieldtype.Role, role.Admin).
				Clear(fieldtype.Strings),
		).SaveX(ctx)
		require.Equal(t, 10, updated[0].Int)
		require.Equal(t, 5, *updated[0].NillableInt)
		require.Equal(t, fieldtype.StateOn, updated[0].State)
		require.Equal(t, []string{"a", "b"}, updated[0].Strings)
		require.Equal(t, time.Hour, updated[0].Duration)
		require.Equal(t, mac.String(), updated[0].MAC.String())
		require.Equal(t, role.Read, updated[0].Role)
		require.Equal(t, 5, updated[1].Int)
		require.Nil(t, updated[1].NillableInt)
		require.Equal(t, role.Admin, updated[1].Role)
		require.Nil(t, updated[1].Strings)
		require.Equal(t, time.Duration(100), updated[1].Duration, "update default applies to rows that do not set the field")
		for _, entity := range updated {
			require.Equal(t, int64(100), entity.Int64, "update default applies to every row")
			require.False(t, entity.DeletedAt.Time.Before(start), "update default is refreshed")
			require.Equal(t, entity.String(), client.FieldType.GetX(ctx, entity.ID).String())
		}

		updated = client.FieldType.UpdateBulk(
			client.FieldType.UpdateOne(updated[0]).Set(fieldtype.StringArray, schema.Strings{"x", "y"}),
			client.FieldType.UpdateOne(updated[1]).Add(fieldtype.Int, 1),
		).SaveX(ctx)
		require.Equal(t, schema.Strings{"x", "y"}, updated[0].StringArray)
		require.Equal(t, 6, updated[1].Int)
		for _, entity := range updated {
			require.Equal(t, entity.String(), client.FieldType.GetX(ctx, entity.ID).String())
		}

		_, err = client.FieldType.UpdateBulk(
			client.FieldType.UpdateOne(updated[0]).Set(fieldtype.Int, 100),
			client.FieldType.UpdateOne(updated[1]).Append(fieldtype.Strings, []string{"appended"}),
		).Save(ctx)
		require.Error(t, err)
		require.Equal(t, updated[0].Int, client.FieldType.GetX(ctx, updated[0].ID).Int)
		require.Nil(t, client.FieldType.GetX(ctx, updated[1].ID).Strings)

		// The update default sets the field that Add adds to.
		single := client.FieldType.UpdateOneID(fields[0].ID).Add(fieldtype.Int64, 3).SaveX(ctx)
		updated = client.FieldType.UpdateBulk(
			client.FieldType.UpdateOneID(fields[1].ID).Add(fieldtype.Int64, 3),
			client.FieldType.UpdateOneID(fields[0].ID).Set(fieldtype.Int, 7),
		).SaveX(ctx)
		require.Equal(t, int64(103), single.Int64)
		require.Equal(t, single.Int64, updated[0].Int64)
		require.Equal(t, int64(100), updated[1].Int64)
		for _, entity := range updated {
			require.Equal(t, entity.String(), client.FieldType.GetX(ctx, entity.ID).String())
		}
	})

	t.Run("JSON", func(t *testing.T) {
		tasks := client.Task.CreateBulk(
			client.Task.Create().Set(enttask.Priorities, map[string]task.Priority{"initial": task.PriorityLow}),
			client.Task.Create().Set(enttask.Priorities, map[string]task.Priority{"initial": task.PriorityLow}),
		).SaveX(ctx)
		updated := client.Task.UpdateBulk(
			client.Task.UpdateOne(tasks[0]).Set(enttask.Priorities, map[string]task.Priority{"a": task.PriorityHigh, "b": task.PriorityMid}),
			client.Task.UpdateOne(tasks[1]).Clear(enttask.Priorities).Set(enttask.Priority, task.PriorityHigh),
		).SaveX(ctx)
		require.Equal(t, map[string]task.Priority{"a": task.PriorityHigh, "b": task.PriorityMid}, updated[0].Priorities)
		require.Equal(t, task.PriorityMid, updated[0].Priority)
		require.Nil(t, updated[1].Priorities)
		require.Equal(t, task.PriorityHigh, updated[1].Priority)
		for _, entity := range updated {
			stored := client.Task.GetX(ctx, entity.ID)
			require.Equal(t, entity.Priorities, stored.Priorities)
			require.Equal(t, entity.Priority, stored.Priority)
		}
	})

	t.Run("Edges", func(t *testing.T) {
		users := createUsers("edge-a", "edge-b")
		pets := client.Pet.CreateBulk(
			client.Pet.Create().Set(pet.Name, "edge-owned").SetEdge(pet.Owner, users[0].ID),
			client.Pet.Create().Set(pet.Name, "edge-orphan"),
			client.Pet.Create().Set(pet.Name, "edge-kept").SetEdge(pet.Owner, users[0].ID),
		).SaveX(ctx)
		updated := client.Pet.UpdateBulk(
			client.Pet.UpdateOne(pets[0]).ClearEdge(pet.Owner),
			client.Pet.UpdateOne(pets[1]).SetEdge(pet.Owner, users[1].ID),
			client.Pet.UpdateOne(pets[2]).Set(pet.Name, "edge-renamed"),
		).SaveX(ctx)
		require.Equal(t, "edge-renamed", updated[2].Name)
		require.False(t, pets[0].QueryOwner().ExistX(ctx))
		require.Equal(t, users[1].ID, pets[1].QueryOwner().OnlyIDX(ctx))
		require.Equal(t, users[0].ID, pets[2].QueryOwner().OnlyIDX(ctx))
	})

	t.Run("Columns", func(t *testing.T) {
		users := createUsers("columns-a", "columns-b")
		updated := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Set(user.Name, "columns-a2").Set(user.Age, 99).Columns(user.Name),
			client.User.UpdateOne(users[1]).Set(user.Name, "columns-b2"),
		).SaveX(ctx)
		require.Equal(t, users[0].ID, updated[0].ID)
		require.Equal(t, "columns-a2", updated[0].Name)
		require.Zero(t, updated[0].Age, "columns outside the selection are not populated")
		require.Empty(t, updated[0].Nickname)
		require.Equal(t, client.User.GetX(ctx, users[1].ID).String(), updated[1].String())
		require.Equal(t, 99, client.User.GetX(ctx, users[0].ID).Age)
	})

	t.Run("NoChanges", func(t *testing.T) {
		users := createUsers("unchanged-a", "unchanged-b", "unchanged-c")
		updated := client.User.UpdateBulk(
			client.User.UpdateOneID(users[2].ID),
			client.User.UpdateOneID(users[0].ID).Columns(user.Name),
			client.User.UpdateOne(users[1]),
		).SaveX(ctx)
		require.Equal(t, []int{users[2].ID, users[0].ID, users[1].ID}, entityIDs(updated), "results follow the order of the builders")
		require.Equal(t, client.User.GetX(ctx, users[2].ID).String(), updated[0].String())
		require.Equal(t, "unchanged-a", updated[1].Name)
		require.Zero(t, updated[1].Age, "columns outside the selection are not populated")
		require.Equal(t, client.User.GetX(ctx, users[1].ID).String(), updated[2].String())
		require.NoError(t, client.User.UpdateBulk(client.User.UpdateOneID(users[0].ID), client.User.UpdateOneID(users[1].ID)).Exec(ctx))

		client.User.DeleteOne(users[2]).ExecX(ctx)
		_, err := client.User.UpdateBulk(client.User.UpdateOneID(users[0].ID), client.User.UpdateOneID(users[2].ID)).Save(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		err = client.User.UpdateBulk(client.User.UpdateOneID(users[0].ID), client.User.UpdateOneID(users[2].ID)).Exec(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)

		_, err = client.User.UpdateBulk(
			client.User.UpdateOneID(users[0].ID),
			client.User.UpdateOneID(users[1].ID),
		).Where(user.Name.NEQ("unchanged-b")).Save(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		for _, entity := range users[:2] {
			require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
		}
	})

	t.Run("Where", func(t *testing.T) {
		users := createUsers("where-a", "where-b")
		updated := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Add(user.Age, 1),
			client.User.UpdateOne(users[1]).Add(user.Age, 1),
		).Where(user.Name.HasPrefix("where-")).SaveX(ctx)
		require.Equal(t, 11, updated[0].Age)
		require.Equal(t, 21, updated[1].Age)

		_, err := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Add(user.Age, 1),
			client.User.UpdateOne(users[1]).Add(user.Age, 1),
		).Where(user.Age.LT(15)).Save(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		require.Equal(t, 11, client.User.GetX(ctx, users[0].ID).Age, "matched rows roll back")
		require.Equal(t, 21, client.User.GetX(ctx, users[1].ID).Age)

		updated = client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Add(user.Age, 1),
			client.User.UpdateOne(users[1]).Set(user.Name, "where-b2"),
		).Where(user.Tags.HasAny("where-a", "where-b"), user.Tags.Contains(), user.Tags.LenEQ(1)).SaveX(ctx)
		require.Equal(t, 12, updated[0].Age)
		require.Equal(t, "where-b2", updated[1].Name)
	})

	t.Run("WhereBeforeBatch", func(t *testing.T) {
		create := func() (*ent.User, *ent.User) {
			parent := client.User.Create().Set(user.Name, "before-b").Set(user.Age, 1).SaveX(ctx)
			return client.User.Create().Set(user.Name, "before-a").Set(user.Age, 1).SaveX(ctx), parent
		}
		child, parent := create()
		err := client.User.UpdateBulk(
			client.User.UpdateOneID(child.ID).SetEdge(user.Parent, parent.ID),
			client.User.UpdateOneID(parent.ID).Set(user.Age, 2),
		).Where(user.Children.Has().Not()).Exec(ctx)
		require.NoError(t, err, "parent has no children before the batch")
		require.Equal(t, parent.ID, child.QueryParent().OnlyIDX(ctx))
		require.Equal(t, 2, client.User.GetX(ctx, parent.ID).Age)

		child, parent = create()
		_, err = client.User.UpdateBulk(
			client.User.UpdateOneID(child.ID).SetEdge(user.Parent, parent.ID),
			client.User.UpdateOneID(parent.ID).Set(user.Age, 9),
		).Where(entgo.Or(user.ID.EQ(child.ID), user.Children.Has())).Save(ctx)
		require.True(t, ent.IsNotFound(err), "parent has no children before the batch, got %v", err)
		require.Equal(t, 1, client.User.GetX(ctx, parent.ID).Age)
		require.False(t, child.QueryParent().ExistX(ctx))
	})

	t.Run("SetAndAdd", func(t *testing.T) {
		users := createUsers("set-add-a", "set-add-b", "set-add-c", "set-add-d")
		single := client.User.UpdateOne(users[2]).Set(user.Age, 1).Add(user.Age, 1).SaveX(ctx)
		cleared := client.User.UpdateOne(users[3]).Clear(user.OptionalInt).Add(user.OptionalInt, 2).SaveX(ctx)
		updated := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Set(user.Age, 1).Add(user.Age, 1),
			client.User.UpdateOne(users[1]).Clear(user.OptionalInt).Add(user.OptionalInt, 2),
		).SaveX(ctx)
		require.Equal(t, 2, single.Age)
		require.Equal(t, single.Age, updated[0].Age)
		require.Equal(t, users[3].OptionalInt+2, cleared.OptionalInt)
		require.Equal(t, users[1].OptionalInt+2, updated[1].OptionalInt)
		for _, entity := range updated {
			require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
		}

		builders := make([]*ent.FieldTypeCreate, 3)
		for index := range builders {
			builders[index] = client.FieldType.Create().Set(fieldtype.Int, 1).Set(fieldtype.Int8, 8).Set(fieldtype.Int16, 16).Set(fieldtype.Int32, 32).Set(fieldtype.Int64, 64)
		}
		fields := client.FieldType.CreateBulk(builders...).SaveX(ctx)
		first, second := 0.1, 0.2
		setAdd := func(entity *ent.FieldType) *ent.FieldTypeUpdateOne {
			return client.FieldType.UpdateOne(entity).Set(fieldtype.OptionalFloat, first).Add(fieldtype.OptionalFloat, second).
				Set(fieldtype.OptionalInt64, 5_000_000_000).Add(fieldtype.OptionalInt64, 1).Add(fieldtype.Int64, 5_000_000_000)
		}
		singleField := setAdd(fields[0]).SaveX(ctx)
		updatedFields := client.FieldType.UpdateBulk(setAdd(fields[1]), setAdd(fields[2])).SaveX(ctx)
		for _, entity := range updatedFields {
			require.Equal(t, singleField.OptionalFloat, entity.OptionalFloat)
			require.Equal(t, singleField.OptionalInt64, entity.OptionalInt64)
			require.Equal(t, singleField.Int64, entity.Int64)
			require.Equal(t, entity.String(), client.FieldType.GetX(ctx, entity.ID).String())
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		users := createUsers("missing-a", "missing-b")
		client.User.DeleteOne(users[1]).ExecX(ctx)
		_, err := client.User.UpdateBulk(client.User.UpdateOne(users[0]).Set(user.Name, "missing-a2"), client.User.UpdateOne(users[1]).Set(user.Name, "missing-b2")).Save(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		require.Equal(t, "missing-a", client.User.GetX(ctx, users[0].ID).Name)
		err = client.User.UpdateBulk(client.User.UpdateOne(users[0]).Set(user.Name, "missing-a2"), client.User.UpdateOne(users[1]).Set(user.Name, "missing-b2")).Exec(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		require.Equal(t, "missing-a", client.User.GetX(ctx, users[0].ID).Name)
	})

	t.Run("Rejected", func(t *testing.T) {
		users := createUsers("rejected-a", "rejected-b")
		for _, tt := range []struct {
			name      string
			builder   *ent.UserUpdateOne
			validator bool
			// unchanged asserts what the rows do not show.
			unchanged func(t *testing.T)
		}{
			{name: "Validator", builder: client.User.UpdateOne(users[1]).Set(user.OptionalInt, -1), validator: true},
			{name: "BuilderError", builder: client.User.UpdateOne(users[1]).Append(user.Name, "invalid"), validator: true},
			{name: "DuplicateID", builder: client.User.UpdateOneID(users[0].ID).Set(user.Age, 1)},
			{name: "Expression", builder: client.User.UpdateOne(users[1]).SetExpr(user.Age, ent.Literal(1))},
			{name: "Modifier", builder: client.User.UpdateOne(users[1]).Modify(func(update *sql.UpdateBuilder) { update.Set(user.FieldAge, 1) })},
			{name: "RowPredicate", builder: client.User.UpdateOne(users[1]).Set(user.Age, 1).Where(user.Age.GT(0))},
			{name: "ManyToMany", builder: client.User.UpdateOne(users[1]).AddIDs(user.Friends, users[0].ID), unchanged: func(t *testing.T) {
				require.False(t, users[1].QueryFriends().ExistX(ctx))
			}},
			{name: "OneToMany", builder: client.User.UpdateOne(users[1]).ClearEdge(user.Pets)},
			{name: "OneToOne", builder: client.User.UpdateOne(users[1]).ClearEdge(user.Card)},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := client.User.UpdateBulk(client.User.UpdateOne(users[0]).Set(user.Name, "rejected-changed"), tt.builder).Save(ctx)
				require.Error(t, err)
				require.Equal(t, tt.validator, ent.IsValidationError(err), "got %v", err)
				require.False(t, ent.IsNotFound(err))
				for _, entity := range users {
					require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
				}
				if tt.unchanged != nil {
					tt.unchanged(t)
				}
			})
		}
	})

	t.Run("Constraint", func(t *testing.T) {
		users := createUsers("constraint-a", "constraint-b")
		_, err := client.User.UpdateBulk(
			client.User.UpdateOne(users[0]).Set(user.Name, "constraint-a2"),
			client.User.UpdateOne(users[1]).SetOptional(user.Nickname, ent.Some(users[0].Nickname)),
		).Save(ctx)
		require.True(t, ent.IsConstraintError(err), "got %v", err)
		for _, entity := range users {
			require.Equal(t, entity.String(), client.User.GetX(ctx, entity.ID).String())
		}
	})

	t.Run("Tx", func(t *testing.T) {
		users := createUsers("tx-a", "tx-b")
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		updated := tx.User.UpdateBulk(
			tx.User.UpdateOneID(users[0].ID).Set(user.Name, "tx-a2"),
			tx.User.UpdateOneID(users[1].ID).Add(user.Age, 1),
		).SaveX(ctx)
		require.Equal(t, "tx-a2", updated[0].Name)
		require.Equal(t, "tx-a2", tx.User.GetX(ctx, users[0].ID).Name, "the update is visible inside the transaction")
		require.NoError(t, tx.Rollback())
		require.Equal(t, "tx-a", client.User.GetX(ctx, users[0].ID).Name)
		require.Equal(t, 20, client.User.GetX(ctx, users[1].ID).Age)

		tx, err = client.Tx(ctx)
		require.NoError(t, err)
		tx.User.UpdateBulk(
			tx.User.UpdateOneID(users[0].ID).Set(user.Name, "tx-a3"),
			tx.User.UpdateOneID(users[1].ID).Add(user.Age, 2),
		).ExecX(ctx)
		require.NoError(t, tx.Commit())
		require.Equal(t, "tx-a3", client.User.GetX(ctx, users[0].ID).Name)
		require.Equal(t, 22, client.User.GetX(ctx, users[1].ID).Age)
	})

	t.Run("Empty", func(t *testing.T) {
		updated, err := client.User.UpdateBulk().Save(ctx)
		require.NoError(t, err)
		require.Empty(t, updated)
		require.NoError(t, client.User.UpdateBulk().Exec(ctx))
	})
}
