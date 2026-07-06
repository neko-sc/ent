// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/field"
)

// Pet holds the schema definition for the Pet entity.
type Pet struct {
	ent.Schema
}

// Fields of the Pet.
func (Pet) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

// PetUserName represents a user/pet name returned from the pet_user_names view.
type PetUserName struct {
	ent.View
}

// Annotations of the PetUserName.
func (PetUserName) Annotations() []schema.Annotation {
	return []schema.Annotation{
		// The definition below returns all names in the system
		// by unifying the "users" table and the "pets" table.
		entsql.ViewFor(dialect.Postgres, func(s *sql.Selector) {
			s.SelectDistinct("name").
				From(
					s.New().Select("name").From(sql.Table("users")).
						Union(
							s.New().Select("name").From(sql.Table("pets")),
						).
						As("all_names"),
				)
		}),
		// Alternatively, you can use raw definitions to define the view.
		// But note, this definition is skipped if the ViewFor annotation
		// is defined for the dialect we generated migration to (Postgres).
		entsql.View(`SELECT DISTINCT name
FROM (
    SELECT users.name
    FROM users
    UNION
    SELECT pets.name
    FROM pets
) AS all_names;
`),
	}
}

// Fields of the PetUserName.
func (PetUserName) Fields() []ent.Field {
	return []ent.Field{
		// Skip adding the "id" field as
		// it is not needed for this view.
		field.String("name"),
	}
}
