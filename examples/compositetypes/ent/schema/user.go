// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"database/sql/driver"
	"fmt"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("address").
			GoType(&Address{}).
			SchemaType(map[string]string{
				dialect.Postgres: "address",
			}),
	}
}

type Address struct {
	Street, City string
}

var _ field.ValueScanner = (*Address)(nil)

// Scan implements the database/sql.Scanner interface.
func (a *Address) Scan(v interface{}) (err error) {
	switch v := v.(type) {
	case nil:
	case string:
		_, err = fmt.Sscanf(v, "(%q,%q)", &a.Street, &a.City)
	case []byte:
		_, err = fmt.Sscanf(string(v), "(%q,%q)", &a.Street, &a.City)
	}
	return
}

// Value implements the driver.Valuer interface.
func (a *Address) Value() (driver.Value, error) {
	return fmt.Sprintf("(%q,%q)", a.Street, a.City), nil
}
