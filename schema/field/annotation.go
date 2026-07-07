// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field

import (
	"maps"

	"github.com/neko-sc/ent/schema"
)

// Annotation is a builtin schema annotation for
// configuring the schema fields in codegen.
type Annotation struct {
	// The StructTag option allows overriding the struct-tag
	// of the fields in the generated entity. For example:
	//
	//	field.Annotation{
	//		StructTag: map[string]string{
	//			"id": `json:"id,omitempty" yaml:"-"`,
	//		},
	//	}
	//
	StructTag map[string]string

	// ID defines a multi-field schema identifier. Note,
	// the annotation is valid only for edge schemas.
	//
	//	func (TweetLike) Annotations() []schema.Annotation {
	//		return []schema.Annotation{
	//			field.ID("user_id", "tweet_id"),
	//		}
	//	}
	//
	ID []string
}

// ID defines a multi-field schema identifier. Note, the
// annotation is valid only for edge schemas.
//
//	func (TweetLike) Annotations() []schema.Annotation {
//		return []schema.Annotation{
//			field.ID("user_id", "tweet_id"),
//		}
//	}
func ID(first, second string, fields ...string) *Annotation {
	return &Annotation{ID: append([]string{first, second}, fields...)}
}

// Name describes the annotation name.
func (Annotation) Name() string {
	return "Fields"
}

// Merge implements the schema.Merger interface.
func (a Annotation) Merge(other schema.Annotation) schema.Annotation {
	var ant Annotation
	switch other := other.(type) {
	case Annotation:
		ant = other
	case *Annotation:
		if other != nil {
			ant = *other
		}
	default:
		return a
	}
	if a.StructTag == nil && len(ant.StructTag) > 0 {
		a.StructTag = make(map[string]string, len(ant.StructTag))
	}
	maps.Copy(a.StructTag, ant.StructTag)
	if len(ant.ID) > 0 {
		a.ID = ant.ID
	}
	return a
}

var _ interface {
	schema.Annotation
	schema.Merger
} = (*Annotation)(nil)
