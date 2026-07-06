// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
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

// Fields of the PetUserName.
func (PetUserName) Fields() []ent.Field {
	return []ent.Field{
		// Skip adding the "id" field as
		// it is not needed for this view.
		field.String("name"),
	}
}
