// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("name"),
	}
}

// UserAuditLog holds the schema definition for the UserAuditLog entity.
type UserAuditLog struct {
	ent.Schema
}

// Fields of the UserAuditLog.
func (UserAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("operation_type"),
		field.String("operation_time"),
		field.String("old_value").
			Optional(),
		field.String("new_value").
			Optional(),
	}
}
