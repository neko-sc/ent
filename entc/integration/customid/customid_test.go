// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package customid

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/pg"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/entc/integration/customid/ent"
	account "github.com/neko-sc/ent/entc/integration/customid/ent/account"
	"github.com/neko-sc/ent/entc/integration/customid/ent/blob"
	car "github.com/neko-sc/ent/entc/integration/customid/ent/car"
	device "github.com/neko-sc/ent/entc/integration/customid/ent/device"
	"github.com/neko-sc/ent/entc/integration/customid/ent/doc"
	group "github.com/neko-sc/ent/entc/integration/customid/ent/group"
	"github.com/neko-sc/ent/entc/integration/customid/ent/intsid"
	note "github.com/neko-sc/ent/entc/integration/customid/ent/note"
	other "github.com/neko-sc/ent/entc/integration/customid/ent/other"
	"github.com/neko-sc/ent/entc/integration/customid/ent/pet"
	"github.com/neko-sc/ent/entc/integration/customid/ent/revision"
	entschema "github.com/neko-sc/ent/entc/integration/customid/ent/schema"
	"github.com/neko-sc/ent/entc/integration/customid/ent/token"
	"github.com/neko-sc/ent/entc/integration/customid/ent/user"
	"github.com/neko-sc/ent/entc/integration/customid/ent/valuescan"
	"github.com/neko-sc/ent/entc/integration/customid/sid"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	atlas "github.com/neko-sc/atlas/sql/schema"
	"github.com/stretchr/testify/require"
)

func TestPostgres(t *testing.T) {
	for version, port := range map[string]int{"16": 5436, "17": 5437, "18": 5438} {
		t.Run(version, func(t *testing.T) {
			dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=pass sslmode=disable dbname=test", port)
			db, err := sql.Open(dialect.Postgres, dsn)
			require.NoError(t, err)
			defer db.Close()
			_, err = db.Exec("CREATE SCHEMA IF NOT EXISTS custom_id")
			require.NoError(t, err, "creating schema")
			_, err = db.Exec("SET search_path TO custom_id")
			require.NoError(t, err, "setting schema")
			_, err = db.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp" SCHEMA custom_id`)
			require.NoError(t, err, "creating extension")
			defer db.Exec(`DROP EXTENSION "uuid-ossp"`)
			defer db.Exec("DROP SCHEMA custom_id CASCADE")

			client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			err = client.Schema.Create(context.Background(), schema.WithDiffHook(expectOnePetsIndex))
			require.NoError(t, err)
			CustomID(t, client)
			BytesID(t, client)
			// lib/pq does not accept the array parameters of batch updates; use the native driver.
			native, err := pg.Open(context.Background(), dsn+" search_path=custom_id")
			require.NoError(t, err)
			defer native.Close()
			UpdateBulk(t, ent.NewClient(ent.Driver(native)))
		})
	}
}

func TestSQLite(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.Schema.Create(context.Background(), schema.WithHooks(clearDefault)))
	CustomID(t, client)
	BytesID(t, client)
	UpdateBulk(t, client)
}

