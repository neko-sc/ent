// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	stdsql "database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	sqlschema "github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/dialect/sql/sqlgraph"
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/card"
	comment "github.com/neko-sc/ent/entc/integration/ent/comment"
	"github.com/neko-sc/ent/entc/integration/ent/enttest"
	"github.com/neko-sc/ent/entc/integration/ent/exvaluescan"
	"github.com/neko-sc/ent/entc/integration/ent/file"
	"github.com/neko-sc/ent/entc/integration/ent/filetype"
	"github.com/neko-sc/ent/entc/integration/ent/group"
	"github.com/neko-sc/ent/entc/integration/ent/groupinfo"
	"github.com/neko-sc/ent/entc/integration/ent/item"
	"github.com/neko-sc/ent/entc/integration/ent/license"
	"github.com/neko-sc/ent/entc/integration/ent/migrate"
	"github.com/neko-sc/ent/entc/integration/ent/node"
	"github.com/neko-sc/ent/entc/integration/ent/pet"
	"github.com/neko-sc/ent/entc/integration/ent/schema"
	"github.com/neko-sc/ent/entc/integration/ent/schema/task"
	enttask "github.com/neko-sc/ent/entc/integration/ent/task"
	"github.com/neko-sc/ent/entc/integration/ent/user"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSQLite(t *testing.T) {
	t.Parallel()
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&cache=shared&_fk=1", opts)
	defer client.Close()
	for _, tt := range tests {
		name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
		t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
			drop(t, client)
			tt(t, client)
		})
	}
}

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{
		"16": 5436,
		"17": 5437,
		"18": 5438,
	} {
		addr := fmt.Sprintf("host=localhost port=%d user=postgres dbname=test password=pass sslmode=disable", port)
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			client := enttest.Open(t, dialect.Postgres, addr, opts)
			defer client.Close()
			for _, tt := range tests {
				name := runtime.FuncForPC(reflect.ValueOf(tt).Pointer()).Name()
				t.Run(name[strings.LastIndex(name, ".")+1:], func(t *testing.T) {
					drop(t, client)
					tt(t, client)
				})
			}
		})
	}
}

var (
	opts = enttest.WithMigrateOptions(
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
	)
	tests = [...]func(*testing.T, *ent.Client){
		Sanity,
		Arrays,
		WritePath,
		ReadPath,
		NoSchemaChanges,
		Tx,
		Lock,
		Indexes,
		Types,
		Clone,
		Paging,
		Select,
		Aggregate,
		Delete,
		Upsert,
		Relation,
		ExecQuery,
		Predicate,
		AddValues,
		ClearEdges,
		ClearFields,
		UniqueConstraint,
		O2OTwoTypes,
		O2OSameType,
		O2OSelfRef,
		O2MTwoTypes,
		O2MSameType,
		M2MSelfRef,
		M2MSameType,
		M2MTwoTypes,
		DefaultValue,
		Sensitive,
		EagerLoading,
		NamedEagerLoading,
		Mutation,
		CreateBulk,
		ConstraintChecks,
		NillableRequired,
		ExtValueScan,
		OrderByEdgeCount,
		OrderByEdgeTerms,
		OrderByFluent,
	}
)

