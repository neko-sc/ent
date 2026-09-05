// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/neko-sc/ent/examples/edgeindex/ent"
	city "github.com/neko-sc/ent/examples/edgeindex/ent/city"
	street "github.com/neko-sc/ent/examples/edgeindex/ent/street"

	_ "github.com/mattn/go-sqlite3"
)

func Example_edgeIndex() {
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
}

func Do(ctx context.Context, client *ent.Client) error {
	// Unlike `Save`, `SaveX` panics if an error occurs.
	tlv := client.City.
		Create().
		Set(city.Name, "TLV").
		SaveX(ctx)
	nyc := client.City.
		Create().
		Set(city.Name, "NYC").
		SaveX(ctx)
	// Add a street "ST" to "TLV".
	client.Street.
		Create().
		Set(street.Name, "ST").
		SetEdge(street.City, tlv.ID).
		SaveX(ctx)
	// This operation fails because "ST"
	// was already created under "TLV".
	if err := client.Street.
		Create().
		Set(street.Name, "ST").
		SetEdge(street.City, tlv.ID).
		Exec(ctx); err == nil {
		return fmt.Errorf("expecting creation to fail")
	}
	// Add a street "ST" to "NYC".
	client.Street.
		Create().
		Set(street.Name, "ST").
		SetEdge(street.City, nyc.ID).
		SaveX(ctx)
	return nil
}
