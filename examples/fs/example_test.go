// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/examples/fs/ent"
	"github.com/neko-sc/ent/examples/fs/ent/file"

	_ "github.com/mattn/go-sqlite3"
)

func Example_recursiveTraversal() {
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

	// Add multiple files in the following tree structure:
	//
	//	a/
	//	├─ b/
	//	│  ├─ ba
	//	│  ├─ bb
	//	│  └─ bc (deleted)
	//	├─ c/ (deleted)
	//	│  ├─ ca
	//	│  └─ cb
	//	└─ d (deleted)
	//
	a := client.File.Create().Set(file.Name, "a").SaveX(ctx)
	b := client.File.Create().Set(file.Name, "b").SetEdge(file.Parent, a.ID).SaveX(ctx)
	client.File.Create().Set(file.Name, "ba").SetEdge(file.Parent, b.ID).SaveX(ctx)
	client.File.Create().Set(file.Name, "bb").SetEdge(file.Parent, b.ID).SaveX(ctx)
	client.File.Create().Set(file.Name, "bc").SetEdge(file.Parent, b.ID).Set(file.Deleted, true).SaveX(ctx)
	c := client.File.Create().Set(file.Name, "c").SetEdge(file.Parent, a.ID).Set(file.Deleted, true).SaveX(ctx)
	client.File.Create().Set(file.Name, "ca").SetEdge(file.Parent, c.ID).SaveX(ctx)
	client.File.Create().Set(file.Name, "cb").SetEdge(file.Parent, c.ID).SaveX(ctx)
	client.File.Create().Set(file.Name, "d").SetEdge(file.Parent, a.ID).Set(file.Deleted, true).SaveX(ctx)

	// Query undeleted files:
	//
	//	a/
	//	└─ b/
	//	   ├─ ba
	//	   └─ bb
	//
	names, projectionError := ent.Values(ctx, client.File.Query().
		Where(func(s *sql.Selector) {
			t1, t2 := sql.Table(file.Table), sql.Table(file.Table)
			with := sql.WithRecursive("undeleted", file.FieldID, file.FieldParentID)
			with.As(
				// The initial `SELECT` statement executed once at the start,
				// and produces the initial row or rows for the recursion.
				sql.Select(t1.Columns(file.FieldID, file.FieldParentID)...).
					From(t1).
					Where(
						sql.And(
							sql.IsNull(t1.C(file.FieldParentID)),
							sql.EQ(t1.C(file.FieldDeleted), false),
						),
					).
					// Merge the `SELECT` statement above with the following:
					UnionAll(
						// A `SELECT` statement that produces additional rows and recurses by referring
						// to the CTE name (e.g. "undeleted"), and ends when there are no more new rows.
						sql.Select(t2.Columns(file.FieldID, file.FieldParentID)...).
							From(t2).
							Join(with).
							On(t2.C(file.FieldParentID), with.C(file.FieldID)).
							Where(
								sql.EQ(t1.C(file.FieldDeleted), false),
							),
					),
			)
			// Join the root `SELECT` query with the CTE result (`WITH` clause).
			s.Prefix(with).Join(with).On(s.C(file.FieldID), with.C(file.FieldID))
		}).
		Select(file.Name), file.Name)
	if projectionError != nil {
		panic(projectionError)
	}
	fmt.Printf("%q\n", names)

	// Output:
	// ["a" "b" "ba" "bb"]
}
