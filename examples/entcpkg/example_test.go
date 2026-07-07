// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/neko-sc/ent/examples/entcpkg/ent"
	"github.com/neko-sc/ent/examples/entcpkg/ent/hook"

	_ "github.com/mattn/go-sqlite3"
)

func Example_entcPkg() {
	client, err := ent.Open(
		"sqlite3",
		"file:ent?mode=memory&cache=shared&_fk=1",
		ent.Writer(os.Stdout),
		ent.HTTPClient(http.DefaultClient),
	)
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()
	// An example for using the injected dependencies in the generated builders.
	client.User.Use(func(next ent.Mutator) ent.Mutator {
		return hook.UserFunc(func(ctx context.Context, m *ent.UserMutation) (ent.Value, error) {
			_ = m.HTTPClient
			_ = m.Writer
			return next.Mutate(ctx, m)
		})
	})
	ctx := context.Background()
	// Run the auto migration tool.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	usr := client.User.Create().SaveX(ctx)
	fmt.Println("boring user:", usr)

	// Output: boring user: User(id=1, name=, age=0)
}