func Sanity(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().Set(user.Name, "foo").Set(user.Age, 20).SaveX(ctx)
	client.User.Update().ExecX(ctx)
	client.User.UpdateOne(usr).ExecX(ctx)
	require.Equal("foo", usr.Name)
	require.Equal(20, usr.Age)
	require.NotEmpty(usr.ID)
	client.User.Query().OnlyX(ctx)
	client.User.Delete().ExecX(ctx)
	require.Empty(client.User.Query().AllX(ctx))
	pt := client.Pet.Create().Set(pet.Name, "pedro").SaveX(ctx)
	usr = client.User.Create().Set(user.Name, "foo").Set(user.Age, 20).AddIDs(user.Pets, pt.ID).SaveX(ctx)
	child := client.User.Create().Set(user.Name, "bar").Set(user.Age, 20).AddIDs(user.Children, usr.ID).SaveX(ctx)
	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	grp := client.Group.Create().Set(group.Name, "Github").Set(group.Expire, time.Now()).AddIDs(group.Users, usr.ID, child.ID).SetEdge(group.Info, inf.ID).SaveX(ctx)
	require.Equal(1, client.Group.Query().CountX(ctx))
	require.Zero(client.Group.Query().Where(group.Active.EQ(false)).CountX(ctx))
	require.Len(grp.QueryUsers().AllX(ctx), 2)
	usr.QueryGroups().OnlyX(ctx)
	child.QueryGroups().OnlyX(ctx)
	usr2 := client.User.Create().Set(user.Name, "qux").Set(user.Age, 20).SetEdge(user.Spouse, usr.ID).SaveX(ctx)
	usr2.QuerySpouse().OnlyX(ctx)
	usr.QuerySpouse().OnlyX(ctx)
	require.Equal(usr.Name, usr.QueryPets().QueryOwner().OnlyX(ctx).Name)
	require.Equal(pt.Name, usr.QueryPets().QueryOwner().QueryPets().OnlyX(ctx).Name)
	require.Empty(usr.QuerySpouse().QueryPets().AllX(ctx))
	require.Equal(pt.Name, usr2.QuerySpouse().QueryPets().OnlyX(ctx).Name)
	require.Len(usr.QueryGroups().QueryUsers().AllX(ctx), 2)
	require.Len(usr.QueryGroups().QueryUsers().QueryGroups().AllX(ctx), 1, "should be unique by default")
	require.Len(usr.QueryGroups().AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Pets.Has()).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Spouse.Has()).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Not(user.Spouse.Has())).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Groups.Has()).AllX(ctx), 2)
	require.Len(client.Group.Query().Where(group.Users.Has()).AllX(ctx), 1)
	require.Len(client.Group.Query().Where(group.Users.HasWith(user.Name.EQ("foo"))).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Groups.HasWith(group.Name.HasPrefix("G"))).AllX(ctx), 2)
	require.Equal(3, client.User.Query().CountX(ctx))
	require.Equal(client.Group.Query().Where(group.Users.HasWith(user.Name.EQ("foo"))).CountX(ctx), 1)
	require.True(client.User.Query().ExistX(ctx))
	require.True(client.User.Query().Where(user.Pets.HasWith(pet.Name.HasPrefix("ped"))).ExistX(ctx))
	require.False(client.User.Query().Where(user.Pets.HasWith(pet.Name.HasPrefix("pan"))).ExistX(ctx))
	require.Equal(child.Name, client.User.Query().Order(user.Name.Asc()).FirstX(ctx).Name)
	require.Equal(usr2.Name, client.User.Query().Order(user.Name.Desc()).FirstX(ctx).Name)
	// Update fields.
	client.User.Update().Where(user.ID.EQ(child.ID)).Set(user.Name, "Ariel").SaveX(ctx)
	client.User.Query().Where(user.Name.EQ("Ariel")).OnlyX(ctx)
	// Update edges.
	require.Empty(child.QueryPets().AllX(ctx))
	require.NoError(client.Pet.UpdateOne(pt).ClearEdge(pet.Owner).Exec(ctx))
	client.User.Update().Where(user.ID.EQ(child.ID)).AddIDs(user.Pets, pt.ID).SaveX(ctx)
	require.NotEmpty(child.QueryPets().AllX(ctx))
	client.User.Update().Where(user.ID.EQ(child.ID)).RemoveIDs(user.Pets, pt.ID).SaveX(ctx)
	require.Empty(child.QueryPets().AllX(ctx))
	// Remove edges.
	client.User.Update().ClearEdge(user.Spouse).SaveX(ctx)
	require.Empty(client.User.Query().Where(user.Spouse.Has()).AllX(ctx))
	client.User.Update().AddIDs(user.Friends, child.ID).RemoveIDs(user.Groups, grp.ID).Where(user.ID.EQ(usr.ID)).SaveX(ctx)
	require.NotEmpty(child.QueryGroups().AllX(ctx))
	require.Empty(usr.QueryGroups().AllX(ctx))
	require.Len(child.QueryFriends().AllX(ctx), 1)
	require.Len(usr.QueryFriends().AllX(ctx), 1)
	// Update one node.
	usr = client.User.UpdateOne(usr).Set(user.Name, "baz").AddIDs(user.Groups, grp.ID).SaveX(ctx)
	require.Equal("baz", usr.Name)
	require.NotEmpty(usr.QueryGroups().AllX(ctx))
	// Update unknown node.
	err := client.User.UpdateOneID(usr.ID+math.MaxInt8).Set(user.Name, "foo").Exec(ctx)
	require.Error(err)
	require.True(ent.IsNotFound(err))
	// Update a vertex with filter.
	u := client.User.UpdateOneID(usr.ID).Set(user.Name, "foo")
	u.Mutation().Where(user.Name.EQ(usr.Name))
	require.NoError(u.Exec(ctx))
	u = client.User.UpdateOneID(usr.ID).Set(user.Name, "bar")
	u.Mutation().Where(user.Name.EQ("baz"))
	require.Error(u.Exec(ctx))
	require.True(ent.IsNotFound(err))

	var v []struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Sum   int    `json:"sum"`
		Count int    `json:"count"`
	}
	require.NoError(client.User.Query().
		GroupBy(user.Name, user.Age).
		Aggregate(ent.Count(), ent.Sum(user.Age).As("sum")).Scan(ctx, &v))
	require.NotEmpty(v)
	// IN predicates.
	ids := client.User.Query().IDsX(ctx)
	require.Len(ids, 3)
	client.User.Delete().Where(user.ID.In(ids...)).ExecX(ctx)
	ids = client.User.Query().IDsX(ctx)
	require.Empty(ids)
	// Nop.
	client.User.Delete().Where(user.ID.In(ids...)).ExecX(ctx)
	// Check the struct-tag annotation.
	fi, ok := reflect.TypeOf(ent.Card{}).FieldByName("Edges")
	require.True(ok)
	require.NotEmpty(fi.Tag.Get("mashraki"))
	fi, ok = reflect.TypeOf(ent.Card{}).FieldByName("ID")
	require.True(ok)
	require.Equal("-", fi.Tag.Get("json"))
	fi, ok = reflect.TypeOf(ent.Card{}).FieldByName("Number")
	require.True(ok)
	require.Equal("-", fi.Tag.Get("json"))
	client.User.Create().Set(user.Name, "tarrence").Set(user.Age, 30).ExecX(ctx)

	t.Run("StringPredicates", func(t *testing.T) {
		client.Pet.Delete().ExecX(ctx)
		a := client.Pet.Create().Set(pet.Name, "a%").SaveX(ctx)
		require.True(client.Pet.Query().Where(pet.Name.HasPrefix("a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.HasPrefix("%a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Or(pet.Name.HasPrefix("%a%"), pet.Name.HasPrefix("%a%"))).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.HasSuffix("%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.HasSuffix("a%%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.Contains("a")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.Contains("a%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.Contains("%a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.ContainsFold("A%")).ExistX(ctx))

		a.Update().Set(pet.Name, "a_\\").ExecX(ctx)
		require.True(client.Pet.Query().Where(pet.Name.HasPrefix("a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.HasPrefix("%a")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.HasPrefix("a_")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.HasSuffix("a_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.HasSuffix("%a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.HasSuffix("a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.Contains("a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.Contains("%a")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.Contains("a%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.ContainsFold("A")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.ContainsFold("%A")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.ContainsFold("A%")).ExistX(ctx))
		require.True(client.Pet.Query().Where(pet.Name.EqualFold("A_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.EqualFold("%A_\\")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.EqualFold("A_\\%")).ExistX(ctx))
		require.False(client.Pet.Query().Where(pet.Name.EqualFold("A%")).ExistX(ctx))
	})
}

func Upsert(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	u := client.User.Create().Set(user.Name, "Ariel").Set(user.Age, 30).Set(user.Phone, "0000").SaveX(ctx)
	require.Equal(t, "static", u.Address, "address was set by default func")
	err := client.User.Create().Set(user.Name, "Mashraki").Set(user.Age, 30).Set(user.Phone, "0000").Exec(ctx)
	require.True(t, ent.IsConstraintError(err), "phone field is unique")
	err = client.User.Create().Set(user.Name, "Mashraki").Set(user.Age, 30).Set(user.Phone, "0000").OnConflict().Exec(ctx)
	require.EqualError(t, err, "ent: missing options for UserCreate.OnConflict")

	client.User.Create().
		Set(user.Name, "Mashraki").
		Set(user.Age, 30).
		Set(user.Phone, "0000").
		OnConflictOptions(
			sql.ConflictColumns(user.FieldPhone),
		).
		// Update "name" to the value that was set on create ("Mashraki").
		DoUpdate(func(update *ent.UserUpsert) { update.UpdateNewValue(user.Name) }).
		ExecX(ctx)
	u = client.User.GetX(ctx, u.ID)
	require.Equal(t, "Mashraki", u.Name, "name was changed by the UPDATE clause")

	id := client.User.Create().
		Set(user.Name, "Boring").
		Set(user.Age, 33).
		Set(user.Phone, "0000").
		OnConflict(user.Phone).
		// Override some fields with custom update.
		DoUpdate(func(u *ent.UserUpsert) {
			// Age was set to the new value (33).
			u.UpdateNewValue(user.Age)
			// Update an additional field that was defined in `VALUES`.
			u.Set(user.Address, "localhost")
		}).
		IDX(ctx)
	require.Equal(t, u.ID, id)
	u = client.User.GetX(ctx, u.ID)
	require.Equal(t, "Mashraki", u.Name)
	require.Equal(t, 33, u.Age, "age was modified by the UPDATE clause")
	require.Equal(t, "localhost", u.Address, "address was modified by the UPDATE clause")

	id = client.User.Create().
		Set(user.Name, "Boring").
		Set(user.Age, 33).
		Set(user.Phone, "0000").
		OnConflict(user.Phone).
		// Override some fields with custom update.
		DoUpdate(func(update *ent.UserUpsert) { update.Add(user.Age, -1) }).
		IDX(ctx)
	u = client.User.GetX(ctx, id)
	require.Equal(t, 32, u.Age, "age was modified by the UPDATE clause")

	builders := []*ent.UserCreate{
		client.User.Create().Set(user.Name, "A").Set(user.Age, 1).Set(user.Phone, "0000"), // Duplicate
		client.User.Create().Set(user.Name, "B").Set(user.Age, 1).Set(user.Phone, "1111"), // New row.
	}
	client.User.CreateBulk(builders...).
		OnConflict(user.Phone).
		UpdateNewValues().
		ExecX(ctx)
	users := client.User.Query().Order(user.Phone.Asc()).AllX(ctx)
	require.Equal(t, "0000", users[0].Phone)
	require.Equal(t, "A", users[0].Name)
	require.Equal(t, "1111", users[1].Phone)
	require.Equal(t, "B", users[1].Name)

	// Setting primary key manually.
	a := client.Item.Create().Set(item.ID, "A").SaveX(ctx)
	require.Equal(t, "A", a.ID)
	aid := client.Item.Create().Set(item.ID, "A").OnConflictOptions(sql.ConflictColumns(item.FieldID)).Ignore().IDX(ctx)
	require.Equal(t, a.ID, aid)
	client.Item.Delete().ExecX(ctx)

	// Primary key is set by a default function.
	b := client.Item.Create().Set(item.Text, "hello").SaveX(ctx)
	require.NotZero(t, b.ID)
	bid := client.Item.Create().Set(item.ID, b.ID).Set(item.Text, "hello").OnConflict(item.Text).Ignore().IDX(ctx)
	require.Equal(t, b.ID, bid)
	bid = client.Item.Create().Set(item.Text, "hello").OnConflict(item.Text).UpdateNewValues().IDX(ctx)
	require.Equal(t, bid, b.ID)
	require.Equal(t, bid, client.Item.Query().OnlyIDX(ctx))
	bid = client.Item.Create().Set(item.ID, bid).Set(item.Text, "world").OnConflict(item.ID).UpdateNewValues().IDX(ctx)
	require.Equal(t, bid, b.ID)
	b = client.Item.Query().OnlyX(ctx)
	require.Equal(t, bid, b.ID)
	require.Equal(t, "world", b.Text)

	client.Item.CreateBulk(client.Item.Create().Set(item.ID, bid).Set(item.Text, "hello")).
		OnConflict(item.ID).
		Ignore().
		ExecX(ctx)
	require.Equal(t, bid, client.Item.Query().OnlyIDX(ctx))

	ts := time.Unix(1623279251, 0)
	c1 := client.Card.Create().
		Set(card.Number, "102030").
		Set(card.CreateTime, ts).
		Set(card.UpdateTime, ts).
		SaveX(ctx)

	// "DO UPDATE SET ... WHERE ..." is supported by PostgreSQL and SQLite.
	err = client.Card.Create().
		Set(card.Number, c1.Number).
		OnConflictOptions(
			sql.ConflictColumns(card.FieldNumber),
			sql.UpdateWhere(sql.NEQ(card.FieldCreateTime, ts)),
		).
		UpdateNewValues().
		Exec(ctx)
	// Only rows for which the "UpdateWhere" expression
	// returns true will be updated. That is, none.
	require.NoError(t, err)

	id = client.Card.Create().
		Set(card.Number, c1.Number).
		OnConflictOptions(
			sql.ConflictColumns(card.FieldNumber),
			sql.UpdateWhere(sql.EQ(card.FieldCreateTime, ts)),
		).
		UpdateNewValues().
		IDX(ctx)

	// Ensure immutable fields were not changed during upsert.
	c2 := client.Card.GetX(ctx, id)
	require.Equal(t, c1.CreateTime.Unix(), c2.CreateTime.Unix())
	require.NotEqual(t, c1.UpdateTime.Unix(), c2.UpdateTime.Unix())

	// Ensure immutable fields were not changed during bulk upsert.
	l1 := client.License.Create().Set(license.CreateTime, ts).Set(license.UpdateTime, ts).SaveX(ctx)
	client.License.CreateBulk(client.License.Create().Set(license.ID, l1.ID)).
		OnConflict(license.ID).
		UpdateNewValues().
		ExecX(ctx)
	l2 := client.License.GetX(ctx, l1.ID)
	require.Equal(t, l1.CreateTime.Unix(), l2.CreateTime.Unix())
	require.NotEqual(t, l1.UpdateTime.Unix(), l2.UpdateTime.Unix())

	c3 := client.Card.Create().Set(card.Name, "a8m").Set(card.Number, "405060").SaveX(ctx)
	client.Card.Create().Set(card.Number, c3.Number).OnConflict(card.Number).DoUpdate(func(update *ent.CardUpsert) { update.Clear(card.Name) }).UpdateNewValues().ExecX(ctx)
	require.Empty(t, client.Card.GetX(ctx, c3.ID).Name)
	c3.Update().Set(card.Name, "a8m").ExecX(ctx)
	client.Card.CreateBulk(client.Card.Create().Set(card.Number, c3.Number), client.Card.Create().Set(card.Number, "708090").Set(card.Name, "m8a")).
		OnConflict(card.Number).
		UpdateNewValues().
		ExecX(ctx)
	require.Empty(t, client.Card.GetX(ctx, c3.ID).Name, "existing name fields should be cleared when not set (= set to nil)")
	require.NotEmpty(t, client.Card.Query().Where(card.Number.EQ("708090")).OnlyX(ctx).Name, "new record should set their name")

	// Conflict on a composite unique index.
	t1 := client.Task.Create().Set(enttask.Name, "todo1").Set(enttask.Owner, "a8m").Set(enttask.Priority, task.PriorityLow).SaveX(ctx)
	tid := client.Task.Create().
		Set(enttask.Name, "todo1").
		Set(enttask.Owner, "a8m").
		Set(enttask.Priority, task.PriorityHigh).
		OnConflict(enttask.Name, enttask.Owner).
		DoUpdate(func(update *ent.TaskUpsert) { update.UpdateNewValue(enttask.Priority) }).
		IDX(ctx)
	require.Equal(t, t1.ID, tid)
	require.Equal(t, task.PriorityHigh, client.Task.GetX(ctx, tid).Priority)
}

func Clone(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	f1 := client.File.Create().Set(file.Name, "foo").Set(file.Size, 10).SaveX(ctx)
	f2 := client.File.Create().Set(file.Name, "foo").Set(file.Size, 20).SaveX(ctx)
	base := client.File.Query().Where(file.Name.EQ("foo"))
	require.Equal(t, f1.Size, base.Clone().Where(file.Size.EQ(f1.Size)).OnlyX(ctx).Size)
	require.Equal(t, f2.Size, base.Clone().Where(file.Size.EQ(f2.Size)).OnlyX(ctx).Size)
	// ensure clone emits valid code.
	query := client.Pet.Query().Where(pet.Name.EQ("unknown")).QueryTeam()
	for i := 0; i < 10; i++ {
		_, err := query.Clone().Where(user.Name.EQ("unknown")).First(ctx)
		require.True(t, ent.IsNotFound(err), "should not return syntax error")
	}
}

func Paging(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	for i := 1; i <= 10; i++ {
		client.User.Create().Set(user.Name, fmt.Sprintf("name-%d", i)).Set(user.Age, i).SaveX(ctx)
	}

	require.Equal(10, client.User.Query().CountX(ctx))
	require.Len(client.User.Query().Offset(5).AllX(ctx), 5)
	require.Len(client.User.Query().Offset(6).AllX(ctx), 4)
	projectedAges, projectionError := ent.Values(ctx, client.User.Query().
		Offset(6).
		Limit(2).
		Order(user.Age.Asc()).
		GroupBy(user.Age), user.Age)
	require.NoError(projectionError)
	require.Equal(
		[]int{7, 8},
		projectedAges,
	)
	for i := 0; i < 10; i++ {
		require.Equal(i+1, client.User.Query().Order(user.Age.Asc()).Offset(i).Limit(1).AllX(ctx)[0].Age)
	}
}

func Select(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	t.Log("select one field")
	u := client.User.Create().Set(user.Name, "foo").Set(user.Age, 30).SaveX(ctx)
	row, err := client.User.Query().Where(user.ID.EQ(u.ID)).Select(user.Name).Row(ctx)
	require.NoError(err)
	require.Equal("foo", ent.Get(row, user.Name))
	client.User.Create().Set(user.Name, "bar").Set(user.Age, 30).AddIDs(user.Friends, u.ID).SaveX(ctx)
	t.Log("select one field with ordering")
	names, projectionError := ent.Values(ctx, client.User.
		Query().
		Order(user.Name.Asc()).
		Select(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal([]string{"bar", "foo"}, names)
	names, projectionError = ent.Values(ctx, client.User.
		Query().
		Order(user.Name.Desc()).
		Select(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal([]string{"foo", "bar"}, names)
	client.User.Create().Set(user.Name, "baz").Set(user.Age, 30).SaveX(ctx)
	names, projectionError = ent.Values(ctx, client.User.
		Query().
		Order(user.Name.Asc()).
		Select(user.Name), user.Name)
	require.NoError(projectionError)
	require.Equal([]string{"bar", "baz", "foo"}, names)

	t.Log("select 2 fields")
	var v []struct {
		Age  int    `json:"age"`
		Name string `json:"name"`
	}
	require.NoError(client.User.
		Query().
		Order(user.Name.Asc()).
		Select(user.Age, user.Name).Scan(ctx, &v))
	require.Equal([]int{30, 30, 30}, []int{v[0].Age, v[1].Age, v[2].Age})
	require.Equal([]string{"bar", "baz", "foo"}, []string{v[0].Name, v[1].Name, v[2].Name})

	users := client.User.Query().Where(user.Name.EQ("foo")).WithFriends().AllX(ctx)
	for _, loaded := range users {
		require.NotZero(loaded.ID)
		require.NotZero(loaded.Age)
		for _, friend := range loaded.Edges.Friends {
			require.NotEmpty(friend.Name)
			require.NotZero(friend.ID)
		}
	}
	a8m := client.User.Create().Set(user.Name, "Ariel").Set(user.Nickname, "a8m").Set(user.Age, 30).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.NotEmpty(a8m.Name)
	require.NotEmpty(a8m.Nickname)
	a8m = a8m.Update().Set(user.Age, 32).Select(user.Age).SaveX(ctx)
	require.NotEmpty(a8m.ID)
	require.NotEmpty(a8m.Age)
	require.Empty(a8m.Name)
	require.Empty(a8m.Nickname)

	client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "a"),
		client.Pet.Create().Set(pet.Name, "a"),
	).ExecX(ctx)
	names, err = ent.Values(ctx, client.Pet.Query().Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"a", "a"}, names)
	names, err = ent.Values(ctx, client.Pet.Query().Unique(true).Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"a"}, names)
	client.Pet.Delete().ExecX(ctx)

	pets := client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "a"),
		client.Pet.Create().Set(pet.Name, "b"),
		client.Pet.Create().Set(pet.Name, "c"),
		client.Pet.Create().Set(pet.Name, "b"),
	).SaveX(ctx)
	client.User.Create().Set(user.Name, "foo").Set(user.Age, 20).AddIDs(user.Pets, pets[0].ID, pets[1].ID).SaveX(ctx)
	client.User.Create().Set(user.Name, "bar").Set(user.Age, 20).AddIDs(user.Pets, pets[2].ID, pets[3].ID).SaveX(ctx)
	names, err = ent.Values(ctx, client.Pet.Query().Order(pet.ID.Asc()).Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"a", "b", "c", "b"}, names)
	names, err = ent.Values(ctx, client.Pet.Query().Order(pet.Name.Asc()).Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"a", "b", "b", "c"}, names)
	names, err = ent.Values(ctx, client.Pet.Query().
		Order(func(s *sql.Selector) {
			// Join with user table for ordering by owner-name
			// and pet-name (edge + field ordering).
			t := sql.Table(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldID))
			s.OrderBy(t.C(user.FieldName), s.C(pet.FieldName))
		}).
		Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"b", "c", "a", "b"}, names)

	var ps []*ent.Pet
	require.NoError(client.Pet.Query().Select().Scan(ctx, &ps))
	require.Len(ps, 4, "support scanning nodes manually")

	var lens []int
	require.NoError(client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.Select("LENGTH(name)")
		}).Scan(ctx, &lens))
	require.Equal([]int{1, 1, 1, 1}, lens)

	var dlen []int
	require.NoError(client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.SelectExpr(sql.ExprFunc(func(b *sql.Builder) {
				b.WriteString("LENGTH(name)").WriteOp(sql.OpMul).Arg(2)
			}))
		}).Scan(ctx, &dlen))
	require.Equal([]int{2, 2, 2, 2}, dlen)

	for i := range pets {
		pets[i].Update().Set(pet.Name, pets[i].Name+pets[i].Name).ExecX(ctx)
	}
	var lengths []int
	require.NoError(client.Pet.Query().
		Modify(func(s *sql.Selector) {
			s.Select("SUM(LENGTH(name))")
		}).Scan(ctx, &lengths))
	require.Equal([]int{8}, lengths)

	var (
		p1 []struct {
			ent.Pet
			NameLength int `sql:"length"`
		}
		p2 = client.Pet.Query().Order(pet.ID.Asc()).AllX(ctx)
	)
	require.NoError(client.Pet.Query().
		Order(pet.ID.Asc()).
		Modify(func(s *sql.Selector) {
			s.AppendSelect("LENGTH(name)")
		}).Scan(ctx, &p1))
	for i := range p2 {
		require.Equal(p2[i].ID, p1[i].ID)
		require.Equal(p2[i].Age, p1[i].Age)
		require.Equal(p2[i].Name, p1[i].Name)
		require.Equal(len(p1[i].Name), p1[1].NameLength)
	}

	// Select count.
	names, err = ent.Values(ctx, client.Pet.Query().Order(pet.Name.Asc()).Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Equal([]string{"aa", "bb", "bb", "cc"}, names)
	counts, err := ent.Values(ctx, client.Pet.Query().Select(ent.CountColumn(pet.Name)), ent.CountColumn(pet.Name))
	require.NoError(err)
	require.Equal([]int{4}, counts)
	names, err = ent.Values(ctx, client.Pet.Query().Unique(true).Select(pet.Name), pet.Name)
	require.NoError(err)
	require.Len(names, 3)

	var (
		gs []struct {
			ent.Group
			UsersCount int `sql:"users_count"`
		}
		inf = client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
		hub = client.Group.Create().Set(group.Name, "GitHub").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).AddIDs(group.Users, a8m.ID).SaveX(ctx)
		lab = client.Group.Create().Set(group.Name, "GitLab").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users)...).SaveX(ctx)
	)
	require.NoError(client.Group.Query().
		Order(group.ID.Asc()).
		Modify(func(s *sql.Selector) {
			t := sql.Table(group.UsersTable)
			s.LeftJoin(t).
				On(
					s.C(group.FieldID),
					t.C(group.UsersPrimaryKey[1]),
				).
				// Append the "users_count" column to the selected columns.
				AppendSelect(
					sql.As(sql.Count(t.C(group.UsersPrimaryKey[1])), "users_count"),
				).
				GroupBy(s.C(group.FieldID))
		}).Scan(ctx, &gs))
	require.Len(gs, 2)
	require.Equal(hub.QueryUsers().CountX(ctx), gs[0].UsersCount)
	require.Equal(lab.QueryUsers().CountX(ctx), gs[1].UsersCount)

	// Select Subquery.
	t.Log("select subquery")
	var constants []int
	require.NoError(client.User.Query().Modify(func(s *sql.Selector) {
		subQuery := sql.SelectExpr(sql.Raw("1")).As("s")
		s.Select("*").From(subQuery)
	}).Scan(ctx, &constants))
	require.Equal([]int{1}, constants)

	// Select with join.
	u = client.User.Create().Set(user.Name, "crossworth").Set(user.Age, 28).SaveX(ctx)
	id := client.User.
		Query().
		Where(func(s *sql.Selector) {
			subQuery := sql.Select(user.FieldID).
				From(sql.Table(user.Table)).
				Where(sql.EQ(s.C(user.FieldName), "crossworth"))
			s.Join(subQuery).On(s.C(user.FieldID), subQuery.C(user.FieldID))
		}).
		OnlyIDX(ctx)
	require.Equal(u.ID, id)

	// Update modifiers.
	allUpper := func() bool {
		names, err := ent.Values(ctx, client.User.Query().Select(user.Name), user.Name)
		require.NoError(err)
		for _, name := range names {
			if strings.ToUpper(name) != name {
				return false
			}
		}
		return true
	}
	require.False(allUpper(), "at least one name is not upper-cased")
	// Execute custom update modifier.
	client.User.Update().
		Modify(func(u *sql.UpdateBuilder) {
			u.Set(user.FieldName, sql.Expr(fmt.Sprintf("UPPER(%s)", user.FieldName)))
		}).
		ExecX(ctx)
	require.True(allUpper(), "at names must be upper-cased")

	// Select and scan dynamic values.
	const (
		as1 = "name_length"
		as2 = "another_name"
	)
	length := ent.ExprSelection(ent.Literal(0), as1)
	optionalTime := pet.OptionalTime.As(as2).Nullable()
	projected, err := client.Pet.Query().Select(pet.Name, length, optionalTime).
		Modify(func(selector *sql.Selector) {
			selector.Select(selector.C(pet.FieldName), sql.As("LENGTH(name)", as1), sql.As(selector.C(pet.FieldOptionalTime), as2))
		}).Rows(ctx)
	require.NoError(err)
	for _, row := range projected {
		require.Equal(len(ent.Get(row, pet.Name)), ent.Get(row, length))
		_, present := ent.GetNullable(row, optionalTime)
		require.False(present)
	}

	require.NoError(client.Pet.Update().Set(pet.OptionalTime, time.Now()).Exec(ctx))
	projected, err = client.Pet.Query().Select(optionalTime).Rows(ctx)
	require.NoError(err)
	for _, row := range projected {
		value, present := ent.GetNullable(row, optionalTime)
		require.True(present)
		require.False(value.IsZero())
	}

	// Order by random value should compile a valid query.
	_, err = client.User.Query().Order(sql.OrderByRand()).All(ctx)
	require.NoError(err)
}

