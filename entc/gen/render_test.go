// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"testing"

	"github.com/neko-sc/ent/entc/load"
	"github.com/stretchr/testify/require"
)

func TestTypeRenderer_ImportCollisions(t *testing.T) {
	renderer := NewTypeRenderer(load.Package{Path: "example.com/generated", Name: "generated"}, "json", "model")
	typ := namedType("example.com/containers", "container", "Box",
		namedType("example.com/one/model", "model", "Value"),
		namedType("example.com/two/model", "model", "Other"),
		namedType("example.com/json", "json", "Document"),
		namedType("example.com/generated", "generated", "Local"),
	)
	require.NoError(t, renderer.Add(typ))

	require.Equal(t, []Import{
		{Path: "example.com/containers"},
		{Alias: "json2", Path: "example.com/json"},
		{Alias: "model2", Path: "example.com/one/model"},
		{Alias: "model3", Path: "example.com/two/model"},
	}, renderer.Imports())
	rendered, err := renderer.Render(typ)
	require.NoError(t, err)
	require.Equal(t, "container.Box[model2.Value, model3.Other, json2.Document, Local]", rendered)
}

func TestTypeRenderer_QualifiesSourceTypeWithGeneratedPackagePath(t *testing.T) {
	renderer := NewTypeRenderer(load.Package{Path: "example.com/ent/task", Name: "task"}, "task")
	priority := namedType("example.com/ent/schema/task", "task", "Priority")
	require.NoError(t, renderer.Add(priority))
	require.Equal(t, []Import{{Alias: "task2", Path: "example.com/ent/schema/task"}}, renderer.Imports())
	rendered, err := renderer.Render(priority)
	require.NoError(t, err)
	require.Equal(t, "task2.Priority", rendered)
}

func TestTypeRenderer_ReservedPredeclaredAndStableOrder(t *testing.T) {
	types := []*load.TypeExpression{
		namedType("example.com/map", "map", "Map"),
		namedType("example.com/int", "int", "Integer"),
		namedType("example.com/z", "shared", "Z"),
		namedType("example.com/a", "shared", "A"),
	}
	renderer := NewTypeRenderer(load.Package{Path: "example.com/generated", Name: "generated"})
	require.NoError(t, renderer.Add(types...))
	require.Equal(t, []Import{
		{Path: "example.com/a"},
		{Alias: "int2", Path: "example.com/int"},
		{Alias: "map2", Path: "example.com/map"},
		{Alias: "shared2", Path: "example.com/z"},
	}, renderer.Imports())
}

func TestTypeRenderer_FixedTemplateQualifier(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		qualifier string
	}{
		{name: "predicate", path: "example.com/predicate", qualifier: "predicate2"},
		{name: "sql", path: "example.com/sql", qualifier: "sql2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			renderer := NewTypeRenderer(load.Package{Path: "example.com/generated", Name: "generated"})
			typ := namedType(tt.path, tt.name, "Value")
			require.NoError(t, renderer.Add(typ))
			require.Equal(t, []Import{{Alias: tt.qualifier, Path: tt.path}}, renderer.Imports())
			rendered, err := renderer.Render(typ)
			require.NoError(t, err)
			require.Equal(t, tt.qualifier+".Value", rendered)
		})
	}
}

func TestTypeRenderer_Composites(t *testing.T) {
	typ := &load.TypeExpression{
		Kind: load.TypeKindStruct,
		Fields: []load.StructField{
			{Name: "Values", Type: &load.TypeExpression{
				Kind:    load.TypeKindMap,
				Key:     basicType(load.BasicKindString),
				Element: &load.TypeExpression{Kind: load.TypeKindPointer, Element: &load.TypeExpression{Kind: load.TypeKindArray, Length: 2, Element: namedType("example.com/model", "model", "Value")}},
			}, Tag: `json:"values"`},
			{Name: "Ready", Type: &load.TypeExpression{Kind: load.TypeKindChannel, ChannelDirection: load.ChannelReceive, Element: basicType(load.BasicKindBool)}},
		},
	}
	renderer := NewTypeRenderer(load.Package{Path: "example.com/generated", Name: "generated"})
	require.NoError(t, renderer.Add(typ))
	rendered, err := renderer.Render(typ)
	require.NoError(t, err)
	require.Equal(t, "struct {Values map[string]*[2]model.Value \"json:\\\"values\\\"\"; Ready <-chan bool}", rendered)
}

func TestTypeRenderer_AliasHint(t *testing.T) {
	typ := &load.TypeExpression{
		Kind:   load.TypeKindAlias,
		Alias:  &load.TypeName{Package: load.Package{Path: "example.com/alias", Name: "alias"}, Name: "Value"},
		Target: namedType("example.com/model", "model", "Value"),
	}
	renderer := NewTypeRenderer(load.Package{Path: "example.com/generated", Name: "generated"})
	require.NoError(t, renderer.Add(typ))
	rendered, err := renderer.Render(typ)
	require.NoError(t, err)
	require.Equal(t, "alias.Value", rendered)
	require.Equal(t, []Import{{Path: "example.com/alias"}}, renderer.Imports())
}

func namedType(path, packageName, name string, arguments ...*load.TypeExpression) *load.TypeExpression {
	return &load.TypeExpression{
		Kind: load.TypeKindNamed,
		Named: &load.TypeName{
			Package: load.Package{Path: path, Name: packageName},
			Name:    name,
		},
		Arguments: arguments,
	}
}

func basicType(kind load.BasicKind) *load.TypeExpression {
	return &load.TypeExpression{Kind: load.TypeKindBasic, Basic: kind}
}