func CustomID(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	nat := client.User.Create().SaveX(ctx)
	require.Equal(t, 1, nat.ID)
	_, err := client.User.Create().Set(user.ID, 1).Save(ctx)
	require.True(t, ent.IsConstraintError(err), "duplicate id")
	a8m := client.User.Create().Set(user.ID, 5).SaveX(ctx)
	require.Equal(t, 5, a8m.ID)

	hub := client.Group.Create().Set(group.ID, 3).AddIDs(group.Users, a8m.ID, nat.ID).SaveX(ctx)
	require.Equal(t, 3, hub.ID)
	require.Equal(t, []int{1, 5}, hub.QueryUsers().Order(user.ID.Asc()).IDsX(ctx))

	blb := client.Blob.Create().SaveX(ctx)
	require.NotEmpty(t, blb.ID, "use default value")
	id := uuid.New()
	chd := client.Blob.Create().Set(blob.ID, id).SetEdge(blob.Parent, blb.ID).SaveX(ctx)
	require.Equal(t, id, chd.ID, "use provided id")
	require.Equal(t, blb.ID, chd.QueryParent().OnlyX(ctx).ID)
	lnk := client.Blob.Create().Set(blob.ID, uuid.New()).AddIDs(blob.Links, chd.ID, blb.ID).SaveX(ctx)
	require.Equal(t, 2, lnk.QueryLinks().CountX(ctx))
	require.Equal(t, lnk.ID, chd.QueryLinks().OnlyX(ctx).ID)
	require.Equal(t, lnk.ID, blb.QueryLinks().OnlyX(ctx).ID)
	require.Len(t, client.Blob.Query().IDsX(ctx), 3)
	links := lnk.QueryBlobLinks().AllX(ctx)
	require.Len(t, links, 2)
	require.Equal(t, lnk.ID, links[0].BlobID)
	require.NotEqual(t, uuid.Nil, links[0].LinkID)
	require.Equal(t, lnk.ID, links[1].BlobID)
	require.NotEqual(t, uuid.Nil, links[1].LinkID)

	pedro := client.Pet.Create().Set(pet.ID, "pedro").SetEdge(pet.Owner, a8m.ID).SaveX(ctx)
	require.Equal(t, a8m.ID, pedro.QueryOwner().OnlyIDX(ctx))
	require.Equal(t, pedro.ID, a8m.QueryPets().OnlyIDX(ctx))
	xabi := client.Pet.Create().Set(pet.ID, "xabi").AddIDs(pet.Friends, pedro.ID).SetEdge(pet.BestFriend, pedro.ID).SaveX(ctx)
	require.Equal(t, "xabi", xabi.ID)
	pedro = client.Pet.Query().Where(pet.Owner.HasWith(user.ID.EQ(a8m.ID))).OnlyX(ctx)
	require.Equal(t, "pedro", pedro.ID)

	pets := client.Pet.Query().WithFriends().WithBestFriend().Order(pet.ID.Asc()).AllX(ctx)
	require.Len(t, pets, 2)

	require.Equal(t, pedro.ID, pets[0].ID)
	require.NotNil(t, pets[0].Edges.BestFriend)
	require.Equal(t, xabi.ID, pets[0].Edges.BestFriend.ID)
	require.Len(t, pets[0].Edges.Friends, 1)
	require.Equal(t, xabi.ID, pets[0].Edges.Friends[0].ID)

	require.Equal(t, xabi.ID, pets[1].ID)
	require.NotNil(t, pets[1].Edges.BestFriend)
	require.Equal(t, pedro.ID, pets[1].Edges.BestFriend.ID)
	require.Len(t, pets[1].Edges.Friends, 1)
	require.Equal(t, pedro.ID, pets[1].Edges.Friends[0].ID)

	bee := client.Car.Create().Set(car.Model, "Chevrolet Camaro").SetEdge(car.Owner, pedro.ID).SaveX(ctx)
	require.NotNil(t, bee)
	bee = client.Car.Query().WithOwner().OnlyX(ctx)
	require.Equal(t, "Chevrolet Camaro", bee.Model)
	require.NotNil(t, bee.Edges.Owner)
	require.Equal(t, pedro.ID, bee.Edges.Owner.ID)

	pets = client.Pet.CreateBulk(
		client.Pet.Create().Set(pet.ID, "luna").SetEdge(pet.Owner, a8m.ID).AddIDs(pet.Friends, xabi.ID),
		client.Pet.Create().Set(pet.ID, "layla").SetEdge(pet.Owner, a8m.ID).AddIDs(pet.Friends, pedro.ID),
		client.Pet.Create().AddIDs(pet.Friends, pedro.ID, xabi.ID),
	).SaveX(ctx)
	require.Equal(t, "luna", pets[0].ID)
	require.Equal(t, xabi.ID, pets[0].QueryFriends().OnlyIDX(ctx))
	require.Equal(t, "layla", pets[1].ID)
	require.Equal(t, pedro.ID, pets[1].QueryFriends().OnlyIDX(ctx))
	require.Equal(t, []string{"pedro", "xabi"}, pets[2].QueryFriends().Order(pet.ID.Asc()).IDsX(ctx))

	u1, u2 := uuid.New(), uuid.New()
	blobs := client.Blob.CreateBulk(
		client.Blob.Create().Set(blob.ID, u1),
		client.Blob.Create().Set(blob.ID, u2),
	).SaveX(ctx)
	require.Equal(t, u1, blobs[0].ID)
	require.Equal(t, u2, blobs[1].ID)

	parent := client.Note.Create().Set(note.Text, "parent").SaveX(ctx)
	require.NotEmpty(t, parent.ID)
	require.NotEmpty(t, parent.Text)
	child := client.Note.Create().Set(note.Text, "child").SetEdge(note.Parent, parent.ID).SaveX(ctx)
	require.NotEmpty(t, child.QueryParent().OnlyIDX(ctx))

	t.Run("ValueScanner ID", func(t *testing.T) {
		id1 := entschema.ValueScanID{V: 10}
		id2 := entschema.ValueScanID{V: 20}
		id3 := entschema.ValueScanID{V: 30}
		id4 := entschema.ValueScanID{V: 40}

		client.ValueScan.Create().Set(valuescan.ID, id1).Set(valuescan.Name, "first").SaveX(ctx)
		client.ValueScan.Create().Set(valuescan.ID, id2).Set(valuescan.Name, "second").SaveX(ctx)
		require.Equal(t, id1, client.ValueScan.GetX(ctx, id1).ID)
		require.Equal(t, id2, client.ValueScan.Query().Where(valuescan.ID.EQ(id2)).OnlyX(ctx).ID)
		require.True(t, client.ValueScan.Query().Where(valuescan.ID.EQ(id1)).ExistX(ctx))
		require.False(t, client.ValueScan.Query().Where(valuescan.ID.EQ(entschema.ValueScanID{V: 999})).ExistX(ctx))

		client.ValueScan.CreateBulk(
			client.ValueScan.Create().Set(valuescan.ID, id3).Set(valuescan.Name, "third"),
			client.ValueScan.Create().Set(valuescan.ID, id4).Set(valuescan.Name, "fourth"),
		).SaveX(ctx)
		require.ElementsMatch(t, []entschema.ValueScanID{id1, id2, id3, id4}, client.ValueScan.Query().IDsX(ctx))

		client.ValueScan.UpdateOneID(id2).Set(valuescan.Name, "updated").ExecX(ctx)
		require.Equal(t, "updated", client.ValueScan.GetX(ctx, id2).Name)

		var raw []struct {
			ID int
		}
		require.NoError(t, client.ValueScan.Query().
			Where(valuescan.Name.EQ("updated")).
			Select(valuescan.ID).Scan(ctx, &raw))
		require.Len(t, raw, 1)
		require.Equal(t, 20, raw[0].ID)
	})

	pdoc := client.Doc.Create().Set(doc.Text, "parent").SaveX(ctx)
	require.NotEmpty(t, pdoc.ID)
	require.NotEmpty(t, pdoc.Text)
	cdoc := client.Doc.Create().Set(doc.Text, "child").SetEdge(doc.Parent, pdoc.ID).SaveX(ctx)
	require.NotEmpty(t, cdoc.QueryParent().OnlyIDX(ctx))

	t.Run("IntSID", func(t *testing.T) {
		root := client.IntSID.Create().SaveX(ctx)
		require.EqualValues(t, sid.ID("1"), root.ID)
		children := client.IntSID.CreateBulk(
			client.IntSID.Create().SetEdge(intsid.Parent, root.ID),
			client.IntSID.Create().SetEdge(intsid.Parent, root.ID),
		).SaveX(ctx)
		require.EqualValues(t, sid.ID("2"), children[0].ID)
		require.EqualValues(t, sid.ID("3"), children[1].ID)
		el := client.IntSID.Query().Where(intsid.ID.EQ(root.ID)).WithChildren().AllX(ctx)
		require.EqualValues(t, 1, len(el))
		require.EqualValues(t, 2, len(el[0].Edges.Children))
		cid := sid.ID("100")
		child := client.IntSID.Create().Set(intsid.ID, cid).SetEdge(intsid.Parent, root.ID).SaveX(ctx)
		require.EqualValues(t, cid, child.ID)
		require.EqualValues(t, root.ID, child.QueryParent().OnlyX(ctx).ID)
	})

	t.Run("Upsert", func(t *testing.T) {
		id := uuid.New()
		client.Blob.Create().
			Set(blob.ID, id).
			OnConflict(blob.ID).
			UpdateNewValues().
			ExecX(ctx)
		require.Zero(t, client.Blob.GetX(ctx, id).Count)
		client.Blob.Create().
			Set(blob.ID, id).
			OnConflict(blob.ID).
			DoUpdate(func(set *ent.BlobUpsert) {
				set.Add(blob.Count, 1)
			}).
			ExecX(ctx)
		require.Equal(t, 1, client.Blob.GetX(ctx, id).Count)

		d := client.Doc.Create().SaveX(ctx)
		client.Doc.Create().
			Set(doc.ID, d.ID).
			OnConflict(doc.ID).
			DoUpdate(func(update *ent.DocUpsert) { update.Set(doc.Text, "Hello World") }).
			UpdateNewValues().
			ExecX(ctx)
		require.Equal(t, "Hello World", client.Doc.GetX(ctx, d.ID).Text)
	})

	t.Run("Other ID", func(t *testing.T) {
		o := client.Other.Create().SaveX(ctx)
		require.NotEmpty(t, o.ID.String())

		o = client.Other.Create().Set(other.ID, sid.NewLength(15)).SaveX(ctx)
		require.NotEmpty(t, o.ID.String())
	})

	t.Run("CustomID edge", func(t *testing.T) {
		a := client.Account.Create().Set(account.Email, "test@example.org").SaveX(ctx)
		require.NotEmpty(t, a.ID)

		tk := client.Token.Create().SetEdge(token.Account, a.ID).Set(token.Body, "token").SaveX(ctx)
		require.NotEmpty(t, tk.ID)

		ta := client.Token.Query().Where(token.Body.EQ("token")).WithAccount().FirstX(ctx)
		require.Equal(t, tk.ID, ta.ID)
		require.NotNil(t, ta.Edges.Account)
		require.Equal(t, a.ID, ta.Edges.Account.ID)
	})

	t.Run("UUID compatible", func(t *testing.T) {
		l := client.Link.Create().SaveX(ctx)
		require.NotEmpty(t, l.ID)
		require.Len(t, l.LinkInformation, 1)
		require.Equal(t, "ent", l.LinkInformation["ent"].Name)
		require.Equal(t, "https://entgo.io/", l.LinkInformation["ent"].Link)
	})
}

