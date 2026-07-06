// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

// Package fakent fis a fake generated Ent package.
package fakent

import (
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/entc/load/testdata/cycle"
)

type Hook = ent.Hook

var _ = &cycle.Used{}
