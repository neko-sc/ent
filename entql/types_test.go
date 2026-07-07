// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package entql

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFielder(t *testing.T) {
	tests := []struct {
		input    Fielder
		expected string
	}{
		{
			input:    StringEQ("value"),
			expected: `field == "value"`,
		},
		{
			input: StringOr(
				StringEQ("a"),
				StringEQ("b"),
				StringEQ("c"),
			),
			expected: `(field == "a" || field == "b" || field == "c")`,
		},
		{
			input: StringAnd(
				StringEQ("a"),
				StringNot(
					StringOr(
						StringEQ("b"),
						StringGT("c"),
						StringNEQ("d"),
					),
				),
			),
			expected: `field == "a" && !((field == "b" || field > "c" || field != "d"))`,
		},
		{
			input:    IntGT(1),
			expected: `field > 1`,
		},
		{
			input:    IntGTE(1),
			expected: `field >= 1`,
		},
		{
			input:    IntLT(1),
			expected: `field < 1`,
		},
		{
			input:    IntLTE(1),
			expected: `field <= 1`,
		},
		{
			input:    IntGT(1),
			expected: `field > 1`,
		},
		{
			input:    IntNot(IntGTE(1)),
			expected: `!(field >= 1)`,
		},
		{
			input: BoolNot(
				BoolOr(
					BoolEQ(true),
					BoolEQ(false),
				),
			),
			expected: `!(field == true || field == false)`,
		},
	}
	for i := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			p := tests[i].input.Field("field")
			assert.Equal(t, tests[i].expected, p.String())
		})
	}
}
