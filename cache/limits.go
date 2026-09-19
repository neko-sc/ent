// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

// Limits bounds one recorded result. Nonpositive fields select the defaults.
type Limits struct {
	MaxRows  int
	MaxBytes int64
}

const (
	DefaultMaxRows        = 5000
	DefaultMaxBytes int64 = 4 << 20
)