func BytesID(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	s := client.Session.Create().SaveX(ctx)
	require.NotEmpty(t, s.ID)
	client.Device.Create().SetEdge(device.ActiveSession, s.ID).AddIDs(device.Sessions, s.ID).SaveX(ctx)
	d := client.Device.Query().WithActiveSession().WithSessions().OnlyX(ctx)
	require.Equal(t, s.ID, d.Edges.ActiveSession.ID)
	require.Equal(t, s.ID, d.Edges.Sessions[0].ID)
}

func UpdateBulk(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	t.Run("UUID", func(t *testing.T) {
		blobs := client.Blob.CreateBulk(client.Blob.Create(), client.Blob.Create()).SaveX(ctx)
		updated := client.Blob.UpdateBulk(
			client.Blob.UpdateOne(blobs[1]).Set(blob.Count, 5),
			client.Blob.UpdateOne(blobs[0]).Add(blob.Count, 2),
		).SaveX(ctx)
		require.Equal(t, []uuid.UUID{blobs[1].ID, blobs[0].ID}, []uuid.UUID{updated[0].ID, updated[1].ID})
		require.Equal(t, 5, updated[0].Count)
		require.Equal(t, 2, updated[1].Count)
		require.Equal(t, 5, client.Blob.GetX(ctx, blobs[1].ID).Count)
		require.Equal(t, 2, client.Blob.GetX(ctx, blobs[0].ID).Count)
		_, err := client.Blob.UpdateBulk(
			client.Blob.UpdateOne(blobs[0]).Set(blob.Count, 100),
			client.Blob.UpdateOneID(uuid.New()).Set(blob.Count, 100),
		).Save(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		require.Equal(t, 2, client.Blob.GetX(ctx, blobs[0].ID).Count)
	})

	t.Run("NonCanonicalID", func(t *testing.T) {
		ids := []entschema.DocID{entschema.DocID(strings.ToUpper(uuid.NewString())), entschema.DocID(strings.ReplaceAll(uuid.NewString(), "-", ""))}
		for _, id := range ids {
			client.Doc.Create().Set(doc.ID, id).Set(doc.Text, "created").ExecX(ctx)
		}
		updated := client.Doc.UpdateBulk(
			client.Doc.UpdateOneID(ids[1]).Set(doc.Text, "bulk"),
			client.Doc.UpdateOneID(ids[0]).Set(doc.Text, "bulk"),
		).SaveX(ctx)
		for index, id := range []entschema.DocID{ids[1], ids[0]} {
			require.Equal(t, client.Doc.UpdateOneID(id).Set(doc.Text, "bulk").SaveX(ctx).String(), updated[index].String())
		}
	})

	t.Run("Other", func(t *testing.T) {
		accounts := client.Account.CreateBulk(
			client.Account.Create().Set(account.Email, "first@example.org"),
			client.Account.Create().Set(account.Email, "second@example.org"),
		).SaveX(ctx)
		updated := client.Account.UpdateBulk(
			client.Account.UpdateOne(accounts[0]).Set(account.Email, "first-updated@example.org"),
			client.Account.UpdateOne(accounts[1]).Set(account.Email, "second-updated@example.org"),
		).SaveX(ctx)
		require.Equal(t, accounts[0].ID, updated[0].ID)
		require.Equal(t, "first-updated@example.org", updated[0].Email)
		require.Equal(t, accounts[1].ID, updated[1].ID)
		require.Equal(t, "second-updated@example.org", updated[1].Email)
		require.Equal(t, "second-updated@example.org", client.Account.GetX(ctx, accounts[1].ID).Email)
	})

	t.Run("ValueScanner", func(t *testing.T) {
		first, second := entschema.ValueScanID{V: 50}, entschema.ValueScanID{V: 60}
		client.ValueScan.CreateBulk(
			client.ValueScan.Create().Set(valuescan.ID, first).Set(valuescan.Name, "first"),
			client.ValueScan.Create().Set(valuescan.ID, second).Set(valuescan.Name, "second"),
		).ExecX(ctx)
		updated := client.ValueScan.UpdateBulk(
			client.ValueScan.UpdateOneID(second).Set(valuescan.Name, "second-updated"),
			client.ValueScan.UpdateOneID(first).Set(valuescan.Name, "first-updated"),
		).SaveX(ctx)
		require.Equal(t, second, updated[0].ID)
		require.Equal(t, "second-updated", updated[0].Name)
		require.Equal(t, first, updated[1].ID)
		require.Equal(t, "first-updated", updated[1].Name)
		require.Equal(t, "first-updated", client.ValueScan.GetX(ctx, first).Name)
		err := client.ValueScan.UpdateBulk(
			client.ValueScan.UpdateOneID(first).Set(valuescan.Name, "changed"),
			client.ValueScan.UpdateOneID(entschema.ValueScanID{V: 999}).Set(valuescan.Name, "changed"),
		).Exec(ctx)
		require.True(t, ent.IsNotFound(err), "got %v", err)
		require.Equal(t, "first-updated", client.ValueScan.GetX(ctx, first).Name)
	})

	t.Run("Bytes", func(t *testing.T) {
		sessions := client.Session.CreateBulk(client.Session.Create(), client.Session.Create()).SaveX(ctx)
		devices := client.Device.CreateBulk(
			client.Device.Create().SetEdge(device.ActiveSession, sessions[0].ID),
			client.Device.Create(),
		).SaveX(ctx)
		updated := client.Device.UpdateBulk(
			client.Device.UpdateOne(devices[1]).SetEdge(device.ActiveSession, sessions[1].ID),
			client.Device.UpdateOne(devices[0]).ClearEdge(device.ActiveSession),
		).SaveX(ctx)
		require.Equal(t, devices[1].ID, updated[0].ID)
		require.Equal(t, devices[0].ID, updated[1].ID)
		require.Equal(t, sessions[1].ID, devices[1].QueryActiveSession().OnlyIDX(ctx))
		require.False(t, devices[0].QueryActiveSession().ExistX(ctx))
	})
}

func TestStringIDPredicates(t *testing.T) {
	client, err := ent.Open("sqlite3", "file:string-id-predicates?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	defer client.Close()
	require.NoError(t, client.Schema.Create(t.Context(), schema.WithHooks(clearDefault)))

	client.Revision.CreateBulk(
		client.Revision.Create().Set(revision.ID, "alpha"),
		client.Revision.Create().Set(revision.ID, "Bravo"),
		client.Revision.Create().Set(revision.ID, "charlie"),
	).ExecX(t.Context())

	require.Equal(t, []string{"alpha", "charlie"}, client.Revision.Query().Where(revision.ID.GT("Bravo")).Order(revision.ID.Asc()).IDsX(t.Context()))
	require.Equal(t, []string{"Bravo", "alpha", "charlie"}, client.Revision.Query().Where(revision.ID.GTE("Bravo")).Order(revision.ID.Asc()).IDsX(t.Context()))
	require.Equal(t, []string{"Bravo"}, client.Revision.Query().Where(revision.ID.LT("alpha")).Order(revision.ID.Asc()).IDsX(t.Context()))
	require.Equal(t, []string{"Bravo", "alpha"}, client.Revision.Query().Where(revision.ID.LTE("alpha")).Order(revision.ID.Asc()).IDsX(t.Context()))
	require.Equal(t, "Bravo", client.Revision.Query().Where(revision.ID.EqualFold("bravo")).OnlyIDX(t.Context()))
	require.Equal(t, "charlie", client.Revision.Query().Where(revision.ID.ContainsFold("HARL")).OnlyIDX(t.Context()))
}

// clearDefault clears the id's default for non-postgres dialects.
func clearDefault(c schema.Creator) schema.Creator {
	return schema.CreateFunc(func(ctx context.Context, tables ...*schema.Table) error {
		// Drop DEFAULT clause for non-postgres dialects without changing the tables.
		copiedTables, err := schema.CopyTables(tables)
		if err != nil {
			return err
		}
		copiedTables[1].Columns[0].Default = nil
		return c.Create(ctx, copiedTables...)
	})
}

// expectOnePetsIndex expects that pets table contains only one index.
func expectOnePetsIndex(next schema.Differ) schema.Differ {
	return schema.DiffFunc(func(current, desired *atlas.Schema) ([]atlas.Change, error) {
		changes, err := next.Diff(current, desired)
		for _, c := range changes {
			addT, ok := c.(*atlas.AddTable)
			if !ok || addT.T.Name != pet.Table {
				continue
			}
			if n := len(addT.T.Indexes); n != 1 {
				return nil, fmt.Errorf("expect only one index, but got: %d", n)
			}
		}
		return changes, err
	})
}
