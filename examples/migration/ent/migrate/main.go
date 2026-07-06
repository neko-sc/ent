// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build ignore

package main

import (
	"context"
	"log"
	"os"

	atlas "github.com/neko-sc/atlas/sql/migrate"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/examples/migration/ent/migrate"
)

func main() {
	ctx := context.Background()
	dir, err := atlas.NewLocalDir("ent/migrate/migrations")
	if err != nil {
		log.Fatalf("failed creating atlas migration directory: %v", err)
	}
	// Migrate diff options.
	opts := []schema.MigrateOption{
		schema.WithDir(dir),                          // provide migration directory
		schema.WithMigrationMode(schema.ModeReplay),  // provide migration mode
		schema.WithDialect(dialect.Postgres),            // Ent dialect to use
		schema.WithFormatter(atlas.DefaultFormatter), // Default Atlas formatter
	}
	if len(os.Args) != 2 {
		log.Fatalln("migration name is required. Use: 'go run -mod=mod ./ent/migrate/main.go <name>'")
	}
	// Generate migrations using Atlas support for PostgreSQL (note the Ent dialect option passed above).
	err = migrate.NamedDiff(ctx, "postgres://postgres:pass@localhost:5436/test?sslmode=disable", os.Args[1], opts...)
	if err != nil {
		log.Fatalf("failed generating migration file: %v", err)
	}
}
