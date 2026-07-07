// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/examples/enumtypes/ent"
	"github.com/neko-sc/ent/examples/enumtypes/ent/user"

	"ariga.io/atlas-go-sdk/atlasexec"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestEnumTypes(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip()
	}
	ctx := context.Background()
	client, err := ent.Open(dialect.Postgres, os.Getenv("DB_URL"))
	if err != nil {
		log.Fatalln(err)
	}
	ac, err := atlasexec.NewClient(".", "atlas")
	if err != nil {
		log.Fatalf("failed to initialize client: %v", err)
	}
	// Automatically update the database with the desired schema.
	// Another option, is to use 'migrate apply' or 'schema apply' manually.
	_, err = ac.SchemaApply(ctx, &atlasexec.SchemaApplyParams{
		// URL to your database. For example:
		// postgres://postgres:pass@localhost:5432/database?search_path=public&sslmode=disable
		URL:         os.Getenv("DB_URL"),
		Env:         "local",
		AutoApprove: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { client.User.Delete().ExecX(ctx) })
	client.User.Create().SetStatus(user.StatusActive).SaveX(ctx)
}
