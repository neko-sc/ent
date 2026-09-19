// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/cache"
	generated "github.com/neko-sc/ent/cache/internal/testschema"
	"github.com/neko-sc/ent/cache/internal/testschema/group"
	"github.com/neko-sc/ent/cache/internal/testschema/pet"
	"github.com/neko-sc/ent/cache/internal/testschema/schema"
	"github.com/neko-sc/ent/cache/internal/testschema/user"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/dialect/sqlite"
	"github.com/stretchr/testify/require"
)

func newClient(t *testing.T, options ...cache.Option) (*generated.Client, *cache.Driver) {
	t.Helper()
	driver, err := sqlite.Open(t.Context(), ":memory:")
	require.NoError(t, err)
	cached, err := cache.New(driver, append([]cache.Option{
		cache.Levels(cache.NewMemory(1000, 0)), cache.TTL(time.Minute),
	}, options...)...)
	require.NoError(t, err)
	client := generated.NewClient(generated.Driver(cached))
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.NoError(t, client.Schema.Create(t.Context()))
	return client, cached
}

func createUser(t *testing.T, client *generated.Client, email string) *generated.User {
	t.Helper()
	created, err := client.User.Create().
		Set(user.Name, "Ada").Set(user.Email, email).Set(user.Age, 30).
		Set(user.Role, user.RoleAdmin).Set(user.Tags, []string{"go", "sqlite"}).
		Set(user.Profile, schema.Profile{Biography: "Engineer", Links: []string{"https://example.com"}}).
		Set(user.Avatar, []byte{1, 2, 3}).Save(t.Context())
	require.NoError(t, err)
	return created
}

func requireUser(t *testing.T, expected, actual *generated.User) {
	t.Helper()
	require.Equal(t, expected.ID, actual.ID)
	require.Equal(t, expected.Name, actual.Name)
	require.Equal(t, expected.Email, actual.Email)
	require.Equal(t, expected.Age, actual.Age)
	require.Equal(t, expected.Nickname, actual.Nickname)
	require.Equal(t, expected.Role, actual.Role)
	require.Equal(t, expected.Tags, actual.Tags)
	require.Equal(t, expected.Profile, actual.Profile)
	require.Equal(t, expected.CreatedAt.UTC().Round(0), actual.CreatedAt.UTC().Round(0))
	require.Equal(t, expected.Avatar, actual.Avatar)
}