func Aggregate(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	a8m := client.User.Create().Set(user.Age, 1).Set(user.Name, "a8m").SaveX(ctx)
	nat := client.User.Create().Set(user.Age, 1).Set(user.Name, "nati").SetEdge(user.Spouse, a8m.ID).SaveX(ctx)
	owners := []*ent.User{a8m, nat}
	for i := 1; i <= 10; i++ {
		client.Pet.Create().Set(pet.Name, fmt.Sprintf("pet%d", i)).Set(pet.Age, float64(i)).SetEdge(pet.Owner, owners[i%2].ID).SaveX(ctx)
	}
	sum := ent.Sum(pet.Age).As("sum")
	s1, err := ent.Values(ctx, client.Pet.Query().Aggregate(sum), sum)
	require.NoError(t, err)
	require.Equal(t, []float64{55}, s1)
	s2, err := ent.Values(ctx, client.Pet.Query().Where(pet.Owner.Has()).Aggregate(sum), sum)
	require.NoError(t, err)
	require.Equal(t, s1, s2)

	// Aggregate traversals.
	for _, test := range []struct {
		query *ent.PetQuery
		want  float64
	}{
		{a8m.QueryPets(), 30},
		{nat.QueryPets(), 25},
		{a8m.QuerySpouse().QueryPets(), 25},
		{nat.QuerySpouse().QueryPets(), 30},
	} {
		values, err := ent.Values(ctx, test.query.Aggregate(sum), sum)
		require.NoError(t, err)
		require.Equal(t, []float64{test.want}, values)
	}

	// Aggregate 2 fields.
	var vs1 []struct{ Sum, Count int }
	require.NoError(t, client.Pet.Query().
		Aggregate(
			ent.Sum(pet.Age).As("sum"),
			ent.Count(),
		).Scan(ctx, &vs1))
	require.Len(t, vs1, 1)
	require.Equal(t, 55, vs1[0].Sum)
	require.Equal(t, 10, vs1[0].Count)

	// Aggregate 4 fields.
	var vs2 []struct {
		Sum, Min, Max, Count int
		Avg                  float64
	}
	require.NoError(t, client.Pet.Query().
		Aggregate(
			ent.Sum(pet.Age).As("sum"),
			ent.Min(pet.Age).As("min"),
			ent.Max(pet.Age).As("max"),
			ent.Avg(pet.Age).As("avg"),
			ent.Count(),
		).Scan(ctx, &vs2))
	require.Len(t, vs2, 1)
	require.Equal(t, 55, vs2[0].Sum)
	require.Equal(t, 1, vs2[0].Min)
	require.Equal(t, 10, vs2[0].Max)
	require.Equal(t, 10, vs2[0].Count)
	require.Equal(t, 5.5, vs2[0].Avg)
}

func ExecQuery(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	rows, err := client.Driver().Query(ctx, "SELECT 1", nil)
	require.NoError(err)
	require.True(rows.Next())
	require.NoError(rows.Close())
	tx, err := client.Tx(ctx)
	require.NoError(err)
	tx.Task.Create().ExecX(ctx)
	require.Equal(1, tx.Task.Query().CountX(ctx))
	rows, err = tx.Client().Driver().Query(ctx, "SELECT COUNT(*) FROM "+enttask.Table, nil)
	require.NoError(err)
	count, err := sql.ScanInt(rows)
	require.NoError(err)
	require.NoError(rows.Close())
	require.Equal(1, count)
	require.NoError(tx.Commit())
}

func NillableRequired(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	client.Task.Create().Set(enttask.Name, "Name").ExecX(ctx)
	tk := client.Task.Query().OnlyX(ctx)
	require.Empty(tk.Name, "Name is not selected by default")
	require.NotNil(tk.CreatedAt, "field value should be populated by default by the database")
	require.False(reflect.ValueOf(tk.Update()).MethodByName("SetNillableCreatedAt").IsValid(), "immutable-nillable should not have SetNillable setter on update")
	row, err := client.Task.Query().Select(enttask.ID, enttask.Priority, enttask.Name).Row(ctx)
	require.NoError(err)
	require.Equal("Name", ent.Get(row, enttask.Name), "Name should be populated when selected manually")
}

func Predicate(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	f1 := client.File.Create().Set(file.Name, "1").Set(file.Size, 10).SaveX(ctx)
	f2 := client.File.Create().Set(file.Name, "2").Set(file.Size, 20).SaveX(ctx)
	f3 := client.File.Create().Set(file.Name, "3").Set(file.Size, 30).SaveX(ctx)
	f4 := client.File.Create().Set(file.Name, "4").Set(file.Size, 40).SaveX(ctx)
	files := client.File.Query().
		Where(
			file.Or(
				file.Name.EQ(f1.Name),
				file.And(file.Name.EQ(f2.Name), file.Size.EQ(f2.Size)),
			),
		).
		Order(file.Name.Asc()).
		AllX(ctx)
	require.Equal(f1.Name, files[0].Name)
	require.Equal(f2.Name, files[1].Name)

	match := client.File.Query().
		Where(file.Or(file.Name.EQ(f1.Name), file.Name.EQ(f2.Name))).
		Where(file.Size.EQ(f1.Size)).
		OnlyX(ctx)
	require.Equal(f1.Name, match.Name)

	match = client.File.Query().
		Where(file.Size.EQ(f2.Size)).
		Where(file.Or(file.Name.EQ(f1.Name), file.Name.EQ(f2.Name))).
		OnlyX(ctx)
	require.Equal(f2.Name, match.Name)

	files = client.File.Query().
		Where(file.Or(file.Size.EQ(f3.Size), file.Size.EQ(f4.Size))).
		Where(file.Or(file.Name.EQ(f3.Name), file.Name.EQ(f4.Name))).
		Where(file.Not(file.Or(file.Name.EQ(f1.Name), file.Size.EQ(f1.Size)))).
		Order(file.Name.Asc()).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	files = client.File.Query().
		Where(
			file.Or(
				file.Name.EQ(f4.Name),
				file.And(file.Name.EQ(f3.Name), file.Size.EQ(f3.Size)),
			),
		).
		Order(file.Name.Asc()).
		AllX(ctx)
	require.Equal(f3.Name, files[0].Name)
	require.Equal(f4.Name, files[1].Name)

	require.Zero(client.File.Query().Where(file.User.NotNull()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.User.IsNull()).CountX(ctx))
	require.Zero(client.File.Query().Where(file.Group.NotNull()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.Group.IsNull()).CountX(ctx))

	f1 = f1.Update().Set(file.User, "a8m").SaveX(ctx)
	require.NotNil(f1.User)
	require.Equal("a8m", *f1.User)
	require.Equal(3, client.File.Query().Where(file.User.IsNull()).CountX(ctx))
	require.Equal(f1.Name, client.File.Query().Where(file.User.NotNull()).OnlyX(ctx).Name)
	f5 := client.File.Create().Set(file.Name, "5").Set(file.Size, 40).Set(file.User, "mashraki").SaveX(ctx)
	require.NotNil(f5.User)
	require.Equal("mashraki", *f5.User)
	require.Equal(3, client.File.Query().Where(file.User.IsNull()).CountX(ctx))
	require.Equal(2, client.File.Query().Where(file.User.NotNull()).CountX(ctx))

	require.Equal(5, client.File.Query().Where(file.Group.IsNull()).CountX(ctx))
	f4 = f4.Update().Set(file.Group, "fbc").SaveX(ctx)
	require.Equal(1, client.File.Query().Where(file.Group.NotNull()).CountX(ctx))
	require.Equal(4, client.File.Query().Where(file.Group.IsNull()).CountX(ctx))
	require.Equal(
		5,
		client.File.Query().
			Where(
				file.Or(
					file.Group.IsNull(),
					file.And(
						file.Group.NotNull(),
						file.Name.EQ(f4.Name),
					),
				),
			).
			CountX(ctx),
	)

	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	hub := client.Group.Create().Set(group.Name, "GitHub").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).SaveX(ctx)
	lab := client.Group.Create().Set(group.Name, "GitLab").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).Set(group.Active, false).SaveX(ctx)
	require.Equal(hub.ID, client.Group.Query().Where(group.Active.EQ(true)).OnlyIDX(ctx))
	require.Equal(lab.ID, client.Group.Query().Where(group.Active.EQ(false)).OnlyIDX(ctx))
	require.Equal(hub.ID, client.Group.Query().Where(group.Active.NEQ(false)).OnlyIDX(ctx))
	require.Equal(lab.ID, client.Group.Query().Where(group.Active.NEQ(true)).OnlyIDX(ctx))

	client.User.CreateBulk(
		client.User.Create().Set(user.Age, 1).Set(user.Name, "Ariel").Set(user.Nickname, "A"),
		client.User.Create().Set(user.Age, 1).Set(user.Name, "Ariel").Set(user.Nickname, "A%"),
	).ExecX(ctx)
	a1 := client.User.Query().Where(sql.FieldsHasPrefix(user.FieldName, user.FieldNickname)).OnlyX(ctx)
	require.Equal("A", a1.Nickname)
	a2 := client.User.Query().Where(user.Not(sql.FieldsHasPrefix(user.FieldName, user.FieldNickname))).OnlyX(ctx)
	require.Equal("A%", a2.Nickname)
}

func AddValues(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	t.Log("add values to fields")
	cmt := client.Comment.Create().Set(comment.UniqueInt, 1).Set(comment.UniqueFloat, 1).SaveX(ctx)
	cmt = cmt.Update().Add(comment.UniqueInt, 10).SaveX(ctx)
	require.Equal(11, cmt.UniqueInt)
	require.Equal(11, client.Comment.Query().OnlyX(ctx).UniqueInt, "should be updated in the database")
	t.Log("add values to null fields")
	cmt = cmt.Update().Add(comment.NillableInt, 10).SaveX(ctx)
	require.Equal(10, *cmt.NillableInt)

	cmt1 := client.Comment.Create().Set(comment.UniqueInt, 1).Set(comment.UniqueFloat, 10).SaveX(ctx)
	err := cmt1.Update().Add(comment.UniqueInt, 10).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	cmt1 = cmt1.Update().Add(comment.UniqueInt, 20).Add(comment.NillableInt, 20).SaveX(ctx)
	require.Equal(21, cmt1.UniqueInt)
	require.Equal(20, *cmt1.NillableInt)

	cmt1 = cmt1.Update().Add(comment.UniqueInt, 10).Add(comment.UniqueInt, -1).SaveX(ctx)
	require.Equal(30, cmt1.UniqueInt)
	require.Equal(30, client.Comment.GetX(ctx, cmt1.ID).UniqueInt)
}

func Delete(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	nd := client.Node.Create().Set(node.Value, 1e3).SaveX(ctx)
	err := client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.NoError(err)
	err = client.Node.DeleteOneID(nd.ID).Exec(ctx)
	require.True(ent.IsNotFound(err))

	for i := 0; i < 5; i++ {
		client.Node.Create().Set(node.Value, i).ExecX(ctx)
	}
	affected, err := client.Node.Delete().Where(node.Value.GT(2)).Exec(ctx)
	require.NoError(err)
	require.Equal(2, affected)

	affected, err = client.Node.Delete().Exec(ctx)
	require.NoError(err)
	require.Equal(3, affected)

	info := client.GroupInfo.Create().Set(groupinfo.Desc, "group info").SaveX(ctx)
	hub := client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Name, "GitHub").Set(group.Expire, time.Now().Add(time.Hour)).SaveX(ctx)
	err = client.GroupInfo.DeleteOne(info).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	// Group.DeleteOneID(id).Where(...), is identical to Group.Delete().Where(group.ID.EQ(id), ...),
	// but, in case the OpDelete is not an allowed operation, the DeleteOne can be used with Where.
	n, err := client.Group.Delete().
		Where(
			group.ID.EQ(hub.ID),
			group.Expire.LT(time.Now()), // Expired.
		).Exec(ctx)
	require.Zero(n)
	require.NoError(err)

	err = client.Group.DeleteOne(hub).
		Where(group.Expire.LT(time.Now())).
		Exec(ctx)
	require.True(ent.IsNotFound(err))
	hub.Update().Set(group.Expire, time.Now().Add(-time.Hour)).ExecX(ctx)
	client.Group.DeleteOne(hub).
		Where(group.Expire.LT(time.Now())).
		ExecX(ctx)

	// The behavior described above it also applied to UpdateOne.
	hub = client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Name, "GitHub").Set(group.Expire, time.Now().Add(time.Hour)).SaveX(ctx)
	err = hub.Update().
		Set(group.Active, false).
		Set(group.Expire, time.Time{}).
		Where(group.Expire.LT(time.Now())). // Expired.
		Exec(ctx)
	require.True(ent.IsNotFound(err))
}

