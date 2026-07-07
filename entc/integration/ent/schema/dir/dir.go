// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

// Package schemadir is used as an example to test cases where
// package-name is not equal to the package-path (schemadir <> dir).
package schemadir

import (
	"net/http"
)

type Dir http.Dir