func TestGeneratedClient(t *testing.T) {
	t.Run("get by ID", func(t *testing.T) {
		client, cached := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))

		t.Run("first read fills", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			found, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			requireUser(t, created, found)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.Zero(t, after.Hits["memory"]-before.Hits["memory"])
			require.False(t, info.Hit)
			require.True(t, info.PointRead)
		})

		t.Run("second read serves from cache", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			found, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			requireUser(t, created, found)
			after := cached.Stats().Snapshot()
			require.Zero(t, after.Fills-before.Fills)
			require.Positive(t, after.Hits["memory"]-before.Hits["memory"])
			require.True(t, info.Hit)
		})

		t.Run("cached values are copied per read", func(t *testing.T) {
			mutated, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			mutated.Avatar[0] = 99
			mutated.Tags[0] = "changed"
			mutated.Profile.Links[0] = "changed"
			found, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			require.True(t, info.Hit)
			requireUser(t, created, found)
		})

		t.Run("update invalidates", func(t *testing.T) {
			updated, err := client.User.UpdateOneID(created.ID).
				Set(user.Nickname, "Ace").Set(user.Role, user.RoleMember).Save(t.Context())
			require.NoError(t, err)
			found, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			require.False(t, info.Hit)
			requireUser(t, updated, found)

			repeat, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			require.True(t, info.Hit)
			requireUser(t, updated, repeat)
			*repeat.Nickname = "changed"
			found, err = client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			requireUser(t, updated, found)
		})
	})

	t.Run("unique email", func(t *testing.T) {
		client, _ := newClient(t)
		created := createUser(t, client, "ada@example.com")
		other := createUser(t, client, "other@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func() *generated.User {
			found, err := client.User.Query().Where(user.Email.EQ(created.Email)).Only(ctx)
			require.NoError(t, err)
			require.True(t, info.PointRead)
			return found
		}
		requireUser(t, created, query())
		require.False(t, info.Hit)
		query()
		require.True(t, info.Hit)
		require.NoError(t, client.User.UpdateOneID(created.ID).Set(user.Name, "Grace").Exec(t.Context()))
		require.Equal(t, "Grace", query().Name)
		require.False(t, info.Hit)
		query()
		require.True(t, info.Hit)
		require.NoError(t, client.User.UpdateOneID(other.ID).Set(user.Name, "Other").Exec(t.Context()))
		query()
		require.True(t, info.Hit)

		_, err := client.User.Query().Where(user.Email.EQ("missing@example.com")).Only(ctx)
		require.True(t, generated.IsNotFound(err))
		require.False(t, info.Hit)
		_, err = client.User.Query().Where(user.Email.EQ("missing@example.com")).Only(ctx)
		require.True(t, generated.IsNotFound(err))
		require.True(t, info.Hit)
		missing := createUser(t, client, "missing@example.com")
		found, err := client.User.Query().Where(user.Email.EQ(missing.Email)).Only(ctx)
		require.NoError(t, err)
		require.False(t, info.Hit)
		requireUser(t, missing, found)
	})

	t.Run("list", func(t *testing.T) {
		client, _ := newClient(t)
		createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func() []*generated.User {
			users, err := client.User.Query().Where(user.Age.GT(10)).All(ctx)
			require.NoError(t, err)
			return users
		}
		require.Len(t, query(), 1)
		require.False(t, info.Hit)
		require.Len(t, query(), 1)
		require.True(t, info.Hit)
		createUser(t, client, "other@example.com")
		require.Len(t, query(), 2)
		require.False(t, info.Hit)
	})

	t.Run("eager load", func(t *testing.T) {
		client, cached := newClient(t)
		created := createUser(t, client, "ada@example.com")
		require.NoError(t, client.Pet.Create().Set(pet.Name, "Cat").SetEdge(pet.Owner, created.ID).Exec(t.Context()))
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func(pets int) {
			users, err := client.User.Query().WithPets().All(ctx)
			require.NoError(t, err)
			require.Len(t, users[0].Edges.Pets, pets)
		}

		t.Run("first read fills", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.Zero(t, after.Hits["memory"]-before.Hits["memory"])
			require.False(t, info.Hit)
		})

		t.Run("second read serves from cache", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Zero(t, after.Fills-before.Fills)
			require.Positive(t, after.Hits["memory"]-before.Hits["memory"])
			require.True(t, info.Hit)
		})

		t.Run("edge write invalidates", func(t *testing.T) {
			require.NoError(t, client.Pet.Create().Set(pet.Name, "Dog").SetEdge(pet.Owner, created.ID).Exec(t.Context()))
			before := cached.Stats().Snapshot()
			query(2)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.False(t, info.Hit)
			require.Equal(t, []string{"pets"}, info.Tables)
		})
	})

	t.Run("edge counts", func(t *testing.T) {
		client, cached := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func(expected int) {
			users, err := client.User.Query().WithCount(user.Pets).All(ctx)
			require.NoError(t, err)
			require.Len(t, users, 1)
			count, loaded := users[0].Edges.Count(user.Pets)
			require.True(t, loaded)
			require.Equal(t, expected, count)
		}

		t.Run("first read fills", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(0)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.Zero(t, after.Hits["memory"]-before.Hits["memory"])
			require.False(t, info.Hit)
		})

		t.Run("second read serves from cache", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(0)
			after := cached.Stats().Snapshot()
			require.Zero(t, after.Fills-before.Fills)
			require.Positive(t, after.Hits["memory"]-before.Hits["memory"])
			require.True(t, info.Hit)
		})

		t.Run("edge write invalidates the count", func(t *testing.T) {
			require.NoError(t, client.Pet.Create().Set(pet.Name, "Cat").SetEdge(pet.Owner, created.ID).Exec(t.Context()))
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.False(t, info.Hit)
			query(1)
			require.True(t, info.Hit)
		})
	})

	t.Run("filtered edge counts", func(t *testing.T) {
		client, cached := newClient(t)
		member := createUser(t, client, "ada@example.com")
		require.NoError(t, client.Group.Create().Set(group.Name, "Team").AddIDs(group.Users, member.ID).Exec(t.Context()))
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func(expected int) {
			groups, err := client.Group.Query().WithCount(group.Users, user.Name.EQ("Ada")).All(ctx)
			require.NoError(t, err)
			require.Len(t, groups, 1)
			count, loaded := groups[0].Edges.Count(group.Users)
			require.True(t, loaded)
			require.Equal(t, expected, count)
		}

		t.Run("first read fills and tracks the neighbor table", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.False(t, info.Hit)
			require.Contains(t, info.Tables, user.Table)
		})

		t.Run("second read serves from cache", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Zero(t, after.Fills-before.Fills)
			require.Positive(t, after.Hits["memory"]-before.Hits["memory"])
			require.True(t, info.Hit)
		})

		t.Run("neighbor write invalidates the count", func(t *testing.T) {
			require.NoError(t, client.User.UpdateOneID(member.ID).Set(user.Name, "Grace").Exec(t.Context()))
			before := cached.Stats().Snapshot()
			query(0)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.False(t, info.Hit)
			query(0)
			require.True(t, info.Hit)
		})
	})

	t.Run("many to many", func(t *testing.T) {
		client, cached := newClient(t)
		first := createUser(t, client, "ada@example.com")
		second := createUser(t, client, "other@example.com")
		created, err := client.Group.Create().Set(group.Name, "Team").AddIDs(group.Users, first.ID).Save(t.Context())
		require.NoError(t, err)
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func(users int) {
			groups, err := client.Group.Query().WithUsers().All(ctx)
			require.NoError(t, err)
			require.Len(t, groups[0].Edges.Users, users)
		}

		t.Run("first read fills and tracks the join table", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.Zero(t, after.Hits["memory"]-before.Hits["memory"])
			require.Contains(t, info.Tables, group.UsersTable)
		})

		t.Run("second read serves from cache", func(t *testing.T) {
			before := cached.Stats().Snapshot()
			query(1)
			after := cached.Stats().Snapshot()
			require.Zero(t, after.Fills-before.Fills)
			require.Positive(t, after.Hits["memory"]-before.Hits["memory"])
			require.True(t, info.Hit)
		})

		t.Run("join table write invalidates", func(t *testing.T) {
			require.NoError(t, created.Update().AddIDs(group.Users, second.ID).Exec(t.Context()))
			before := cached.Stats().Snapshot()
			query(2)
			after := cached.Stats().Snapshot()
			require.Positive(t, after.Fills-before.Fills)
			require.False(t, info.Hit)
			require.Positive(t, cached.Stats().Snapshot().Bumps[group.UsersTable])
		})
	})

	t.Run("select and group by", func(t *testing.T) {
		client, _ := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		query := func() int {
			rows, err := client.User.Query().Select(user.Age).Rows(ctx)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			return ent.Get(rows[0], user.Age)
		}
		require.Equal(t, 30, query())
		require.False(t, info.Hit)
		require.Equal(t, 30, query())
		require.True(t, info.Hit)
		var ages []int
		require.NoError(t, client.User.Query().Select(user.Age).Scan(ctx, &ages))
		require.Equal(t, []int{30}, ages)
		require.True(t, info.Hit)
		updated, err := client.User.Update().Where(user.ID.EQ(created.ID)).Set(user.Age, 31).Returning(t.Context())
		require.NoError(t, err)
		require.Len(t, updated, 1)
		require.Equal(t, 31, updated[0].Age)
		require.Equal(t, 31, query())
		require.False(t, info.Hit)

		count := ent.Count().As("total")
		aggregate := func() int {
			rows, err := client.User.Query().GroupBy(user.Role).Aggregate(count).Rows(ctx)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, user.RoleAdmin, ent.Get(rows[0], user.Role))
			return ent.Get(rows[0], count)
		}
		require.Equal(t, 1, aggregate())
		require.False(t, info.Hit)
		require.Equal(t, 1, aggregate())
		require.True(t, info.Hit)
		createUser(t, client, "other@example.com")
		require.Equal(t, 2, aggregate())
		require.False(t, info.Hit)
	})

	t.Run("count and exist", func(t *testing.T) {
		client, _ := newClient(t)
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))

		t.Run("count", func(t *testing.T) {
			for iteration := range 2 {
				count, err := client.User.Query().Count(ctx)
				require.NoError(t, err)
				require.Zero(t, count)
				require.Equal(t, iteration > 0, info.Hit)
			}
		})

		t.Run("exist", func(t *testing.T) {
			for iteration := range 2 {
				exists, err := client.User.Query().Exist(ctx)
				require.NoError(t, err)
				require.False(t, exists)
				require.Equal(t, iteration > 0, info.Hit)
			}
		})

		t.Run("write invalidates both", func(t *testing.T) {
			createUser(t, client, "ada@example.com")
			count, err := client.User.Query().Count(ctx)
			require.NoError(t, err)
			require.Equal(t, 1, count)
			require.False(t, info.Hit)
			exists, err := client.User.Query().Exist(ctx)
			require.NoError(t, err)
			require.True(t, exists)
			require.False(t, info.Hit)
		})
	})

	t.Run("locking reads bypass the cache", func(t *testing.T) {
		client, cached := newClient(t)
		createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		// SQLite rejects row locks while rendering, so the query never reaches the
		// cache driver. Either way no entry may be stored for a locking read.
		if _, err := client.User.Query().ForUpdate().All(ctx); err != nil {
			require.ErrorContains(t, err, "FOR UPDATE/SHARE not supported in SQLite")
		} else {
			require.Equal(t, cache.BypassLock, info.Bypass)
		}
		require.Zero(t, cached.Stats().Snapshot().Fills)
		require.False(t, info.Hit)
	})

	t.Run("transaction", func(t *testing.T) {
		client, _ := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		_, err := client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		transaction, err := client.Tx(ctx)
		require.NoError(t, err)
		inside, err := transaction.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, created.Name, inside.Name)
		require.Equal(t, cache.BypassTx, info.Bypass)
		require.NoError(t, transaction.User.UpdateOneID(created.ID).Set(user.Name, "Committed").Exec(ctx))
		require.NoError(t, transaction.Commit())
		found, err := client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, "Committed", found.Name)
		require.False(t, info.Hit)
		transaction, err = client.Tx(ctx)
		require.NoError(t, err)
		require.NoError(t, transaction.User.UpdateOneID(created.ID).Set(user.Name, "Rolled back").Exec(ctx))
		require.NoError(t, transaction.Rollback())
		found, err = client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, "Committed", found.Name)
		require.True(t, info.Hit)
	})

	t.Run("explicit and global modes", func(t *testing.T) {
		client, cached := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(t.Context())
		for range 2 {
			_, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			require.Equal(t, cache.BypassDisabled, info.Bypass)
		}
		require.Zero(t, cached.Stats().Snapshot().Fills)
		for range 2 {
			_, err := client.User.Get(cache.Cache(ctx), created.ID)
			require.NoError(t, err)
		}
		require.True(t, info.Hit)

		global, cached := newClient(t, cache.Global(true))
		created = createUser(t, global, "global@example.com")
		for range 2 {
			_, err := global.User.Get(ctx, created.ID)
			require.NoError(t, err)
		}
		require.True(t, info.Hit)
		_, err := global.User.Get(cache.Skip(ctx), created.ID)
		require.NoError(t, err)
		require.Equal(t, cache.BypassSkip, info.Bypass)
		require.Equal(t, uint64(1), cached.Stats().Snapshot().Fills)
	})

	t.Run("TTL only", func(t *testing.T) {
		client, _ := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context(), cache.TTLOnly()))
		_, err := client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.NoError(t, client.User.UpdateOneID(created.ID).Set(user.Name, "Changed").Exec(t.Context()))
		found, err := client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.True(t, info.Hit)
		require.Equal(t, created.Name, found.Name)
		fresh, err := client.User.Get(t.Context(), created.ID)
		require.NoError(t, err)
		require.Equal(t, "Changed", fresh.Name)
	})

	t.Run("depends on", func(t *testing.T) {
		client, _ := newClient(t)
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(t.Context(), cache.DependsOn(pet.Table)))
		query := func() []int {
			var identifiers []int
			require.NoError(t, client.User.Query().Select(user.ID).Modify(func(selector *sql.Selector) {
				selector.Where(sql.ExprP("EXISTS (SELECT 1 FROM pets WHERE pets.user_pets = users.id)"))
			}).Scan(ctx, &identifiers))
			return identifiers
		}
		require.Empty(t, query())
		require.False(t, info.Hit)
		require.Empty(t, query())
		require.True(t, info.Hit)
		require.Contains(t, info.Tables, pet.Table)
		require.NoError(t, client.Pet.Create().Set(pet.Name, "Cat").SetEdge(pet.Owner, created.ID).Exec(t.Context()))
		require.Equal(t, []int{created.ID}, query())
		require.False(t, info.Hit)
	})

	t.Run("raw SQL", func(t *testing.T) {
		client, cached := newClient(t)
		created := createUser(t, client, "ada@example.com")
		require.NoError(t, client.Group.Create().Set(group.Name, "Team").Exec(t.Context()))
		ctx, info := cache.WithInfo(cache.Cache(t.Context()))
		for range 2 {
			_, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
			_, err = client.Group.Query().All(ctx)
			require.NoError(t, err)
		}
		require.True(t, info.Hit)
		_, err := cached.Exec(t.Context(), "UPDATE users SET age = age + 1", nil)
		require.NoError(t, err)
		found, err := client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, 31, found.Age)
		require.False(t, info.Hit)
		_, err = client.Group.Query().All(ctx)
		require.NoError(t, err)
		require.False(t, info.Hit)
		_, err = cached.Exec(cache.Touches(t.Context(), user.Table), "UPDATE users SET age = age + 1", nil)
		require.NoError(t, err)
		found, err = client.User.Get(ctx, created.ID)
		require.NoError(t, err)
		require.Equal(t, 32, found.Age)
		require.False(t, info.Hit)
		_, err = client.Group.Query().All(ctx)
		require.NoError(t, err)
		require.True(t, info.Hit)
	})

	t.Run("context level", func(t *testing.T) {
		client, cached := newClient(t, cache.Levels(cache.ContextLevel(), cache.NewMemory(1000, 0)))
		created := createUser(t, client, "ada@example.com")
		ctx, info := cache.WithInfo(cache.Cache(cache.NewContext(t.Context())))
		for range 2 {
			_, err := client.User.Get(ctx, created.ID)
			require.NoError(t, err)
		}
		require.True(t, info.Hit)
		require.Equal(t, "context", info.Level)
		require.Equal(t, uint64(1), cached.Stats().Snapshot().Hits["context"])
		other, otherInfo := cache.WithInfo(cache.Cache(cache.NewContext(context.Background())))
		_, err := client.User.Get(other, created.ID)
		require.NoError(t, err)
		require.Equal(t, "memory", otherInfo.Level)
	})
}
