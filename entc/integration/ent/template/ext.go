// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package template

// Extension is a template extension.
type Extension struct {
	Type string
}

func (*Extension) Name() string {
	return "Extension"
}
