// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/mixin"

	"github.com/neko-sc/atlas/sql/postgres"
)

// License holds the schema definition for the License entity.
type License struct {
	ent.Schema
}

func (License) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}

// Fields of the License.
func (License) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			SchemaType(map[string]string{
				dialect.Postgres: postgres.TypeBigSerial,
			}),
	}
}
