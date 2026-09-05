// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"os"
	"path/filepath"
)

var (
	// FeatureNamedEdges provides a feature-flag for eager-loading edges with dynamic names.
	FeatureNamedEdges = Feature{
		Name:        "namedges",
		Stage:       Experimental,
		Default:     false,
		Description: "NamedEdges provides an API for eager-loading edges with dynamic names",
	}

	// FeatureBidiEdgeRefs provides a feature-flag for sql dialect to set two-way
	// references when loading (unique) edges. Note, users that use the standard
	// encoding/json.MarshalJSON should detach the circular references before marshaling.
	FeatureBidiEdgeRefs = Feature{
		Name:        "bidiedges",
		Stage:       Experimental,
		Default:     false,
		Description: "This features guides Ent to set two-way references when loading (O2M/O2O) edges",
	}

	// FeatureSnapshot stores a snapshot of ent/schema and auto-solve merge-conflict (issue #852).
	FeatureSnapshot = Feature{
		Name:        "schema/snapshot",
		Stage:       Experimental,
		Default:     false,
		Description: "Schema snapshot stores a snapshot of ent/schema and auto-solve merge-conflict (issue #852)",
		GraphTemplates: []GraphTemplate{
			{
				Name:   "internal/schema",
				Format: "internal/schema.go",
			},
		},
		cleanup: func(c *Config) error {
			return remove(filepath.Join(c.Target, "internal"), "schema.go")
		},
	}

	// FeatureSchemaConfig allows users to pass init time alternate schema names
	// for each ent model. This is useful if your SQL tables are spread out against
	// multiple databases.
	FeatureSchemaConfig = Feature{
		Name:        "sql/schemaconfig",
		Stage:       Stable,
		Default:     false,
		Description: "Allows alternate schema names for each ent model. Useful if SQL tables are spread out against multiple databases",
		GraphTemplates: []GraphTemplate{
			{
				Name:   "dialect/sql/internal/schemaconfig",
				Format: "internal/schemaconfig.go",
			},
		},
		cleanup: func(c *Config) error {
			return remove(filepath.Join(c.Target, "internal"), "schemaconfig.go")
		},
	}

	// featureMultiSchema indicates that ent/schema is annotated with multiple schemas.
	// This feature-flag is enabled by default by the storage driver and exists to pass
	// this info to the templates.
	featureMultiSchema = Feature{
		Name:  "sql/multischema",
		Stage: Beta,
	}

	FeatureVersionedMigration = Feature{
		Name:        "sql/versioned-migration",
		Stage:       Experimental,
		Default:     false,
		Description: "Allows users to work with versioned migrations / migration files",
	}

	FeatureGlobalID = Feature{
		Name:        "sql/globalid",
		Stage:       Experimental,
		Default:     false,
		Description: "Ensures all nodes have a unique global identifier", GraphTemplates: []GraphTemplate{
			{
				Name:   "internal/globalid",
				Format: "internal/globalid.go",
			},
		},
		cleanup: func(c *Config) error {
			return remove(filepath.Join(c.Target, "internal"), "globalid.go")
		},
	}

	// AllFeatures holds a list of all feature-flags.
	AllFeatures = []Feature{
		FeatureNamedEdges,
		FeatureBidiEdgeRefs,
		FeatureSnapshot,
		FeatureSchemaConfig,
		FeatureVersionedMigration,
		FeatureGlobalID,
	}
	// allFeatures includes all public and private features.
	allFeatures = append(AllFeatures, featureMultiSchema)
)

// FeatureStage describes the stage of the codegen feature.
type FeatureStage int

const (
	_ FeatureStage = iota

	// Experimental features are in development, and actively being tested in the
	// integration environment.
	Experimental

	// Alpha features are features whose initial development was finished, tested
	// on the infra of the ent team, but we expect breaking-changes to their APIs.
	Alpha

	// Beta features are Alpha features that were added to the
	// documentation, and no breaking-changes are expected for them.
	Beta

	// Stable features are Beta features that were running for a while on ent
	// infra.
	Stable
)

// A Feature of the ent codegen.
type Feature struct {
	// Name of the feature.
	Name string

	// Stage of the feature.
	Stage FeatureStage

	// Default values indicates if this feature is enabled by default.
	Default bool

	// A Description of this feature.
	Description string

	// Templates defines list of templates for extending or overriding the default
	// templates. In order to write the template output to a standalone file, use
	// the GraphTemplates below.
	Templates []*Template

	// GraphTemplates defines optional templates to be executed on the graph
	// and will their output will be written to the configured destination.
	GraphTemplates []GraphTemplate

	// cleanup used to cleanup all changes when a feature-flag is removed.
	// e.g. delete files from previous codegen runs.
	cleanup func(*Config) error
}

// remove file (if exists) and its dir if it's empty.
func remove(dir, file string) error {
	if err := os.Remove(filepath.Join(dir, file)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	infos, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		return os.Remove(dir)
	}
	return nil
}
