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
	ctx := context.Background()
	// Run the auto migration tool.
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}
	usr := client.User.Create().SaveX(ctx)
	fmt.Println("boring user:", usr)

	// Output: boring user: User(id=1, name=, age=0)
}
