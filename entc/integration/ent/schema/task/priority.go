// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package task

import "fmt"

type Priority int

const (
	PriorityLow Priority = iota
	PriorityMid
	PriorityHigh
)

func (p Priority) String() string {
	s := "unknown"
	switch p {
	case PriorityLow:
		s = "low"
	case PriorityMid:
		s = "mid"
	case PriorityHigh:
		s = "high"
	}
	return s
}

func (p Priority) Validate() error {
	switch p {
	case PriorityLow, PriorityMid, PriorityHigh:
		return nil
	default:
		return fmt.Errorf("invalid priority value: %v", p)
	}
}
