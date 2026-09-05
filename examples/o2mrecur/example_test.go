// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/neko-sc/ent/examples/o2mrecur/ent"
	"github.com/neko-sc/ent/examples/o2mrecur/ent/node"

	_ "github.com/mattn/go-sqlite3"
)

func Example_o2mRecur() {
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
	// Tree leafs [1 3 5]
	// [1 3 5]
	// Node(id=1, value=2, parent_id=0)
}

func Do(ctx context.Context, client *ent.Client) error {
	root, err := client.Node.
		Create().
		Set(node.Value, 2).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("creating the root: %w", err)
	}

	// Add additional nodes to the tree:
	//
	//       2
	//     /   \
	//    1     4
	//        /   \
	//       3     5
	//

	// Unlike `Save`, `SaveX` panics if an error occurs.
	n1 := client.Node.
		Create().
		Set(node.Value, 1).
		SetEdge(node.Parent, root.ID).
		SaveX(ctx)
	n4 := client.Node.
		Create().
		Set(node.Value, 4).
		SetEdge(node.Parent, root.ID).
		SaveX(ctx)
	n3 := client.Node.
		Create().
		Set(node.Value, 3).
		SetEdge(node.Parent, n4.ID).
		SaveX(ctx)
	n5 := client.Node.
		Create().
		Set(node.Value, 5).
		SetEdge(node.Parent, n4.ID).
		SaveX(ctx)

	fmt.Println("Tree leafs", []int{n1.Value, n3.Value, n5.Value})
	// Output: Tree leafs [1 3 5]

	// Get all leafs (nodes without children).
	ints, projectionError := ent.Values(ctx, client.Node.
		Query().                              // All nodes.
		Where(node.Not(node.Children.Has())). // Only leafs.
		Order(node.Value.Asc()).              // Order by their `value` field.
		GroupBy(node.Value), node.Value)
	if projectionError != nil {
		return projectionError
	}
	fmt.Println(ints)
	// Output: [1 3 5]

	// Get orphan nodes (nodes without parent).
	// Unlike `Only`, `OnlyX` panics if an error occurs.
	orphan := client.Node.
		Query().
		Where(node.Not(node.Parent.Has())).
		OnlyX(ctx)
	fmt.Println(orphan)
	// Output: Node(id=1, value=2, parent_id=0)

	return nil
}
