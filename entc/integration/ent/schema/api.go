// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import "github.com/neko-sc/ent"

// Api represents an example schema with wrong usage of acronym (e.g., API).
// nolint:stylecheck
type Api struct {
	ent.Schema
}
