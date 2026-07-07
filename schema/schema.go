// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

// Annotation is used to attach arbitrary metadata to the schema objects in codegen.
// The object must be serializable to JSON raw value (e.g. struct, map or slice).
//
// Template extensions can retrieve this metadata and use it inside their templates.
type Annotation interface {
	// Name defines the name of the annotation to be retrieved by the codegen.
	Name() string
}

// Merger wraps the single Merge function allows custom annotation to provide
// an implementation for merging 2 or more annotations from the same type.
//
// A common use case is where the same Annotation type is defined both in
// mixin.Schema and ent.Schema.
type Merger interface {
	Merge(Annotation) Annotation
}

// CommentAnnotation is a builtin schema annotation for
// configuring the schema's Godoc comment.
type CommentAnnotation struct {
	Text string // Comment text.
}

// Name implements the Annotation interface.
func (*CommentAnnotation) Name() string {
	return "Comment"
}

// Comment is a builtin schema annotation for
// configuring the schema's Godoc comment.
func Comment(text string) *CommentAnnotation {
	return &CommentAnnotation{Text: text}
}

var _ Annotation = (*CommentAnnotation)(nil)