func Relation(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	t.Log("querying group info")
	info, err := client.GroupInfo.
		Query().
		First(ctx)
	require.Nil(info)
	require.True(ent.IsNotFound(err))

	t.Log("creating group info")
	info = client.GroupInfo.
		Create().
		Set(groupinfo.Desc, "group info").
		SaveX(ctx)
	t.Logf("group info created: %v", info)

	t.Log("creating group")
	grp := client.Group.
		Create().
		SetEdge(group.Info, info.ID).
		Set(group.Name, "Github").
		Set(group.Expire, time.Now().Add(time.Hour)).
		SaveX(ctx)
	require.NotZero(grp.ID)
	require.Equal(grp.MaxUsers, 10)
	require.Equal(grp.Name, "Github")
	t.Logf("group created: %v", grp)

	t.Log("creating user")
	usr := client.User.
		Create().
		Set(user.Age, 20).
		Set(user.Name, "a8m").
		AddIDs(user.Groups, grp.ID).
		SaveX(ctx)
	require.NotZero(usr.ID)
	require.Equal(usr.Age, 20)
	require.Equal(usr.Name, "a8m")
	require.Equal(usr.Last, "unknown")
	t.Logf("user created: %v", usr)

	t.Log("querying assoc edges")
	groups := usr.QueryGroups().IDsX(ctx)
	require.NotEmpty(groups)
	require.Equal(grp.ID, groups[0])
	t.Log("querying inverse edge")
	users := grp.QueryUsers().IDsX(ctx)
	require.NotEmpty(users)
	require.Equal(usr.ID, users[0])

	t.Log("remove group edge")
	client.User.UpdateOne(usr).RemoveIDs(user.Groups, grp.ID).ExecX(ctx)
	require.Empty(grp.QueryUsers().AllX(ctx))
	require.Empty(usr.QueryGroups().AllX(ctx))
	t.Logf("add group edge")
	client.User.UpdateOne(usr).AddIDs(user.Groups, grp.ID).ExecX(ctx)
	require.NotEmpty(grp.QueryUsers().AllX(ctx))
	require.NotEmpty(usr.QueryGroups().AllX(ctx))
	t.Log("remove users inverse edge")
	client.Group.UpdateOne(grp).RemoveIDs(group.Users, usr.ID).ExecX(ctx)
	require.Empty(grp.QueryUsers().AllX(ctx))
	require.Empty(usr.QueryGroups().AllX(ctx))
	t.Logf("add group inverse edge")
	client.Group.UpdateOne(grp).AddIDs(group.Users, usr.ID).ExecX(ctx)
	require.NotEmpty(grp.QueryUsers().AllX(ctx))
	require.NotEmpty(usr.QueryGroups().AllX(ctx))

	t.Log("count vertices")
	require.Equal(1, client.User.Query().CountX(ctx))
	require.Equal(1, client.Group.Query().CountX(ctx))

	t.Log("get only vertices")
	require.NotNil(client.User.Query().OnlyX(ctx))
	require.NotNil(client.Group.Query().OnlyX(ctx))

	t.Log("get only ids")
	require.NotEmpty(client.User.Query().OnlyIDX(ctx))
	require.NotEmpty(client.Group.Query().OnlyIDX(ctx))

	t.Log("query spouse edge")
	require.Zero(client.User.Query().Where(user.Spouse.Has()).CountX(ctx))
	neta := client.User.Create().Set(user.Name, "neta").Set(user.Age, 18).SetEdge(user.Spouse, usr.ID).SaveX(ctx)
	require.Equal(2, client.User.Query().Where(user.Spouse.Has()).CountX(ctx))

	t.Log("check for singular error")
	_, err = client.User.Query().Only(ctx)
	require.True(ent.IsNotSingular(err))

	t.Log("query parent/children edges")
	require.False(usr.QueryParent().ExistX(ctx))
	require.Empty(usr.QueryChildren().AllX(ctx))
	child := client.User.Create().Set(user.Name, "pedro").Set(user.Age, 7).SetEdge(user.Parent, usr.ID).SaveX(ctx)
	require.Equal(usr.Name, child.QueryParent().OnlyX(ctx).Name)
	require.Equal(child.Name, usr.QueryChildren().OnlyX(ctx).Name)
	require.False(usr.QueryParent().ExistX(ctx))

	t.Log("clear parent edge")
	brat := client.User.Create().Set(user.Name, "brat").Set(user.Age, 19).SetEdge(user.Parent, usr.ID).SaveX(ctx)
	require.Equal(2, usr.QueryChildren().CountX(ctx))
	brat = client.User.UpdateOne(brat).ClearEdge(user.Parent).SaveX(ctx)
	err = client.User.UpdateOne(brat).ClearEdge(user.Parent).Exec(ctx)
	require.NoError(err)
	require.False(brat.QueryParent().ExistX(ctx))
	require.Equal(1, usr.QueryChildren().CountX(ctx))

	t.Log("delete child clears edge")
	brat = client.User.UpdateOne(brat).SetEdge(user.Parent, usr.ID).SaveX(ctx)
	require.Equal(2, usr.QueryChildren().CountX(ctx))
	client.User.DeleteOne(brat).ExecX(ctx)
	require.Equal(1, usr.QueryChildren().CountX(ctx))

	client.Group.UpdateOne(grp).AddIDs(group.Blocked, neta.ID).ExecX(ctx)
	blocked := usr.QueryGroups().OnlyX(ctx).QueryBlocked().OnlyX(ctx)
	t.Log("blocked:", blocked)

	t.Log("query users with or condition")
	require.Len(client.User.Query().Where(user.Or(user.Name.EQ("a8m"), user.Name.EQ("neta"))).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Or(user.Name.EQ("a8m"), user.Name.EQ("noam"))).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.Or(user.Name.EQ("alex"), user.Name.EQ("noam"))).AllX(ctx), 0)

	t.Log("query using the in predicate")
	require.Len(client.User.Query().Where(user.Name.In("a8m", "neta")).AllX(ctx), 2)
	require.Len(client.User.Query().Where(user.Name.In("a8m", "alex")).AllX(ctx), 1)
	require.Len(client.User.Query().Where(user.ID.In(neta.ID)).AllX(ctx), 1)

	t.Log("query existence")
	require.True(client.User.Query().Where(user.Name.EQ("a8m")).Exist(ctx))
	require.False(client.User.Query().Where(user.Name.EQ("alex")).Exist(ctx))

	t.Log("query using get")
	require.Equal(usr.Name, client.User.GetX(ctx, usr.ID).Name)
	uid, err := client.User.Query().Where(user.ID.EQ(usr.ID), user.Not(user.Name.EQ(usr.Name))).Only(ctx)
	require.Error(err)
	require.Nil(uid)

	t.Log("test validators")
	err = client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Type, "a").Set(group.Name, "Gituhb").Set(group.Expire, time.Now().Add(time.Hour)).Exec(ctx)
	require.Error(err, "type validator failed")
	err = client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Type, "pass").Set(group.Name, "failed").Set(group.Expire, time.Now().Add(time.Hour)).Exec(ctx)
	require.Error(err, "name validator failed")
	var checkerr schema.CheckError
	require.True(errors.As(err, &checkerr))
	require.EqualError(err, `ent: validator failed for field "Group.name": last name must begin with uppercase`)
	require.EqualError(checkerr, "last name must begin with uppercase")
	err = client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Type, "pass").Set(group.Name, "Github20").Set(group.Expire, time.Now().Add(time.Hour)).Exec(ctx)
	require.Error(err, "name validator failed")
	err = client.Group.Create().SetEdge(group.Info, info.ID).Set(group.Type, "pass").Set(group.Name, "Github").Set(group.MaxUsers, -1).Set(group.Expire, time.Now().Add(time.Hour)).Exec(ctx)
	require.Error(err, "max_users validator failed")
	err = client.Group.Update().Set(group.MaxUsers, -10).Exec(ctx)
	require.Error(err, "max_users validator failed")
	err = client.Group.UpdateOne(grp).Set(group.MaxUsers, -10).Exec(ctx)
	require.Error(err, "max_users validator failed")

	t.Log("query using edge-with predicate")
	require.Len(usr.QueryGroups().Where(group.Info.HasWith(groupinfo.Desc.EQ("group info"))).AllX(ctx), 1)
	require.Empty(usr.QueryGroups().Where(group.Info.HasWith(groupinfo.Desc.EQ("missing info"))).AllX(ctx))
	t.Log("query using edge-with predicate on inverse edges")
	require.Len(client.Group.Query().Where(group.Name.EQ("Github"), group.Users.HasWith(user.Name.EQ("a8m"))).AllX(ctx), 1)
	require.Empty(client.Group.Query().Where(group.Name.EQ("Github"), group.Users.HasWith(user.Name.EQ("alex"))).AllX(ctx))
	t.Logf("query path using edge-with predicate")
	require.Len(client.GroupInfo.Query().Where(groupinfo.Groups.HasWith(group.Users.HasWith(user.Name.EQ("a8m")))).AllX(ctx), 1)
	require.Empty(client.GroupInfo.Query().Where(groupinfo.Groups.HasWith(group.Users.HasWith(user.Name.EQ("alex")))).AllX(ctx))
	require.Len(client.GroupInfo.Query().Where(groupinfo.Or(groupinfo.Desc.EQ("group info"), groupinfo.Groups.HasWith(group.Users.HasWith(user.Name.EQ("alex"))))).AllX(ctx), 1)

	t.Log("query with ordering")
	u1 := client.User.Query().Order(user.Name.Asc()).FirstIDX(ctx)
	u2 := client.User.Query().Order(user.Name.Desc()).FirstIDX(ctx)
	require.NotEqual(u1, u2)
	u1 = client.User.Query().Order(user.Last.Asc(), user.Age.Asc()).FirstIDX(ctx)
	u2 = client.User.Query().Order(user.Last.Asc(), user.Age.Desc()).FirstIDX(ctx)
	require.NotEqual(u1, u2)
	u1 = client.User.Query().Order(user.Name.Asc(), user.Age.Asc()).FirstIDX(ctx)
	u2 = client.User.Query().Order(user.Name.Asc(), user.Age.Asc()).FirstIDX(ctx)
	require.Equal(u1, u2)

	t.Log("query path")
	require.Len(client.Group.Query().QueryUsers().AllX(ctx), 1)
	require.Empty(client.Group.Query().Where(group.Name.EQ("boring")).QueryUsers().AllX(ctx))
	require.Equal(neta.Name, usr.QueryGroups().Where(group.Name.EQ("Github")).QueryUsers().QuerySpouse().OnlyX(ctx).Name)
	require.Empty(client.GroupInfo.Query().Where(groupinfo.Desc.EQ("group info")).QueryGroups().Where(group.Name.EQ("boring")).AllX(ctx))
	require.Equal(child.Name, client.GroupInfo.Query().Where(groupinfo.Desc.EQ("group info")).QueryGroups().Where(group.Name.EQ("Github")).QueryUsers().QueryChildren().FirstX(ctx).Name)
	neta.Update().AddIDs(user.Groups, grp.ID).ExecX(ctx)
	require.Equal(grp.ID, client.User.Query().QueryGroups().OnlyIDX(ctx))

	t.Log("query using string predicate")
	require.Len(client.User.Query().Where(user.Name.In("a8m", "neta", "pedro")).AllX(ctx), 3)
	require.Empty(client.User.Query().Where(user.Name.NotIn("a8m", "neta", "pedro")).AllX(ctx))
	require.Empty(client.User.Query().Where(user.Name.In("alex", "rocket")).AllX(ctx))
	require.NotNil(client.User.Query().Where(user.Parent.HasWith(user.Name.In("a8m", "neta"))).OnlyX(ctx))
	require.Len(client.User.Query().Where(user.Name.Contains("a8")).AllX(ctx), 1)
	require.Equal(1, client.User.Query().Where(user.Name.HasPrefix("a8")).CountX(ctx))
	require.Zero(client.User.Query().Where(user.Name.HasPrefix("%a8%")).CountX(ctx))
	require.Equal(2, client.User.Query().Where(user.Or(user.Name.HasPrefix("a8"), user.Name.HasSuffix("eta"))).CountX(ctx))

	t.Log("group-by one field")
	names, err := ent.Values(ctx, client.User.Query().GroupBy(user.Name), user.Name)
	require.NoError(err)
	sort.Strings(names)
	require.Equal([]string{"a8m", "neta", "pedro"}, names)
	ages, err := ent.Values(ctx, client.User.Query().GroupBy(user.Age), user.Age)
	require.NoError(err)
	require.Len(ages, 3)
	ages, err = ent.Values(ctx, client.User.Query().Where(user.Name.EQ("alexsn")).GroupBy(user.Age), user.Age)
	require.NoError(err)
	require.Empty(ages)

	t.Log("group-by two fields with aggregation")
	client.User.Create().Set(user.Name, usr.Name).Set(user.Age, usr.Age).ExecX(ctx)
	client.User.Create().Set(user.Name, neta.Name).Set(user.Age, neta.Age).ExecX(ctx)
	child2 := client.User.Create().Set(user.Name, child.Name).Set(user.Age, child.Age+1).SaveX(ctx)
	var v []struct {
		Name  string `json:"name"`
		Age   int    `json:"age"`
		Sum   int    `json:"sum"`
		Count int    `json:"count"`
	}
	require.NoError(client.User.Query().
		GroupBy(user.Name, user.Age).
		Aggregate(ent.Count(), ent.Sum(user.Age).As("sum")).Scan(ctx, &v))
	require.Len(v, 4)
	sort.Slice(v, func(i, j int) bool {
		if v[i].Name != v[j].Name {
			return v[i].Name < v[j].Name
		}
		return v[i].Age < v[j].Age
	})
	for i, usr := range []*ent.User{usr, neta} {
		require.Equal(usr.Name, v[i].Name)
		require.Equal(usr.Age, v[i].Age)
		require.Equal(usr.Age*2, v[i].Sum)
		require.Equal(2, v[i].Count, "should have 2 vertices")
	}
	v = v[2:]
	for i, usr := range []*ent.User{child, child2} {
		require.Equal(usr.Name, v[i].Name)
		require.Equal(usr.Age, v[i].Age)
		require.Equal(usr.Age, v[i].Sum)
		require.Equal(1, v[i].Count)
	}

	t.Log("group by with .as modulator")
	var v2 []struct {
		Name  string `json:"name"`
		Total int    `json:"total"`
	}
	require.NoError(client.User.Query().GroupBy(user.Name).Aggregate(ent.Count().As("total")).Scan(ctx, &v2))
	require.Len(v2, 3)
	for i := range v2 {
		require.Equal(2, v2[i].Total)
	}

	t.Log("group by a relation")
	foo := client.User.Create().Set(user.Name, "foo").Set(user.Age, 10).AddIDs(user.Pets, client.Pet.Create().Set(pet.Name, "a").Set(pet.Age, 10).SaveX(ctx).ID,
		client.Pet.Create().Set(pet.Name, "b").Set(pet.Age, 7).SaveX(ctx).ID,
	).SaveX(ctx)
	bar := client.User.Create().Set(user.Name, "bar").Set(user.Age, 10).AddIDs(user.Pets, client.Pet.Create().Set(pet.Name, "c").Set(pet.Age, 14).SaveX(ctx).ID,
		client.Pet.Create().Set(pet.Name, "d").Set(pet.Age, 1).SaveX(ctx).ID,
	).SaveX(ctx)

	var v3 []struct {
		ID      int
		Name    string
		Average float64
	}
	require.NoError(client.User.Query().
		Where(user.ID.In(foo.ID, bar.ID)).
		Order(user.ID.Asc()).
		JoinAs(pet.Table, pet.Table, func(selector *sql.Selector) {
			selector.Where(sql.ColumnsEQ(selector.C(user.FieldID), sql.Table(pet.Table).C(pet.OwnerColumn)))
		}).
		GroupBy(user.ID, user.Name).
		Aggregate(ent.Avg(pet.Age).As("average")).Scan(ctx, &v3))
	require.Len(v3, 2)
	require.Equal(foo.ID, v3[0].ID)
	require.Equal(foo.Name, v3[0].Name)
	require.Equal(8.5, v3[0].Average)
	require.Equal(bar.ID, v3[1].ID)
	require.Equal(bar.Name, v3[1].Name)
	require.Equal(7.5, v3[1].Average)

	var v4 []struct {
		ID    int    `sql:"id"`
		Name  string `sql:"name"`
		Owner string `sql:"owner"`
	}
	require.NoError(client.Pet.Query().
		Where(func(s *sql.Selector) {
			t := sql.Table(user.Table).As(user.Table)
			s.Join(t).On(s.C(pet.OwnerColumn), t.C(user.FieldID)) // owner_id = id for edge fields.
		}).
		Order(pet.ID.Asc()).
		Select(pet.ID, pet.Name, user.Name.As("owner")).Scan(ctx, &v4))
	require.Equal(v4[0].Name, "a")
	require.Equal(v4[0].Owner, "foo")
	require.Equal(v4[1].Name, "b")
	require.Equal(v4[1].Owner, "foo")
	require.Equal(v4[2].Name, "c")
	require.Equal(v4[2].Owner, "bar")
	require.Equal(v4[3].Name, "d")
	require.Equal(v4[3].Owner, "bar")
}

