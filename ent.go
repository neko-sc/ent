// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

// Package ent is the interface between end-user schemas and entc (ent codegen).
package ent

import (
	"slices"

	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

type (
	// The Interface type describes the requirements for an exported type defined in the schema package.
	// It functions as the interface between the user's schema types and codegen loader.
	// Users should use the Schema type for embedding as follows:
	//
	//	type T struct {
	//		ent.Schema
	//	}
	//
	Interface interface {
		// Type is a dummy method, that is used in edge declaration.
		//
		// The Type method should be used as follows:
		//
		//	type S struct { ent.Schema }
		//
		//	type T struct { ent.Schema }
		//
		//	func (T) Edges() []ent.Edge {
		//		return []ent.Edge{
		//			edge.To("S", S.Type),
		//		}
		//	}
		//
		Type()
		// Fields returns the fields of the schema.
		Fields() []Field
		// Edges returns the edges of the schema.
		Edges() []Edge
		// Indexes returns the indexes of the schema.
		Indexes() []Index
		// Config returns an optional config for the schema.
		//
		// Deprecated: the Config method predates the Annotations method, and it
		// is planned be removed in v0.5.0. New code should use Annotations instead.
		//
		//	func (T) Annotations() []schema.Annotation {
		//		return []schema.Annotation{
		//			entsql.Annotation{Table: "Name"},
		//		}
		//	}
		//
		Config() Config
		// Mixin returns an optional list of Mixin to extends
		// the schema.
		Mixin() []Mixin
		// Annotations returns a list of schema annotations to be used by
		// codegen extensions.
		Annotations() []schema.Annotation
	}

	// A Field interface returns a field descriptor for vertex fields/properties.
	// The usage for the interface is as follows:
	//
	//	func (T) Fields() []ent.Field {
	//		return []ent.Field{
	//			field.Int("int"),
	//		}
	//	}
	//
	Field interface {
		Descriptor() *field.Descriptor
	}

	// A Edge interface returns an edge descriptor for vertex edges.
	// The usage for the interface is as follows:
	//
	//	func (T) Edges() []ent.Edge {
	//		return []ent.Edge{
	//			edge.To("S", S.Type),
	//		}
	//	}
	//
	Edge interface {
		Descriptor() *edge.Descriptor
	}

	// An Index interface returns an index descriptor for vertex indexes.
	// The usage for the interface is as follows:
	//
	//	func (T) Indexes() []ent.Index {
	//		return []ent.Index{
	//			index.Fields("f1", "f2").
	//				Unique(),
	//		}
	//	}
	//
	Index interface {
		Descriptor() *index.Descriptor
	}

	// A Config structure is used to configure an entity schema.
	// The usage of this structure is as follows:
	//
	//	func (T) Config() ent.Config {
	//		return ent.Config{
	//			Table: "Name",
	//		}
	//	}
	//
	// Deprecated: the Config object predates the schema.Annotation method and it
	// is planned to be removed in future versions. New code should use Annotations
	// instead.
	//
	//	func (T) Annotations() []schema.Annotation {
	//		return []schema.Annotation{
	//			entsql.Annotation{Table: "Name"},
	//		}
	//	}
	//
	Config struct {
		// A Table is an optional table name defined for the schema.
		Table string
	}

	// The Mixin type describes a set of methods that can extend
	// other methods in the schema without calling them directly.
	//
	//	type TimeMixin struct {}
	//
	//	func (TimeMixin) Fields() []ent.Field {
	//		return []ent.Field{
	//			field.Time("created_at").
	//				Immutable().
	//				Default(time.Now),
	//			field.Time("updated_at").
	//				Default(time.Now).
	//				UpdateDefault(time.Now),
	//		}
	//	}
	//
	//	type T struct {
	//		ent.Schema
	//	}
	//
	// 	func(T) Mixin() []ent.Mixin {
	// 		return []ent.Mixin{
	//			TimeMixin{},
	// 		}
	// 	}
	//
	Mixin interface {
		// Fields returns a slice of fields to add to the schema.
		Fields() []Field
		// Edges returns a slice of edges to add to the schema.
		Edges() []Edge
		// Indexes returns a slice of indexes to add to the schema.
		Indexes() []Index
		// Annotations returns a list of schema annotations to add
		// to the schema annotations.
		Annotations() []schema.Annotation
	}

	// Schema is the default implementation for the schema Interface.
	// It can be embedded in end-user schemas as follows:
	//
	//	type T struct {
	//		ent.Schema
	//	}
	//
	Schema struct {
		Interface
	}

	// A View only schema describes an entity that all its operations
	// are limited to read-only. For example, a database view.
	//
	// Users that wants to define a view schema should embed the View
	// struct in their schema as follows:
	//
	//	type V struct {
	//		ent.View
	//	}
	//
	View struct {
		Schema
	}
)

// Fields of the schema.
func (Schema) Fields() []Field { return nil }

// Edges of the schema.
func (Schema) Edges() []Edge { return nil }

// Indexes of the schema.
func (Schema) Indexes() []Index { return nil }

// Config of the schema.
func (Schema) Config() Config { return Config{} }

// Mixin of the schema.
func (Schema) Mixin() []Mixin { return nil }

// Annotations of the schema.
func (Schema) Annotations() []schema.Annotation { return nil }

// Viewer is an interface that wraps the view method.
// Implemented by the View struct.
type Viewer interface{ view() }

// view is a dummy method to distinguish between Schema and View.
func (View) view() {}

// Mutation identifies an operation on an entity type.
type Mutation interface {
	Op() Op
	Type() string
}

// Query represents an entity query builder.
type Query any

//go:generate go run golang.org/x/tools/cmd/stringer -type Op

// An Op represents a mutation operation.
type Op uint

// Mutation operations.
const (
	OpCreate    Op = 1 << iota // node creation.
	OpUpdate                   // update nodes by predicate (if any).
	OpUpdateOne                // update one node.
	OpDelete                   // delete nodes by predicate (if any).
	OpDeleteOne                // delete one node.
)

// Is reports whether o is match the given operation.
func (i Op) Is(o Op) bool { return i&o != 0 }

// List of query operations used by the codegen.
const (
	OpQueryFirst   = "First"
	OpQueryFirstID = "FirstID"
	OpQueryOnly    = "Only"
	OpQueryOnlyID  = "OnlyID"
	OpQueryAll     = "All"
	OpQueryIDs     = "IDs"
	OpQueryCount   = "Count"
	OpQueryExist   = "Exist"
	OpQueryGroupBy = "GroupBy"
	OpQuerySelect  = "Select"
)

type (
	// QueryContext contains additional information about
	// the context in which the query is executed.
	QueryContext struct {
		// Op defines the operation name. e.g., First, All, Count, etc.
		Op string
		// Type defines the query type as defined in the generated code.
		Type string
		// Unique indicates if the Unique modifier was set on the query and
		// its value. Calling Unique(false) sets the value of Unique to false.
		Unique *bool
		// Limit indicates if the Limit modifier was set on the query and
		// its value. Calling Limit(10) sets the value of Limit to 10.
		Limit *int
		// Offset indicates if the Offset modifier was set on the query and
		// its value. Calling Offset(10) sets the value of Offset to 10.
		Offset *int
		// Fields specifies the fields that were selected in the query.
		Fields []string
	}
)

// Clone returns a deep copy of the query context.
func (q *QueryContext) Clone() *QueryContext {
	c := &QueryContext{
		Op:     q.Op,
		Type:   q.Type,
		Fields: append([]string(nil), q.Fields...),
	}
	if q.Unique != nil {
		v := *q.Unique
		c.Unique = &v
	}
	if q.Limit != nil {
		v := *q.Limit
		c.Limit = &v
	}
	if q.Offset != nil {
		v := *q.Offset
		c.Offset = &v
	}
	return c
}

// AppendFieldOnce adds the given field to the spec if it is not already present.
func (q *QueryContext) AppendFieldOnce(f string) *QueryContext {
	if slices.Contains(q.Fields, f) {
		return q
	}
	q.Fields = append(q.Fields, f)
	return q
}
