// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package cycle

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/load/testdata/cycle/fakent"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the user schema.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.JSON("used", &Used{}),
		field.Enum("e").
			GoType(Enum(0)),
	}
}

type (
	Used        struct{}
	NotUsed     struct{}
	notExported struct{}
	Enum        int
)

func (Enum) Values() []string { return nil }

// The cause for cycle.
var _ fakent.Hook = nil