func ClearFields(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	img := client.File.Create().Set(file.Name, "foo").Set(file.Size, 100).Set(file.User, "a8m").Set(file.Group, "Github").SaveX(ctx)

	t.Log("clear one field")
	img = img.Update().Clear(file.User).SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Equal(t, "Github", img.Group)

	t.Log("clear many fields")
	img = img.Update().Clear(file.User).Clear(file.Group).SaveX(ctx)
	require.Nil(t, img.User)
	img = client.File.Query().OnlyX(ctx)
	require.Nil(t, img.User)
	require.Empty(t, img.Group)

	t.Log("revert previous set")
	img = img.Update().Set(file.User, "a8m").Clear(file.User).SaveX(ctx)
	require.Nil(t, img.User)
}

func ClearEdges(t *testing.T, client *ent.Client) {
	ctx := context.Background()

	t.Log("clear o2m edges")
	ft := client.FileType.Create().Set(filetype.Name, "photo").SaveX(ctx)
	client.File.CreateBulk(
		client.File.Create().Set(file.Name, "A").Set(file.Size, 10).SetEdge(file.Type, ft.ID),
		client.File.Create().Set(file.Name, "B").Set(file.Size, 20).SetEdge(file.Type, ft.ID),
	).ExecX(ctx)
	require.NotZero(t, ft.QueryFiles().CountX(ctx))
	ft = ft.Update().ClearEdge(filetype.Files).SaveX(ctx)
	require.Zero(t, ft.QueryFiles().CountX(ctx))

	t.Log("clear m2m edges")
	a8m := client.User.Create().Set(user.Name, "a8m").Set(user.Age, 30).SaveX(ctx)
	nat := client.User.Create().Set(user.Name, "nati").Set(user.Age, 28).SaveX(ctx)
	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	hub := client.Group.Create().Set(group.Name, "GitHub").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).AddIDs(group.Users, a8m.ID, nat.ID).SaveX(ctx)
	lab := client.Group.Create().Set(group.Name, "GitLab").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).AddIDs(group.Users, a8m.ID, nat.ID).SaveX(ctx)
	require.Equal(t, 2, a8m.QueryGroups().CountX(ctx))
	a8m.Update().ClearEdge(user.Groups).ExecX(ctx)
	require.Zero(t, a8m.QueryGroups().CountX(ctx))
	err := client.Group.Update().AddIDs(group.Users, a8m.ID).Exec(ctx)
	require.NoError(t, err, "return the user-edge back to groups")
	require.Equal(t, 2, a8m.QueryGroups().CountX(ctx))

	t.Log("clear m2m inverse-edges")
	require.Equal(t, 2, hub.QueryUsers().CountX(ctx))
	hub = hub.Update().ClearEdge(group.Users).SaveX(ctx)
	require.Zero(t, hub.QueryUsers().CountX(ctx))
	require.Equal(t, 2, lab.QueryUsers().CountX(ctx))
	client.Group.Update().ClearEdge(group.Users).ExecX(ctx)
	require.Zero(t, lab.QueryUsers().CountX(ctx))
	require.Zero(t, a8m.QueryGroups().CountX(ctx))
	require.Zero(t, nat.QueryGroups().CountX(ctx))

	t.Log("clear m2m bidi-edges")
	friends := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "f1").Set(user.Age, 30).AddIDs(user.Friends, a8m.ID, nat.ID),
		client.User.Create().Set(user.Name, "f2").Set(user.Age, 30).AddIDs(user.Friends, a8m.ID, nat.ID),
		client.User.Create().Set(user.Name, "f3").Set(user.Age, 30).AddIDs(user.Friends, a8m.ID, nat.ID),
	).SaveX(ctx)
	for i := range friends {
		require.Equal(t, 2, friends[i].QueryFriends().CountX(ctx))
	}
	require.Equal(t, 3, a8m.QueryFriends().CountX(ctx))
	require.Equal(t, 3, nat.QueryFriends().CountX(ctx))
	nat = nat.Update().ClearEdge(user.Friends).SaveX(ctx)
	require.Zero(t, nat.QueryFriends().CountX(ctx))
	require.Equal(t, 3, a8m.QueryFriends().CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, friends[i].QueryFriends().CountX(ctx))
	}
	client.User.Update().ClearEdge(user.Friends).ExecX(ctx)
	require.Zero(t, client.User.Query().Where(user.Friends.Has()).CountX(ctx))

	t.Log("clear m2m inverse-bidi-edges")
	a8m = a8m.Update().AddIDs(user.Following, entityIDs(friends)...).SaveX(ctx)
	require.Equal(t, 3, a8m.QueryFollowing().CountX(ctx))
	require.Zero(t, a8m.QueryFollowers().CountX(ctx))
	nat = nat.Update().AddIDs(user.Followers, entityIDs(friends)...).SaveX(ctx)
	require.Zero(t, nat.QueryFollowing().CountX(ctx))
	require.Equal(t, 3, nat.QueryFollowers().CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, friends[i].QueryFollowers().CountX(ctx))
		require.Equal(t, 1, friends[i].QueryFollowing().CountX(ctx))
	}
	nat.Update().ClearEdge(user.Following).ExecX(ctx)
	require.Equal(t, 3, nat.QueryFollowers().CountX(ctx), "expect no effect on followers")
	nat.Update().ClearEdge(user.Followers).ExecX(ctx)
	require.Zero(t, nat.QueryFollowers().CountX(ctx))
	for i := range friends {
		require.Equal(t, 1, friends[i].QueryFollowers().CountX(ctx), "expect no effect to followers")
		require.Zero(t, friends[i].QueryFollowing().CountX(ctx))
	}
	a8m.Update().ClearEdge(user.Followers).ExecX(ctx)
	require.Equal(t, 3, a8m.QueryFollowing().CountX(ctx), "expect no effect on following")
	a8m.Update().ClearEdge(user.Following).ExecX(ctx)
	require.Zero(t, a8m.QueryFollowing().CountX(ctx))
	for i := range friends {
		require.Zero(t, friends[i].QueryFollowers().CountX(ctx))
		require.Zero(t, friends[i].QueryFollowing().CountX(ctx))
	}

	t.Log("remove/clear and add edges")
	a8m = a8m.Update().AddIDs(user.Following, friends[0].ID, friends[1].ID).SaveX(ctx)
	require.Equal(t, []int{friends[0].ID, friends[1].ID}, a8m.QueryFollowing().Order(user.ID.Asc()).IDsX(ctx))
	a8m = a8m.Update().RemoveIDs(user.Following, friends[0].ID, friends[1].ID).AddIDs(user.Following, friends[2].ID).SaveX(ctx)
	require.Equal(t, friends[2].ID, a8m.QueryFollowing().OnlyIDX(ctx))
	a8m = a8m.Update().ClearEdge(user.Following).AddIDs(user.Following, friends[0].ID).SaveX(ctx)
	require.Equal(t, friends[0].ID, a8m.QueryFollowing().OnlyIDX(ctx))
}

func UniqueConstraint(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()

	t.Log("unique constraint violation on 1 field")
	foo := client.User.Create().Set(user.Age, 1).Set(user.Name, "foo").Set(user.Nickname, "baz").SaveX(ctx)
	_, err := client.User.Create().Set(user.Age, 1).Set(user.Name, "bar").Set(user.Nickname, "baz").Save(ctx)
	require.True(ent.IsConstraintError(err))
	bar := client.User.Create().Set(user.Age, 1).Set(user.Name, "bar").Set(user.Nickname, "bar").Set(user.Phone, "1").SaveX(ctx)

	t.Log("unique constraint violation on 2 fields")
	err = client.User.Create().Set(user.Age, 1).Set(user.Name, "baz").Set(user.Nickname, "bar").Set(user.Phone, "1").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.Create().Set(user.Age, 1).Set(user.Name, "baz").Set(user.Nickname, "qux").Set(user.Phone, "1").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.Create().Set(user.Age, 1).Set(user.Name, "baz").Set(user.Nickname, "bar").Set(user.Phone, "2").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.Create().Set(user.Age, 1).Set(user.Name, "baz").Set(user.Nickname, "qux").Set(user.Phone, "2").ExecX(ctx)
	err = client.User.UpdateOne(foo).Set(user.Nickname, "bar").Set(user.Phone, "1").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(foo).Set(user.Nickname, "bar").Set(user.Phone, "2").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.CreateBulk(
		client.User.Create().Set(user.Age, 1).Set(user.Name, "foo").Set(user.Nickname, "baz"),
		client.User.Create().Set(user.Age, 1).Set(user.Name, "foo").Set(user.Nickname, "baz"),
	).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on creation")
	dan := client.User.Create().Set(user.Age, 1).Set(user.Name, "dan").Set(user.Nickname, "dan").SetEdge(user.Spouse, foo.ID).SaveX(ctx)
	require.Equal(dan.Name, foo.QuerySpouse().OnlyX(ctx).Name)
	err = client.User.Create().Set(user.Age, 1).Set(user.Name, "b").SetEdge(user.Spouse, foo.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2m/m2o unique constraint on creation")
	c1 := client.User.Create().Set(user.Age, 1).Set(user.Name, "c1").Set(user.Nickname, "c1").SetEdge(user.Parent, foo.ID).SaveX(ctx)
	c2 := client.User.Create().Set(user.Age, 1).Set(user.Name, "c2").Set(user.Nickname, "c2").SetEdge(user.Parent, foo.ID).SaveX(ctx)
	err = client.User.Create().Set(user.Age, 10).Set(user.Name, "z").Set(user.Nickname, "z").AddIDs(user.Children, c1.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err), "c1 already has a parent")
	err = client.User.Create().Set(user.Age, 10).Set(user.Name, "z").Set(user.Nickname, "z").AddIDs(user.Children, c2.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err), "c2 already has a parent")
	err = client.User.Create().Set(user.Age, 10).Set(user.Name, "z").Set(user.Nickname, "z").AddIDs(user.Children, c1.ID, c2.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	grp := client.Group.Create().Set(group.Name, "Github").Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID).SaveX(ctx)
	err = client.GroupInfo.Create().Set(groupinfo.Desc, "desc").AddIDs(groupinfo.Groups, grp.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	p1 := client.Pet.Create().Set(pet.Name, "p1").SetEdge(pet.Owner, foo.ID).SaveX(ctx)
	p2 := client.Pet.Create().Set(pet.Name, "p2").SetEdge(pet.Owner, foo.ID).SaveX(ctx)
	err = client.User.Create().Set(user.Age, 10).Set(user.Name, "new-owner").AddIDs(user.Pets, p1.ID, p2.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	err = client.User.UpdateOne(c2).Set(user.Nickname, c1.Nickname).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("o2o unique constraint on update")
	err = client.User.UpdateOne(bar).SetEdge(user.Spouse, foo.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(foo).SetEdge(user.Spouse, bar.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	client.User.UpdateOne(bar).ClearEdge(user.Spouse).ExecX(ctx)
	client.User.UpdateOne(foo).ClearEdge(user.Spouse).SetEdge(user.Spouse, bar.ID).ExecX(ctx)
	require.False(dan.QuerySpouse().ExistX(ctx))
	require.Equal(bar.Name, foo.QuerySpouse().OnlyX(ctx).Name)
	require.Equal(foo.Name, bar.QuerySpouse().OnlyX(ctx).Name)

	t.Log("o2m unique constraint on update")
	err = client.User.UpdateOne(bar).Set(user.Age, 1).Set(user.Name, "new-owner").AddIDs(user.Pets, p1.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))
	err = client.User.UpdateOne(bar).Set(user.Age, 1).Set(user.Name, "new-owner").AddIDs(user.Pets, p1.ID, p2.ID).Exec(ctx)
	require.True(ent.IsConstraintError(err))

	t.Log("unique constraint violation when updating more than 1 vertex")
	err = client.User.Update().Set(user.Nickname, "yada").Exec(ctx)
	require.True(ent.IsConstraintError(err))
	require.False(client.User.Query().Where(user.Nickname.EQ("yada")).ExistX(ctx))
	client.User.Update().Where(user.Nickname.EQ("dan")).Set(user.Nickname, "yada").ExecX(ctx)
	require.False(client.User.Query().Where(user.Nickname.EQ("dan")).ExistX(ctx))
	require.True(client.User.Query().Where(user.Nickname.EQ("yada")).ExistX(ctx))

	t.Log("unique constraint on numeric fields")
	cm1 := client.Comment.Create().Set(comment.UniqueInt, 42).Set(comment.UniqueFloat, math.Pi).SaveX(ctx)
	err = client.Comment.Create().Set(comment.UniqueInt, 42).Set(comment.UniqueFloat, math.E).Exec(ctx)
	require.Error(err)
	err = client.Comment.Create().Set(comment.UniqueInt, 7).Set(comment.UniqueFloat, math.Pi).Exec(ctx)
	require.Error(err)
	client.Comment.Create().Set(comment.UniqueInt, 7).Set(comment.UniqueFloat, math.E).ExecX(ctx)
	err = cm1.Update().Set(comment.UniqueInt, 7).Exec(ctx)
	require.Error(err)
	err = cm1.Update().Set(comment.UniqueFloat, math.E).Exec(ctx)
	require.Error(err)

	t.Log("unique constraint on time fields")
	now := time.Now()
	client.File.Create().Set(file.Name, "a").Set(file.Size, 10).Set(file.CreateTime, now).ExecX(ctx)
	err = client.File.Create().Set(file.Name, "b").Set(file.Size, 20).Set(file.CreateTime, now).Exec(ctx)
	require.Error(err)
	require.True(ent.IsConstraintError(err))
	now = now.Add(time.Second)
	client.File.Create().Set(file.Name, "b").Set(file.Size, 20).Set(file.CreateTime, now).ExecX(ctx)
}

type mocker struct{ mock.Mock }

func (m *mocker) onCommit(err error)   { m.Called(err) }
func (m *mocker) onRollback(err error) { m.Called(err) }
func (m *mocker) rHook() ent.RollbackHook {
	return func(next ent.Rollbacker) ent.Rollbacker {
		return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
			err := next.Rollback(ctx, tx)
			m.onRollback(err)
			return err
		})
	}
}

func Tx(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	t.Run("Rollback", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(m.rHook())
		tx.Node.Create().ExecX(ctx)
		require.NoError(t, tx.Rollback())
		require.Zero(t, client.Node.Query().CountX(ctx), "rollback should discard all changes")
	})
	t.Run("Commit", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onCommit", mock.Anything).Twice()
		defer m.AssertExpectations(t)
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Commit(ctx, tx)
				m.onCommit(err)
				return err
			})
		})
		nde := tx.Node.Create().SaveX(ctx)
		require.NoError(t, tx.Commit())
		require.Error(t, tx.Commit(), "should return an error on the second call")
		require.NotZero(t, client.Node.Query().CountX(ctx), "commit should save all changes")
		_, err = nde.QueryNext().Count(ctx)
		require.Error(t, err, "should not be able to query after tx was closed")
		require.Zero(t, nde.Unwrap().QueryNext().CountX(ctx), "should be able to query the entity after wrap")
	})
	t.Run("Nested", func(t *testing.T) {
		tx, err := client.Tx(ctx)
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(m.rHook())
		_, err = tx.Client().Tx(ctx)
		require.Error(t, err, "cannot start a transaction within a transaction")
		require.NoError(t, tx.Rollback())
	})
	t.Run("TxOptions Rollback", func(t *testing.T) {
		skip(t, "SQLite")
		tx, err := client.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		require.NoError(t, err)
		var m mocker
		m.On("onRollback", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnRollback(func(next ent.Rollbacker) ent.Rollbacker {
			return ent.RollbackFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Rollback(ctx, tx)
				m.onRollback(err)
				require.NotNil(t, ctx)
				return err
			})
		})
		err = tx.Item.Create().Exec(ctx)
		require.Error(t, err, "expect creation to fail in read-only tx")
		require.NoError(t, tx.Rollback())
	})
	t.Run("TxOptions Commit", func(t *testing.T) {
		skip(t, "SQLite")
		tx, err := client.BeginTx(ctx, &sql.TxOptions{Isolation: stdsql.LevelReadCommitted})
		require.NoError(t, err)
		var m mocker
		m.On("onCommit", nil).Once()
		defer m.AssertExpectations(t)
		tx.OnCommit(func(next ent.Committer) ent.Committer {
			return ent.CommitFunc(func(ctx context.Context, tx *ent.Tx) error {
				err := next.Commit(ctx, tx)
				m.onCommit(err)
				require.NotNil(t, ctx)
				return err
			})
		})
		err = tx.Item.Create().Exec(ctx)
		require.NoError(t, tx.Commit())
		require.NoError(t, err)
	})
}

