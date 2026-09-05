// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/neko-sc/ent/examples/m2mrecur/ent/user"

	"github.com/neko-sc/ent/examples/m2mrecur/ent"

	_ "github.com/mattn/go-sqlite3"
)

func Example_m2mRecur() {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	// Run the auto migration tool.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	if err := Do(ctx, client); err != nil {
		log.Fatal(err)
	}
	// Output:
	// [User(id=2, age=28, name=nati)]
	// []
	// []
	// [User(id=1, age=30, name=a8m)]
	// [28]
	// [a8m]
}

func Do(ctx context.Context, client *ent.Client) error {
	// Unlike `Save`, `SaveX` panics if an error occurs.
	a8m := client.User.
		Create().
		Set(user.Age, 30).
		Set(user.Name, "a8m").
		SaveX(ctx)
	nati := client.User.
		Create().
		Set(user.Age, 28).
		Set(user.Name, "nati").
		AddIDs(user.Followers, a8m.ID).
		SaveX(ctx)

	// Query following/followers:

	flw := a8m.QueryFollowing().AllX(ctx)
	fmt.Println(flw)
	// Output: [User(id=2, age=28, name=nati)]

	flr := a8m.QueryFollowers().AllX(ctx)
	fmt.Println(flr)
	// Output: []

	flw = nati.QueryFollowing().AllX(ctx)
	fmt.Println(flw)
	// Output: []

	flr = nati.QueryFollowers().AllX(ctx)
	fmt.Println(flr)
	// Output: [User(id=1, age=30, name=a8m)]

	// Traverse the graph:

	ages, projectionError := ent.Values(ctx, nati.
		QueryFollowers(). // [a8m]
		QueryFollowing(). // [nati]
		GroupBy(user.Age), user.Age)
	if projectionError != nil {
		return projectionError
	}
	fmt.Println(ages)
	// Output: [28]

	names, projectionError := ent.Values(ctx, client.User.
		Query().
		Where(user.Not(user.Followers.Has())).
		GroupBy(user.Name), user.Name)
	if projectionError != nil {
		return projectionError
	}
	fmt.Println(names)
	// Output: [a8m]
	return nil
}
