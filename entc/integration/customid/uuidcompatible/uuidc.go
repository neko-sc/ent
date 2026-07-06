// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package uuidc

import (
	"database/sql/driver"

	"github.com/google/uuid"
)

type UUIDC struct {
	uuid uuid.UUID
}

func NewUUIDC() UUIDC {
	return UUIDC{
		uuid: uuid.New(),
	}
}

func (u *UUIDC) Scan(src any) error {
	return u.uuid.Scan(src)
}

func (u UUIDC) Value() (driver.Value, error) {
	return u.uuid.Value()
}