func DefaultValue(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	c1 := client.Card.Create().Set(card.Number, "102030").Set(card.Name, "Firstname Lastname").SaveX(ctx)
	ctime, mtime := c1.CreateTime, c1.UpdateTime
	require.False(t, ctime.IsZero())
	require.False(t, mtime.IsZero())
	c1 = c1.Update().Set(card.Name, "F Lastname").SaveX(ctx)
	require.False(t, c1.CreateTime.IsZero())
	require.False(t, c1.UpdateTime.IsZero())
	require.False(t, mtime.Equal(c1.UpdateTime))

	// Enum default value
	usr := client.User.
		Create().
		Set(user.Age, 23).
		Set(user.Name, "dario").
		SaveX(ctx)
	require.Equal(t, usr.Role, user.RoleValue("user"))

	b := time.Now().Add(-time.Hour)
	n1 := client.Node.Create().Set(node.Value, 1).Set(node.UpdatedAt, b).SaveX(ctx)
	require.NotNil(t, n1.UpdatedAt)
	require.WithinDuration(t, b, *n1.UpdatedAt, time.Second)
	n1 = n1.Update().Set(node.Value, 2).SaveX(ctx)
	require.NotNil(t, n1.UpdatedAt)
	require.False(t, b.Equal(*n1.UpdatedAt))
}

func Sensitive(t *testing.T, client *ent.Client) {
	require := require.New(t)
	ctx := context.Background()
	usr := client.User.Create().Set(user.Name, "foo").Set(user.Age, 20).Set(user.Password, "secret-password").SaveX(ctx)
	require.Equal("secret-password", usr.Password)
	require.Contains(usr.String(), "password=<sensitive>")
	b, err := json.Marshal(usr)
	require.NoError(err)
	require.NotContains(string(b), "secret-password")
}

func EagerLoading(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	a8m := client.User.Create().Set(user.Name, "a8m").Set(user.Age, 30).SaveX(ctx)
	nati := client.User.Create().Set(user.Name, "nati").Set(user.Age, 28).SetEdge(user.Spouse, a8m.ID).SaveX(ctx)
	alex := client.User.Create().Set(user.Name, "alexsn").Set(user.Age, 35).AddIDs(user.Friends, a8m.ID).SaveX(ctx)
	client.Pet.Create().Set(pet.Name, "xabi").ExecX(ctx)
	client.Pet.Create().Set(pet.Name, "pedro").SetEdge(pet.Owner, a8m.ID).SetEdge(pet.Team, nati.ID).ExecX(ctx)
	client.Card.Create().Set(card.Number, "102030").SetEdge(card.Owner, a8m.ID).ExecX(ctx)

	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx)
	files := ent.Files{
		client.File.Create().Set(file.Name, "a").Set(file.Size, 10).SaveX(ctx),
		client.File.Create().Set(file.Name, "b").Set(file.Size, 10).SaveX(ctx),
		client.File.Create().Set(file.Name, "c").Set(file.Size, 10).SaveX(ctx),
	}
	typ := client.FileType.Create().Set(filetype.Name, "type").AddIDs(filetype.Files, fileIDs(files)...).SaveX(ctx)
	hub := client.Group.Create().Set(group.Name, "GitHub").Set(group.Expire, time.Now()).AddIDs(group.Users, alex.ID, a8m.ID).SetEdge(group.Info, inf.ID).SaveX(ctx)
	lab := client.Group.Create().Set(group.Name, "GitLab").Set(group.Expire, time.Now()).AddIDs(group.Users, nati.ID, a8m.ID).SetEdge(group.Info, inf.ID).AddIDs(group.Files, fileIDs(files)...).SaveX(ctx)

	t.Run("O2O", func(t *testing.T) {
		users := client.User.
			Query().
			Where(user.Spouse.Has()).
			WithSpouse().
			WithCard().
			WithParent().
			Order(user.Name.Asc()).
			AllX(ctx)
		require.Len(users, 2)
		require.NotNil(users[0].Edges.Spouse)
		require.NotNil(users[1].Edges.Spouse)
		require.NotNil(nati.Name, users[0].Edges.Spouse.Name)
		require.NotNil(a8m.Name, users[1].Edges.Spouse.Name)
		require.NotNil(users[0].Edges.Card)
		require.Nil(users[1].Edges.Card)

		edges := users[0].Edges
		pets, err := edges.PetsOrErr()
		require.True(ent.IsNotLoaded(err))
		require.Nil(pets)
		groups, err := edges.GroupsOrErr()
		require.True(ent.IsNotLoaded(err))
		require.Nil(groups)
		card, err := edges.CardOrErr()
		require.Nil(err)
		require.NotNil(card)
		spouse, err := edges.SpouseOrErr()
		require.Nil(err)
		require.NotNil(spouse)
		parent, err := edges.ParentOrErr()
		require.True(ent.IsNotFound(err), "loaded but was not found")
		require.Nil(parent)
	})

	t.Run("O2M", func(t *testing.T) {
		pets := client.Pet.Query().AllX(ctx)
		require.Nil(pets[0].Edges.Team)
		require.Nil(pets[0].Edges.Owner)
		require.Nil(pets[1].Edges.Team)
		require.Nil(pets[1].Edges.Owner)

		pedro := client.Pet.Query().Where(pet.Owner.Has()).WithOwner().OnlyX(ctx)
		require.Nil(pedro.Edges.Team)
		require.NotNil(pedro.Edges.Owner)
		require.Equal(a8m.Name, pedro.Edges.Owner.Name)

		pedro = client.Pet.Query().Where(pet.Owner.Has()).WithOwner().WithTeam().OnlyX(ctx)
		require.NotNil(pedro.Edges.Team)
		require.NotNil(pedro.Edges.Owner)
		require.Equal(a8m.Name, pedro.Edges.Owner.Name)
		require.Equal(nati.Name, pedro.Edges.Team.Name)
	})

	t.Run("M2O", func(t *testing.T) {
		a8m := client.User.Query().Where(user.ID.EQ(a8m.ID)).OnlyX(ctx)
		require.Empty(a8m.Edges.Pets)

		a8m = client.User.
			Query().
			Where(user.ID.EQ(a8m.ID)).
			WithPets(func(q *ent.PetQuery) {
				q.WithTeam().Order(pet.Name.Asc())
			}).
			OnlyX(ctx)
		require.Len(a8m.Edges.Pets, 1)
		require.Equal("pedro", a8m.Edges.Pets[0].Name)
		require.Equal(nati.Name, a8m.Edges.Pets[0].Edges.Team.Name)
		for _, p := range a8m.Edges.Pets {
			require.Equal(a8m, p.Edges.Owner)
			u, err := p.Edges.OwnerOrErr()
			require.NoError(err)
			require.Equal(a8m, u)
		}

		a8m = client.User.
			Query().
			Where(user.ID.EQ(a8m.ID)).
			WithPets(func(q *ent.PetQuery) {
				q.Where(pet.Name.EQ("unknown"))
			}).
			OnlyX(ctx)
		require.Empty(a8m.Edges.Pets)
		require.NotNil(a8m.Edges.Pets)
	})

	t.Run("M2M", func(t *testing.T) {
		users := client.User.
			Query().
			WithFriends().
			WithGroups(func(q *ent.GroupQuery) {
				q.Order(group.Name.Desc())
			}).
			Order(user.Name.Asc()).
			AllX(ctx)
		require.Equal(a8m.Name, users[0].Name)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[0].Edges.Friends, 1)
		require.Equal(alex.Name, users[0].Edges.Friends[0].Name)
		g1, g2 := users[0].Edges.Groups[0], users[0].Edges.Groups[1]
		require.Equal(lab.Name, g1.Name)
		require.Equal(hub.Name, g2.Name)

		groups := client.Group.
			Query().
			WithUsers(func(q *ent.UserQuery) {
				q.Order(user.Name.Asc())
			}).
			Order(group.Name.Asc()).
			AllX(ctx)
		require.Len(groups, 2)
		g1, g2 = groups[0], groups[1]
		require.Equal(hub.Name, g1.Name)
		require.Equal(lab.Name, g2.Name)
		require.Equal(a8m.Name, g1.Edges.Users[0].Name)
		require.Equal(alex.Name, g1.Edges.Users[1].Name)
		require.Equal(a8m.Name, g2.Edges.Users[0].Name)
		require.Equal(nati.Name, g2.Edges.Users[1].Name)
		require.Equal(g1.Edges.Users[0], g2.Edges.Users[0], "should share the same object")
	})

	t.Run("Graph", func(t *testing.T) {
		users := client.User.
			Query().
			WithSpouse().
			WithFriends().
			WithGroups(func(q *ent.GroupQuery) {
				q.WithInfo()
				q.WithFiles(func(q *ent.FileQuery) {
					q.WithType()
					q.Order(file.Name.Asc())
				})
				q.Order(group.Name.Desc())
			}).
			Order(user.Name.Asc()).
			AllX(ctx)
		require.Equal(a8m.Name, users[0].Name)
		require.NotNil(users[0].Edges.Spouse)
		require.Equal(nati.Name, users[0].Edges.Spouse.Name)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[0].Edges.Friends, 1)
		require.Equal(alex.Name, users[0].Edges.Friends[0].Name)

		require.Equal(alex.Name, users[1].Name)
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(hub.Name, users[1].Edges.Groups[0].Name)

		require.Equal(nati.Name, users[2].Name)
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(lab.Name, users[2].Edges.Groups[0].Name)

		g1, g2 := users[0].Edges.Groups[0], users[0].Edges.Groups[1]
		require.Equal(lab.Name, g1.Name)
		require.Equal(hub.Name, g2.Name)
		require.Equal(inf.Desc, g1.Edges.Info.Desc)
		require.Equal([]string{"a", "c"}, []string{g1.Edges.Files[0].Name, g1.Edges.Files[2].Name})
		for _, f := range g1.Edges.Files {
			require.NotNil(f.Edges.Type)
			require.Equal(typ.Name, f.Edges.Type.Name)
		}
	})

	t.Run("LimitRows/O2M", func(t *testing.T) {
		client.Pet.Delete().ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "nala").SetEdge(pet.Owner, nati.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "xabi3").SetEdge(pet.Owner, a8m.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "xabi2").SetEdge(pet.Owner, a8m.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "xabi1").SetEdge(pet.Owner, a8m.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "lola4").SetEdge(pet.Owner, alex.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "lola3").SetEdge(pet.Owner, alex.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "lola2").SetEdge(pet.Owner, alex.ID).ExecX(ctx)
		client.Pet.Create().Set(pet.Name, "lola1").SetEdge(pet.Owner, alex.ID).ExecX(ctx)

		users := client.User.Query().WithPets().Order(user.ID.Asc()).AllX(ctx)
		require.Len(users[0].Edges.Pets, 3)
		require.Len(users[1].Edges.Pets, 1)
		require.Len(users[2].Edges.Pets, 4)

		users = client.User.
			Query().
			WithPets(func(q *ent.PetQuery) {
				q.Modify(limitRows(pet.OwnerColumn, 2))
			}).
			Order(user.ID.Asc()).
			AllX(ctx)
		require.Len(users[0].Edges.Pets, 2)
		require.Equal(users[0].Edges.Pets[0].Name, "xabi3")
		require.Equal(users[0].Edges.Pets[1].Name, "xabi2")
		require.Len(users[1].Edges.Pets, 1)
		require.Equal(users[1].Edges.Pets[0].Name, "nala")
		require.Len(users[2].Edges.Pets, 2)
		require.Equal(users[2].Edges.Pets[0].Name, "lola4")
		require.Equal(users[2].Edges.Pets[1].Name, "lola3")

		users = client.User.
			Query().
			WithPets(func(q *ent.PetQuery) {
				q.Modify(limitRows(pet.OwnerColumn, 1, pet.FieldName))
			}).
			Order(user.ID.Asc()).
			AllX(ctx)
		require.Len(users[0].Edges.Pets, 1)
		require.Equal(users[0].Edges.Pets[0].Name, "xabi1")
		require.Len(users[1].Edges.Pets, 1)
		require.Equal(users[1].Edges.Pets[0].Name, "nala")
		require.Len(users[2].Edges.Pets, 1)
		require.Equal(users[2].Edges.Pets[0].Name, "lola1")
	})

	t.Run("LimitRows/M2M", func(t *testing.T) {
		users := client.User.Query().WithGroups().Order(user.ID.Asc()).AllX(ctx)
		require.Len(users[0].Edges.Groups, 2)
		require.Len(users[1].Edges.Groups, 1)
		require.Len(users[2].Edges.Groups, 1)

		users = client.User.
			Query().
			WithGroups(func(q *ent.GroupQuery) {
				q.Modify(limitRows(user.GroupsPrimaryKey[0], 1))
			}).
			Order(user.ID.Asc()).
			AllX(ctx)
		require.Len(users[0].Edges.Groups, 1)
		require.Equal(users[0].Edges.Groups[0].Name, "GitHub")
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(users[1].Edges.Groups[0].Name, "GitLab")
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(users[2].Edges.Groups[0].Name, "GitHub")

		client.Group.Create().Set(group.Name, "BitBucket").Set(group.Expire, time.Now()).AddIDs(group.Users, alex.ID, a8m.ID).SetEdge(group.Info, inf.ID).SaveX(ctx)
		users = client.User.
			Query().
			WithGroups(func(q *ent.GroupQuery) {
				q.Modify(limitRows(user.GroupsPrimaryKey[0], 1, group.FieldName))
			}).
			Order(user.ID.Asc()).
			AllX(ctx)
		require.Len(users[0].Edges.Groups, 1)
		require.Equal(users[0].Edges.Groups[0].Name, "BitBucket")
		require.Len(users[1].Edges.Groups, 1)
		require.Equal(users[1].Edges.Groups[0].Name, "GitLab")
		require.Len(users[2].Edges.Groups, 1)
		require.Equal(users[2].Edges.Groups[0].Name, "BitBucket")
	})
}

