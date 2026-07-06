// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/mixin"
)

// This file contains two example of how to define the base schema.
// 1. Use Mixin and use it in all schemas that reside in "db1".
// 2. Create a "base" schema and use struct embedding to in all schemas that reside in "db1".

// Example 1:

// Mixin holds the default configuration for most schemas in this package.
type Mixin struct {
	mixin.Schema
}

// Annotations of the Mixin.
func (Mixin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Schema("db1"),
	}
}

// Example 2:

// Base schema.
type base struct {
	ent.Schema
}

// Annotations of the base schema.
func (base) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Schema("db1"),
	}
}
