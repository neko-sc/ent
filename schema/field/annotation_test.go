// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field_test

import (
	"testing"

	"github.com/neko-sc/ent/schema/field"

	"github.com/stretchr/testify/assert"
)

func TestAnnotation_Merge(t *testing.T) {
	ant := field.Annotation{}
	a := ant.Merge(field.Annotation{
		StructTag: map[string]string{"foo": "bar"},
	})
	assert.Equal(t, a.(field.Annotation).StructTag["foo"], "bar")
	a = ant.Merge(&field.Annotation{
		StructTag: map[string]string{"foo": "baz", "baz": "qux"},
	})
	assert.Equal(t, a.(field.Annotation).StructTag["foo"], "baz")
	assert.Equal(t, a.(field.Annotation).StructTag["baz"], "qux")
}
