// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package encryptfield

import (
	"context"
	"fmt"
	"log"

	"github.com/neko-sc/ent/examples/encryptfield/ent"
	_ "github.com/neko-sc/ent/examples/encryptfield/ent/runtime"
	"github.com/neko-sc/ent/examples/encryptfield/ent/user"

	_ "github.com/mattn/go-sqlite3"
	"gocloud.dev/secrets/localsecrets"
)

func Example_encryptField() {
	key, err := localsecrets.NewRandomKey()
	if err != nil {
		log.Fatalf("failed creating random key: %v", err)
	}
	client, err := ent.Open(
		"sqlite3",
		"file:ent?mode=memory&cache=shared&_fk=1",
		ent.SecretsKeeper(
			localsecrets.NewKeeper(key),
		),
	)
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed migrating schema: %v", err)
	}

	// Create:
	a8m := client.User.Create().SetName("Ariel").SetNickname("a8m").SaveX(ctx)
	// Name is returned decrypted, but stored encrypted.
	fmt.Println(a8m.Name)
	fmt.Println(client.User.Query().OnlyX(ctx).Name) // Only.
	fmt.Println("decrypted:", client.User.Query().Select(user.FieldName).StringX(ctx) != a8m.Name)

	// Update one:
	a8m = client.User.UpdateOne(a8m).SetName("Ariel Ma").SaveX(ctx)
	// Name is returned decrypted, but stored encrypted.
	fmt.Println(a8m.Name)
	fmt.Println(client.User.Query().AllX(ctx)[0].Name) // All.
	fmt.Println("decrypted:", client.User.Query().Select(user.FieldName).StringX(ctx) != a8m.Name)

	// Update:
	client.User.Update().SetName("Ariel Mashraki").SaveX(ctx)
	fmt.Println(client.User.Query().FirstX(ctx).Name) // First.
	fmt.Println("decrypted:", client.User.Query().Select(user.FieldName).StringX(ctx) != "Ariel Mashraki")

	// Output:
	// Ariel
	// Ariel
	// decrypted: true
	// Ariel Ma
	// Ariel Ma
	// decrypted: true
	// Ariel Mashraki
	// decrypted: true
}
