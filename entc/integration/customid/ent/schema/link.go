// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	uuidc "github.com/neko-sc/ent/entc/integration/customid/uuidcompatible"
	"github.com/neko-sc/ent/schema/field"
)

type LinkInformation struct {
	Name string
	Link string
}

// Link holds the schema definition for the Link entity.
type Link struct {
	ent.Schema
}

// Fields of the IntSid.
func (Link) Fields() []ent.Field {
	return []ent.Field{
		field.UUID[uuidc.UUIDC]("id").DefaultFunc(uuidc.NewUUIDC),
		field.JSON[map[string]LinkInformation]("link_information").
			Default(map[string]LinkInformation{
				"ent": {
					Name: "ent",
					Link: "https://entgo.io/",
				},
			}),
	}
}
