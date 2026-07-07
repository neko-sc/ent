// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package edge

import "github.com/neko-sc/ent/schema"

// Annotation is a builtin schema annotation for
// configuring the edges' behavior in codegen.
type Annotation struct {
	// The StructTag option allows overriding the struct-tag
	// of the `Edges` field in the generated entity. For example:
	//
	//	edge.Annotation{
	//		StructTag: `json:"pet_edges"`
	//	}
	//
	StructTag string
}

// Name describes the annotation name.
func (Annotation) Name() string {
	return "Edges"
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
	if tag := ant.StructTag; tag != "" {
		a.StructTag = tag
	}
	return a
}

var (
	_ schema.Annotation = (*Annotation)(nil)
	_ schema.Merger     = (*Annotation)(nil)
)