func limitRows(partitionBy string, limit int, orderBy ...string) func(s *sql.Selector) {
	return func(s *sql.Selector) {
		d := sql.Dialect(s.Dialect())
		s.SetDistinct(false)
		if len(orderBy) == 0 {
			orderBy = append(orderBy, "id")
		}
		with := d.With("src_query").
			As(s.Clone()).
			With("limited_query").
			As(
				d.Select("*").
					AppendSelectExprAs(
						sql.RowNumber().PartitionBy(partitionBy).OrderBy(orderBy...),
						"row_number",
					).
					From(d.Table("src_query")),
			)
		t := d.Table("limited_query").As(s.TableName())
		*s = *d.Select(s.UnqualifiedColumns()...).
			From(t).
			Where(sql.LTE(t.C("row_number"), limit)).
			Prefix(with)
	}
}

func NamedEagerLoading(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	a8m := client.User.Create().Set(user.Name, "a8m").Set(user.Age, 30).SaveX(ctx)
	p1 := client.Pet.Create().Set(pet.Name, "pet1").SetEdge(pet.Owner, a8m.ID).Set(pet.Trained, true).SaveX(ctx)
	p2 := client.Pet.Create().Set(pet.Name, "pet2").SetEdge(pet.Owner, a8m.ID).Set(pet.Trained, false).SaveX(ctx)

	a8m = client.User.Query().
		WithNamedPets("Trained", func(q *ent.PetQuery) { q.Where(pet.Trained.EQ(true)) }).
		WithNamedPets("Untrained", func(q *ent.PetQuery) { q.Where(pet.Trained.EQ(false)) }).
		OnlyX(ctx)
	trained, err := a8m.NamedPets("Trained")
	require.NoError(t, err)
	require.Len(t, trained, 1)
	require.Equal(t, p1.ID, trained[0].ID)
	untrained, err := a8m.NamedPets("Untrained")
	require.NoError(t, err)
	require.Len(t, untrained, 1)
	require.Equal(t, p2.ID, untrained[0].ID)
	unknown, err := a8m.NamedPets("Unknown")
	require.True(t, ent.IsNotLoaded(err))
	require.Nil(t, unknown)

	exists := client.User.Query().
		WithNamedPets("Existing").
		ExistX(ctx)
	require.True(t, exists)
}

// writerFunc is an io.Writer implemented by the underlying func.
type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func NoSchemaChanges(t *testing.T, client *ent.Client) {
	w := writerFunc(func(p []byte) (int, error) {
		stmt := strings.Trim(string(p), "\n;")
		ok := []*regexp.Regexp{
			regexp.MustCompile("^BEGIN$"),
			regexp.MustCompile("^COMMIT$"),
		}
		switch {
		case strings.Contains(t.Name(), "SQLite"):
			ok = append(ok, regexp.MustCompile("^PRAGMA foreign_keys = (off|on)$"))
		}
		if !slices.ContainsFunc(ok, func(re *regexp.Regexp) bool {
			return re.MatchString(stmt)
		}) {
			t.Errorf("expect no statement to execute. got: %q", stmt)
		}
		return len(p), nil
	})
	tables, err := sqlschema.CopyTables(migrate.Tables)
	require.NoError(t, err)
	err = migrate.Create(
		context.Background(),
		migrate.NewSchema(&sqlschema.WriteDriver{Writer: w, Driver: client.Driver()}),
		tables,
		migrate.WithDropIndex(true),
		migrate.WithDropColumn(true),
	)
	require.NoError(t, err)
}

func Mutation(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	ub := client.User.Insert(ent.UserInsert{Name: "a8m", Age: 30})
	pb := client.Pet.Insert(ent.PetInsert{Name: "pedro"})

	a8m := ub.SaveX(ctx)
	require.Equal(t, "a8m", a8m.Name)
	pedro := pb.SaveX(ctx)
	require.Equal(t, "pedro", pedro.Name)

	setUsers := func(ms ...*ent.UserMutation) {
		for _, m := range ms {
			if insert := m.Insert(); insert != nil {
				insert.Name, insert.Age = "boring", 30
			}
			if patch := m.Patch(); patch != nil {
				patch.Name, patch.Age = ent.Some("boring"), ent.Some(30)
			}
		}
	}
	uu := a8m.Update().AddIDs(user.Pets, pedro.ID)
	ub = client.User.Insert(ent.UserInsert{})
	setUsers(ub.Mutation(), uu.Mutation())
	a8m = uu.SaveX(ctx)
	usr := ub.SaveX(ctx)
	require.Equal(t, "boring", a8m.Name)
	require.Equal(t, "boring", usr.Name)

	require.Equal(t, []int{usr.ID}, a8m.Update().AddIDs(user.Friends, usr.ID).Patch().Friends.Add)
	require.Empty(t, a8m.Update().AddIDs(user.Friends, usr.ID).RemoveIDs(user.Friends, usr.ID).Patch().Friends.Add)
	require.Equal(t, []int{usr.ID}, a8m.Update().AddIDs(user.Friends, usr.ID).RemoveIDs(user.Friends, a8m.ID).Patch().Friends.Add)
	a8m.Update().AddIDs(user.Friends, usr.ID).ExecX(ctx)

	t.Run("IDs", func(t *testing.T) {
		ids := client.User.Query().IDsX(ctx)
		u := client.User.Update().Where(user.ID.In(ids...)).Add(user.Age, 1)
		mids, err := u.Mutation().IDs(ctx)
		require.NoError(t, err)
		// Order can change between the 2 queries.
		sort.Ints(ids)
		sort.Ints(mids)
		require.Equal(t, ids, mids)
		u.ExecX(ctx)

		u = client.User.
			Update().
			Add(user.Age, 1).
			Where(
				user.Name.EQ(a8m.Name),
				user.Pets.Has(),
				user.Pets.HasWith(
					pet.Name.EQ(pedro.Name),
				),
			)
		mids, err = u.Mutation().IDs(ctx)
		require.NoError(t, err)
		require.Len(t, mids, 1)
		require.Equal(t, a8m.ID, mids[0])
		u.ExecX(ctx)
	})

	t.Run("Predicate", func(t *testing.T) {
		updater := a8m.Update()
		updater.Mutation().Where(user.Name.EQ(a8m.Name))
		updater.Set(user.Name, "mashraki")
		a8m, err := updater.Save(ctx)
		require.NoError(t, err, "predicate should not affect the returned object")
		require.Equal(t, "mashraki", a8m.Name)

		updater = a8m.Update()
		updater.Mutation().Where(user.Name.EQ(a8m.Name + a8m.Name))
		updater.Set(user.Name, "a8m")
		a8m, err = updater.Save(ctx)
		require.True(t, ent.IsNotFound(err))
		require.Nil(t, a8m)
	})
}

// Test templates codegen.
var (
	_ = ent.CardExtension{}
	_ = ent.Card{}.StaticField
	_ = ent.Client{}.TemplateField
	_ = []filetype.StateValue{filetype.StateOn, filetype.StateOff}
	_ = []filetype.TypeValue{filetype.TypeJPG, filetype.TypePNG, filetype.TypeSVG}
)

func entityIDs(users []*ent.User) []int {
	ids := make([]int, len(users))
	for index := range users {
		ids[index] = users[index].ID
	}
	return ids
}

func fileIDs(files []*ent.File) []int {
	ids := make([]int, len(files))
	for index := range files {
		ids[index] = files[index].ID
	}
	return ids
}

func CreateBulk(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	cards := client.Card.CreateBulk(
		client.Card.Create().Set(card.Number, "10").Set(card.Name, "1st"),
		client.Card.Create().Set(card.Number, "20").Set(card.Name, "2nd"),
		client.Card.Create().Set(card.Number, "30").Set(card.Name, "3rd"),
	).SaveX(ctx)
	require.Equal(t, cards[0].ID, cards[1].ID-1)
	require.Equal(t, cards[1].ID, cards[2].ID-1)

	inf := client.GroupInfo.Create().Set(groupinfo.Desc, "group info").SaveX(ctx)
	names := []string{"GitHub", "GitLab"}
	groups := client.Group.MapCreateBulk(names, func(c *ent.GroupCreate, i int) {
		c.Set(group.Name, names[i]).Set(group.Expire, time.Now()).SetEdge(group.Info, inf.ID)
	}).SaveX(ctx)
	require.Equal(t, inf.ID, groups[0].QueryInfo().OnlyIDX(ctx))
	require.Equal(t, inf.ID, groups[1].QueryInfo().OnlyIDX(ctx))

	_, err := client.Group.MapCreateBulk(1, nil).Save(ctx)
	require.Error(t, err)

	users := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "a8m").Set(user.Age, 20).AddIDs(user.Groups, groups[0].ID, groups[1].ID),
		client.User.Create().Set(user.Name, "nati").Set(user.Age, 20).SetEdge(user.Card, cards[0].ID).AddIDs(user.Groups, groups[0].ID),
	).SaveX(ctx)
	require.Equal(t, 2, users[0].QueryGroups().CountX(ctx))
	require.False(t, users[0].QueryCard().ExistX(ctx))
	require.Equal(t, groups[0].ID, users[1].QueryGroups().OnlyIDX(ctx))
	require.Equal(t, cards[0].ID, users[1].QueryCard().OnlyIDX(ctx))

	pets := client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "pedro").SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "xabi").SetEdge(pet.Owner, users[1].ID),
		client.Pet.Create().Set(pet.Name, "layla"),
	).SaveX(ctx)
	require.Equal(t, "pedro", pets[0].Name)
	require.Equal(t, users[0].ID, pets[0].QueryOwner().OnlyIDX(ctx))
	require.Equal(t, "xabi", pets[1].Name)
	require.Equal(t, users[1].ID, pets[1].QueryOwner().OnlyIDX(ctx))
	require.Equal(t, "layla", pets[2].Name)
	require.False(t, pets[2].QueryOwner().ExistX(ctx))
}

func ConstraintChecks(t *testing.T, client *ent.Client) {
	var cerr *ent.ConstraintError
	err := client.Pet.Create().Set(pet.Name, "orphan").SetEdge(pet.Owner, 0).Exec(context.Background())
	require.True(t, errors.As(err, &cerr))
	require.True(t, sqlgraph.IsForeignKeyConstraintError(err))
	require.False(t, sqlgraph.IsUniqueConstraintError(err))

	client.FileType.Create().Set(filetype.Name, "a unique name").SaveX(context.Background())
	err = client.FileType.Create().Set(filetype.Name, "a unique name").Exec(context.Background())
	require.True(t, errors.As(err, &cerr))
	require.False(t, sqlgraph.IsForeignKeyConstraintError(err))
	require.True(t, sqlgraph.IsUniqueConstraintError(err))
}

func Lock(t *testing.T, client *ent.Client) {
	skip(t, "SQLite")
	ctx := context.Background()
	xabi := client.Pet.Create().Set(pet.Name, "Xabi").SaveX(ctx)

	t.Run("ForUpdate", func(t *testing.T) {
		tx1, err := client.Tx(ctx)
		require.NoError(t, err)
		tx2, err := client.Tx(ctx)
		require.NoError(t, err)
		tx3, err := client.Tx(ctx)
		require.NoError(t, err)
		p1 := tx1.Pet.Query().Where(pet.ID.EQ(xabi.ID)).ForUpdate().OnlyX(ctx)
		_, err = tx2.Pet.Query().Where(pet.ID.EQ(xabi.ID)).ForUpdate(sql.WithLockAction(sql.NoWait)).Only(ctx)
		if strings.Contains(t.Name(), "Postgres") {
			pgErr, ok := errors.AsType[*pgconn.PgError](err)
			require.True(t, ok, "expected a pgconn.PgError, got %T: %v", err, err)
			require.EqualValues(t, "55P03", pgErr.Code)
			require.EqualValues(t, `could not obtain lock on row in relation "pet"`, pgErr.Message)
		}
		require.NoError(t, tx2.Rollback())
		p1.Update().Set(pet.Name, "updated").ExecX(ctx)
		require.NoError(t, tx1.Commit())
		tx3.Pet.Query().Where(pet.ID.EQ(xabi.ID)).ForUpdate().OnlyX(ctx)
		require.NoError(t, tx3.Rollback())
	})

	t.Run("ForShare", func(t *testing.T) {
		tx1, err := client.Tx(ctx)
		require.NoError(t, err)
		tx2, err := client.Tx(ctx)
		require.NoError(t, err)
		tx3, err := client.Tx(ctx)
		require.NoError(t, err)
		tx1.Pet.Query().Where(pet.ID.EQ(xabi.ID)).ForShare().OnlyX(ctx)
		tx2.Pet.Query().Where(pet.ID.EQ(xabi.ID)).ForShare().OnlyX(ctx)
		_, err = tx3.Pet.Query().
			Where(pet.ID.EQ(xabi.ID)).
			ForUpdate(
				sql.WithLockTables(pet.Table),
				sql.WithLockAction(sql.NoWait),
			).
			Only(ctx)
		require.Error(t, err)
		require.NoError(t, tx1.Rollback())
		require.NoError(t, tx2.Rollback())
		require.NoError(t, tx3.Rollback())
	})
}

func ExtValueScan(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	u, err := url.Parse("https://entgo.io")
	require.NoError(t, err)
	check := func(ex *ent.ExValueScan, i *big.Int, u, b64, custom string, ub *url.URL) {
		for _, e := range []*ent.ExValueScan{ex, client.ExValueScan.GetX(ctx, ex.ID)} {
			require.Equal(t, i, e.Text)
			require.Equal(t, u, e.Binary.String())
			require.Equal(t, ub, e.BinaryBytes)
			require.Equal(t, b64, e.Base64)
			require.Equal(t, custom, e.Custom)
		}
	}
	ex := client.ExValueScan.Create().
		Set(exvaluescan.Text, big.NewInt(10)).
		Set(exvaluescan.Binary, u).
		Set(exvaluescan.BinaryBytes, u).
		Set(exvaluescan.Base64, "a8m").
		Set(exvaluescan.Custom, "atlasgo.io").
		SaveX(ctx)
	check(ex, big.NewInt(10), u.String(), "a8m", "atlasgo.io", u)

	// Ensure the database values store as expected.
	var raw []struct {
		Text        string
		Binary      string
		BinaryBytes []byte `sql:"binary_bytes"`
		Base64      string
		Custom      string
	}
	require.NoError(t, client.ExValueScan.Query().
		Select(
			exvaluescan.Text,
			exvaluescan.Binary,
			exvaluescan.BinaryBytes,
			exvaluescan.Base64,
			exvaluescan.Custom,
		).Scan(ctx, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "10", raw[0].Text)
	require.Equal(t, u.String(), raw[0].Binary)
	ub, err := u.MarshalBinary()
	require.NoError(t, err)
	require.Equal(t, ub, raw[0].BinaryBytes)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte(ex.Base64)), raw[0].Base64)
	require.Equal(t, "0x:"+hex.EncodeToString([]byte(ex.Custom)), raw[0].Custom)

	// Update the values and ensure they are updated as expected.
	u.Path = "/docs"
	ex = ex.Update().Set(exvaluescan.Binary, u).Set(exvaluescan.BinaryBytes, u).Set(exvaluescan.Text, big.NewInt(20)).Set(exvaluescan.Base64, "m8a").Set(exvaluescan.Custom, "entgo.io").SaveX(ctx)
	check(ex, big.NewInt(20), u.String(), "m8a", "entgo.io", u)

	// Check predicates.
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.Text.EQ(big.NewInt(20))).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Text.EQ(big.NewInt(10))).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.Text.LTE(big.NewInt(20))).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Text.LTE(big.NewInt(10))).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.Binary.EQ(u)).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Binary.EQ(&url.URL{})).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.Base64.In("m8a")).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Base64.In("a8m")).ExistX(ctx))
	require.True(t, client.ExValueScan.Query().Where(exvaluescan.Custom.HasPrefix("ent")).ExistX(ctx))
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Custom.HasPrefix("atlas")).ExistX(ctx))
	// HasSuffix cannot work with this field as the value is stored as hex (with additional prefix).
	require.False(t, client.ExValueScan.Query().Where(exvaluescan.Custom.HasSuffix("io")).ExistX(ctx))
}

func OrderByFluent(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "a").Set(user.Age, 1),
		client.User.Create().Set(user.Name, "b").Set(user.Age, 2),
		client.User.Create().Set(user.Name, "c").Set(user.Age, 3),
		client.User.Create().Set(user.Name, "d").Set(user.Age, 4),
		client.User.Create().Set(user.Name, "e").Set(user.Age, 5),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "aa").SetEdge(pet.Owner, users[1].ID).Set(pet.Age, 2),
		client.Pet.Create().Set(pet.Name, "ab").SetEdge(pet.Owner, users[1].ID).Set(pet.Age, 2),
		client.Pet.Create().Set(pet.Name, "ac").SetEdge(pet.Owner, users[0].ID).Set(pet.Age, 1),
		client.Pet.Create().Set(pet.Name, "ba").SetEdge(pet.Owner, users[0].ID).Set(pet.Age, 1),
		client.Pet.Create().Set(pet.Name, "bb").SetEdge(pet.Owner, users[0].ID).Set(pet.Age, 1),
		client.Pet.Create().Set(pet.Name, "ca").SetEdge(pet.Owner, users[2].ID).Set(pet.Age, 10),
		client.Pet.Create().Set(pet.Name, "d"),
		client.Pet.Create().Set(pet.Name, "e"),
	).SaveX(ctx)

	t.Run("M2O", func(t *testing.T) {
		ids := client.Pet.Query().
			Order(
				pet.Owner.OrderBy(sql.OrderByField(user.FieldName)),
				pet.ID.Asc(),
			).
			IDsX(ctx)
		require.Equal(t, []int{pets[6].ID, pets[7].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[0].ID, pets[1].ID, pets[5].ID}, ids)

		ids = client.Pet.Query().
			Order(
				pet.Owner.OrderBy(sql.OrderByField(user.FieldName, sql.OrderDesc())),
				pet.ID.Desc(),
			).
			IDsX(ctx)
		require.Equal(t, []int{pets[5].ID, pets[1].ID, pets[0].ID, pets[4].ID, pets[3].ID, pets[2].ID, pets[7].ID, pets[6].ID}, ids)
	})

	t.Run("M2O/SelectedOwner", func(t *testing.T) {
		ownerName := user.Name.As("owner_name").Nullable()
		rows, err := client.Pet.Query().
			LeftJoinAs(user.Table, user.Table, func(selector *sql.Selector) {
				selector.Where(sql.ColumnsEQ(selector.C(pet.OwnerColumn), sql.Table(user.Table).C(user.FieldID)))
			}).
			Order(pet.Owner.OrderBy(user.Name.Term()), pet.ID.Asc()).
			Select(ownerName).Rows(ctx)
		require.NoError(t, err)
		for index, row := range rows {
			name, present := ent.GetNullable(row, ownerName)
			switch {
			case index < 2:
				require.False(t, present)
			case index < 5:
				require.Equal(t, users[0].Name, name)
			case index < 7:
				require.Equal(t, users[1].Name, name)
			default:
				require.Equal(t, users[2].Name, name)
			}
		}
	})

	t.Run("O2M/Count", func(t *testing.T) {
		ids := client.User.Query().
			Order(
				user.Pets.OrderByCount(),
				user.ID.Desc(),
			).
			IDsX(ctx)
		require.Equal(t, []int{users[4].ID, users[3].ID, users[2].ID, users[1].ID, users[0].ID}, ids)

		ids = client.User.Query().
			Order(
				user.Pets.OrderByCount(sql.OrderDesc()),
				user.ID.Desc(),
			).
			IDsX(ctx)
		require.Equal(t, []int{users[0].ID, users[1].ID, users[2].ID, users[4].ID, users[3].ID}, ids)
	})

	t.Run("O2M/SelectedCount", func(t *testing.T) {
		for _, descending := range []bool{false, true} {
			options := []sql.OrderTermOption{}
			if descending {
				options = append(options, sql.OrderDesc())
			}
			ordered := client.User.Query().WithCount(user.Pets).
				Order(user.Pets.OrderByCount(options...), user.ID.Asc()).AllX(ctx)
			for _, loaded := range ordered {
				count, present := loaded.Edges.Count(user.Pets)
				require.True(t, present)
				require.Equal(t, loaded.QueryPets().CountX(ctx), count)
			}
		}
	})

	t.Run("O2M/Sum", func(t *testing.T) {
		ordered := client.User.Query().
			Order(
				user.Pets.OrderBy(
					sql.OrderBySum(
						pet.FieldAge,
						sql.OrderDesc(),
					),
				),
				user.ID.Asc(),
			).
			AllX(ctx)
		require.Equal(t,
			[]int{users[2].ID, users[1].ID, users[0].ID, users[3].ID, users[4].ID},
			[]int{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[3].ID, ordered[4].ID},
		)

		ordered = client.User.Query().
			Order(
				user.Pets.OrderBy(
					sql.OrderBySum(
						pet.FieldAge,
						sql.OrderDesc(),
						sql.OrderSelected(),
					),
				),
				user.ID.Desc(),
			).
			AllX(ctx)
		require.Equal(t,
			[]int{users[2].ID, users[1].ID, users[0].ID, users[3].ID, users[4].ID},
			[]int{ordered[0].ID, ordered[1].ID, ordered[2].ID, ordered[4].ID, ordered[3].ID},
		)
		var projected []struct {
			ID     int
			SumAge *float64 `sql:"sum_age"`
		}
		require.NoError(t, client.User.Query().Order(user.Pets.OrderBy(sql.OrderBySum(pet.FieldAge, sql.OrderDesc(), sql.OrderSelected())), user.ID.Desc()).
			Select(user.ID).Modify(func(selector *sql.Selector) {
			// The order term already selected its aggregate before the explicit projection.
			selector.AppendSelect(sql.As(sql.Table("t1").C("sum_age"), "sum_age"))
		}).Scan(ctx, &projected))
		require.EqualValues(t, 10, *projected[0].SumAge)
		require.EqualValues(t, 4, *projected[1].SumAge)
		require.EqualValues(t, 3, *projected[2].SumAge)
	})
}

// Testing the "low-level" behavior of the sqlgraph package.
// This functionality may be extended to the generated fluent API.
func OrderByEdgeCount(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "a").Set(user.Age, 1),
		client.User.Create().Set(user.Name, "b").Set(user.Age, 2),
		client.User.Create().Set(user.Name, "c").Set(user.Age, 3),
		client.User.Create().Set(user.Name, "d").Set(user.Age, 4),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "aa").SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "ab").SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "ac").SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "ba").SetEdge(pet.Owner, users[1].ID),
		client.Pet.Create().Set(pet.Name, "bb").SetEdge(pet.Owner, users[1].ID),
		client.Pet.Create().Set(pet.Name, "ca").SetEdge(pet.Owner, users[2].ID),
		client.Pet.Create().Set(pet.Name, "d"),
		client.Pet.Create().Set(pet.Name, "e"),
	).SaveX(ctx)
	// O2M edge.
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID}},
		{ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID}},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(pet.Table, pet.OwnerColumn),
						sqlgraph.Edge(sqlgraph.O2M, false, pet.Table, pet.OwnerColumn),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// M2O edge (true or false).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{pets[6].ID, pets[7].ID, pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[5].ID}},
		{ids: []int{pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[5].ID, pets[6].ID, pets[7].ID}},
	} {
		ids := client.Pet.Query().
			Order(
				func(s *sql.Selector) {
					sqlgraph.OrderByNeighborsCount(s,
						sqlgraph.NewStep(
							sqlgraph.From(pet.Table, pet.OwnerColumn),
							sqlgraph.To(user.Table, user.FieldID),
							sqlgraph.Edge(sqlgraph.M2O, true, pet.Table, pet.OwnerColumn),
						),
						tt.opts...,
					)
				},
				pet.ID.Asc(),
			).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	inf, exp := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx), time.Now()
	groups := client.Group.CreateBulk(
		client.Group.Create().Set(group.Name, "Group: 4 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users)...),
		client.Group.Create().Set(group.Name, "Group: 3 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users[:3])...),
		client.Group.Create().Set(group.Name, "Group: 2 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users[:2])...),
		client.Group.Create().Set(group.Name, "Group: 1 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID).AddIDs(group.Users, users[0].ID),
		client.Group.Create().Set(group.Name, "Group: 0 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID),
	).SaveX(ctx)
	// M2M edge (inverse).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{groups[0].ID, groups[1].ID, groups[2].ID, groups[3].ID, groups[4].ID}},
		{ids: []int{groups[4].ID, groups[3].ID, groups[2].ID, groups[1].ID, groups[0].ID}},
	} {
		ids := client.Group.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(group.Table, group.FieldID),
						sqlgraph.To(user.Table, user.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, true, group.UsersTable, group.UsersPrimaryKey...),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// M2M edge (assoc).
	for _, tt := range []struct {
		opts []sql.OrderTermOption
		ids  []int
	}{
		{opts: []sql.OrderTermOption{sql.OrderDesc()}, ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID}},
		{ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID}},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborsCount(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(group.Table, group.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, false, user.GroupsTable, user.GroupsPrimaryKey...),
					),
					tt.opts...,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}

	t.Run("Count", func(t *testing.T) {
		nodes := client.User.Query().WithCount(user.Pets).Order(user.Pets.OrderByCount(sql.OrderDesc())).AllX(ctx)
		require.Len(t, nodes, 4)
		for index, expected := range []int{3, 2, 1, 0} {
			count, present := nodes[index].Edges.Count(user.Pets)
			require.True(t, present)
			require.Equal(t, expected, count)
		}
	})
}

// Testing the "low-level" behavior of the sqlgraph package.
// This functionality may be extended to the generated fluent API.
func OrderByEdgeTerms(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	users := client.User.CreateBulk(
		client.User.Create().Set(user.Name, "a").Set(user.Age, 1),
		client.User.Create().Set(user.Name, "b").Set(user.Age, 2),
		client.User.Create().Set(user.Name, "c").Set(user.Age, 3),
		client.User.Create().Set(user.Name, "d").Set(user.Age, 4),
	).SaveX(ctx)
	pets := client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.Name, "aa").Set(pet.Age, 2).SetEdge(pet.Owner, users[1].ID),
		client.Pet.Create().Set(pet.Name, "ab").Set(pet.Age, 2).SetEdge(pet.Owner, users[1].ID),
		client.Pet.Create().Set(pet.Name, "ac").Set(pet.Age, 1).SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "ba").Set(pet.Age, 1).SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "bb").Set(pet.Age, 1).SetEdge(pet.Owner, users[0].ID),
		client.Pet.Create().Set(pet.Name, "ca").Set(pet.Age, 3).SetEdge(pet.Owner, users[2].ID),
		client.Pet.Create().Set(pet.Name, "d"),
		client.Pet.Create().Set(pet.Name, "e"),
	).SaveX(ctx)
	// M2O edge (inverse).
	// Order pets by their owner's name.
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderByField(user.FieldName),
			ids: []int{pets[6].ID, pets[7].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[0].ID, pets[1].ID, pets[5].ID},
		},
		{
			opt: sql.OrderByField(user.FieldName, sql.OrderDesc()),
			ids: []int{pets[5].ID, pets[0].ID, pets[1].ID, pets[2].ID, pets[3].ID, pets[4].ID, pets[6].ID, pets[7].ID},
		},
	} {
		ids := client.Pet.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(pet.Table, pet.FieldID),
						sqlgraph.To(user.Table, user.FieldID),
						sqlgraph.Edge(sqlgraph.M2O, true, pet.Table, pet.OwnerColumn),
					),
					tt.opt,
				)
			}).
			Order(pet.ID.Asc()).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
	// O2M edge (aggregation).
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderBySum(user.FieldAge),
			ids: []int{users[3].ID, users[0].ID, users[2].ID, users[1].ID},
		},
		{
			opt: sql.OrderBySum(user.FieldAge, sql.OrderDesc()),
			ids: []int{users[1].ID, users[0].ID, users[2].ID, users[3].ID},
		},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(pet.Table, pet.FieldID),
						sqlgraph.Edge(sqlgraph.O2M, false, pet.Table, pet.OwnerColumn),
					),
					tt.opt,
				)
			}).
			Order(user.ID.Asc()).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}

	inf, exp := client.GroupInfo.Create().Set(groupinfo.Desc, "desc").SaveX(ctx), time.Now()
	client.Group.CreateBulk(
		client.Group.Create().Set(group.Name, "Group: 4 users").Set(group.Expire, exp).Set(group.MaxUsers, 40).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users)...),
		client.Group.Create().Set(group.Name, "Group: 3 users").Set(group.Expire, exp).Set(group.MaxUsers, 20).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users[:3])...),
		client.Group.Create().Set(group.Name, "Group: 2 users").Set(group.Expire, exp).Set(group.MaxUsers, 20).SetEdge(group.Info, inf.ID).AddIDs(group.Users, entityIDs(users[:2])...),
		client.Group.Create().Set(group.Name, "Group: 1 users").Set(group.Expire, exp).Set(group.MaxUsers, 100).SetEdge(group.Info, inf.ID).AddIDs(group.Users, users[0].ID),
		client.Group.Create().Set(group.Name, "Group: 0 users").Set(group.Expire, exp).SetEdge(group.Info, inf.ID),
	).ExecX(ctx)
	// M2M edge.
	for _, tt := range []struct {
		opt sql.OrderTerm
		ids []int
	}{
		{
			opt: sql.OrderBySum(
				group.FieldMaxUsers,
			),
			ids: []int{users[3].ID, users[2].ID, users[1].ID, users[0].ID},
		},
		{
			opt: sql.OrderBySum(
				group.FieldMaxUsers,
				sql.OrderDesc(),
			),
			ids: []int{users[0].ID, users[1].ID, users[2].ID, users[3].ID},
		},
	} {
		ids := client.User.Query().
			Order(func(s *sql.Selector) {
				sqlgraph.OrderByNeighborTerms(s,
					sqlgraph.NewStep(
						sqlgraph.From(user.Table, user.FieldID),
						sqlgraph.To(group.Table, group.FieldID),
						sqlgraph.Edge(sqlgraph.M2M, false, user.GroupsTable, user.GroupsPrimaryKey...),
					),
					tt.opt,
				)
			}).
			IDsX(ctx)
		require.Equal(t, tt.ids, ids)
	}
}

func skip(t *testing.T, names ...string) {
	for _, n := range names {
		if strings.Contains(t.Name(), n) {
			t.Skipf("skip %s", n)
		}
	}
}

func drop(t *testing.T, client *ent.Client) {
	t.Log("drop data from database")
	ctx := context.Background()
	client.Pet.Delete().ExecX(ctx)
	client.Item.Delete().ExecX(ctx)
	client.Task.Delete().ExecX(ctx)
	client.File.Delete().ExecX(ctx)
	client.Card.Delete().ExecX(ctx)
	client.Node.Delete().ExecX(ctx)
	client.User.Delete().ExecX(ctx)
	client.Group.Delete().ExecX(ctx)
	client.Comment.Delete().ExecX(ctx)
	client.GroupInfo.Delete().ExecX(ctx)
	client.FieldType.Delete().ExecX(ctx)
	client.FileType.Delete().ExecX(ctx)
	client.ExValueScan.Delete().ExecX(ctx)
}
