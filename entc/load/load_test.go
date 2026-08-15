// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"go/types"
	"testing"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
	"golang.org/x/tools/go/packages"

	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	cfg := &Config{Path: "./testdata/valid"}
	spec, err := cfg.Load()
	require.NoError(t, err)
	require.Len(t, spec.Schemas, 3)
	require.Equal(t, "github.com/neko-sc/ent/entc/load/testdata/valid", spec.PkgPath)

	require.Equal(t, "Group", spec.Schemas[0].Name, "ordered alphabetically")
	require.Equal(t, "Tag", spec.Schemas[1].Name)
	require.Equal(t, "User", spec.Schemas[2].Name)
	for _, schema := range spec.Schemas {
		for _, loadedField := range schema.Fields {
			require.NotNil(t, loadedField.Semantic)
		}
	}
}

func TestLoad_SemanticTypesFromGenericHelperAndMixin(t *testing.T) {
	spec, err := (&Config{Path: "./testdata/semantic"}).Load()
	require.NoError(t, err)
	require.Len(t, spec.Schemas, 2)
	require.Equal(t, "Tag", spec.Schemas[0].Name)
	require.Equal(t, "User", spec.Schemas[1].Name)
	require.Equal(t, LogicalTypeUUID, spec.Schemas[0].Fields[0].Semantic.Logical)
	require.Equal(t, "github.com/google/uuid", spec.Schemas[0].Fields[0].Semantic.Representation.Named.Package.Path)
	require.Len(t, spec.Schemas[1].Fields, 18)

	mixed := spec.Schemas[1].Fields[0]
	require.Equal(t, "mixed", mixed.Name)
	require.True(t, mixed.Position.MixedIn)
	require.Equal(t, LogicalTypeJSON, mixed.Semantic.Logical)
	require.Equal(t, TypeKindNamed, mixed.Semantic.Representation.Kind)
	require.Equal(t, "Phantom", mixed.Semantic.Representation.Named.Name)
	require.Equal(t, "github.com/neko-sc/ent/entc/load/testdata/semantic", mixed.Semantic.Representation.Named.Package.Path)
	require.Len(t, mixed.Semantic.Representation.Arguments, 2)
	require.Equal(t, "URL", mixed.Semantic.Representation.Arguments[0].Named.Name)
	require.Equal(t, "net/url", mixed.Semantic.Representation.Arguments[0].Named.Package.Path)
	require.Equal(t, TypeKindArray, mixed.Semantic.Representation.Arguments[1].Kind)
	require.Equal(t, int64(3), mixed.Semantic.Representation.Arguments[1].Length)
	require.Equal(t, BasicKindBool, mixed.Semantic.Representation.Arguments[1].Element.Basic)

	payload := spec.Schemas[1].Fields[1]
	require.Equal(t, "payload", payload.Name)
	require.False(t, payload.Position.MixedIn)
	require.Equal(t, LogicalTypeJSON, payload.Semantic.Logical)
	require.Equal(t, TypeKindNamed, payload.Semantic.Representation.Kind)
	require.Equal(t, "Envelope", payload.Semantic.Representation.Named.Name)
	require.Len(t, payload.Semantic.Representation.Arguments, 2)
	value := payload.Semantic.Representation.Arguments[0]
	require.Equal(t, TypeKindMap, value.Kind)
	require.Equal(t, BasicKindString, value.Key.Basic)
	phantom := value.Element.Element.Element
	require.Equal(t, TypeKindNamed, phantom.Kind)
	require.Equal(t, "Phantom", phantom.Named.Name)
	require.Len(t, phantom.Arguments, 2)
	require.Equal(t, "URL", phantom.Arguments[0].Named.Name)
	require.Equal(t, "net/url", phantom.Arguments[0].Named.Package.Path)
	require.Equal(t, TypeKindArray, phantom.Arguments[1].Kind)
	require.Equal(t, int64(2), phantom.Arguments[1].Length)
	require.Equal(t, BasicKindInt, phantom.Arguments[1].Element.Basic)
	marker := payload.Semantic.Representation.Arguments[1]
	require.Equal(t, TypeKindChannel, marker.Kind)
	require.Equal(t, ChannelSend, marker.ChannelDirection)
	require.Equal(t, TypeKindSignature, marker.Element.Kind)
	require.Equal(t, "URL", marker.Element.Parameters[0].Named.Name)
	require.Equal(t, "net/url", marker.Element.Parameters[0].Named.Package.Path)
	require.Equal(t, TypeKindNamed, marker.Element.Results[0].Kind)
	require.Equal(t, "error", marker.Element.Results[0].Named.Name)

	duplicate := spec.Schemas[1].Fields[2]
	require.Equal(t, "payload_duplicate", duplicate.Name)
	require.Equal(t, payload.Semantic.ID, duplicate.Semantic.ID)
	require.Equal(t, payload.Semantic.Representation, duplicate.Semantic.Representation)

	var restrictions *Field
	for _, loadedField := range spec.Schemas[1].Fields {
		if loadedField.Name == "restrictions" {
			restrictions = loadedField
			break
		}
	}
	require.NotNil(t, restrictions)
	require.Equal(t, LogicalTypeOther, restrictions.Semantic.Logical)
	require.Equal(t, TypeKindSlice, restrictions.Semantic.Representation.Kind)
	require.Equal(t, "RestrictionType", restrictions.Semantic.Representation.Element.Named.Name)
	require.Equal(t, "github.com/neko-sc/ent/entc/load/testdata/semantic/schema_types", restrictions.Semantic.Representation.Element.Named.Package.Path)
	require.Equal(t, "text[]", restrictions.SchemaType[dialect.Postgres])
	require.Equal(t, "text[]", restrictions.Semantic.Storage.Dialects[dialect.Postgres])
	require.True(t, restrictions.ValueScanner)

	for _, loadedField := range spec.Schemas[1].Fields {
		switch loadedField.Name {
		case "link":
			require.Equal(t, "String()", loadedField.Semantic.Capabilities.LogicalProjection)
		case "direct_at":
			require.Equal(t, LogicalTypeTime, loadedField.Semantic.Logical)
			require.True(t, loadedField.Semantic.Capabilities.ConvertibleToLogical)
			require.True(t, loadedField.Semantic.Capabilities.LogicalReverseConvertible)
		case "encoded":
			require.Equal(t, []field.ValidatorKind{
				field.ValidatorRepresentation,
				field.ValidatorLogical,
				field.ValidatorRepresentation,
			}, loadedField.ValidatorKinds)
		}
	}
}

func TestUnmarshalSchemaLines(t *testing.T) {
	schemas, err := unmarshalSchemaLines("\n{\"name\":\"First\"}\n{\"name\":\"Second\"}")
	require.NoError(t, err)
	require.Len(t, schemas, 2)
	require.Equal(t, "First", schemas[0].Name)
	require.Equal(t, "Second", schemas[1].Name)

	schemas, err = unmarshalSchemaLines("")
	require.NoError(t, err)
	require.Empty(t, schemas)

	schemas, err = unmarshalSchemaLines("not-json\n")
	require.EqualError(t, err, "entc/load: unmarshal schema not-json: invalid character 'o' in literal null (expecting 'u')")
	require.Nil(t, schemas)
}

func TestRenderRuntimeRegistry_Empty(t *testing.T) {
	registry, err := renderRuntimeRegistry(nil, &packages.Package{PkgPath: "example.com/schema"})
	require.NoError(t, err)
	require.Empty(t, registry.Imports)
	require.Empty(t, registry.Entries)
}

func TestRenderRuntimeRegistry_UsesStablePrivateAliases(t *testing.T) {
	firstPackage := types.NewPackage("example.com/one/schema", "schema")
	secondPackage := types.NewPackage("example.com/two/schema", "schema")
	firstType := types.NewNamed(types.NewTypeName(0, firstPackage, "Value", nil), types.NewStruct(nil, nil), nil)
	secondType := types.NewNamed(types.NewTypeName(0, secondPackage, "Value", nil), types.NewStruct(nil, nil), nil)
	representation := &TypeExpression{Kind: TypeKindStruct}
	registry, err := renderRuntimeRegistry([]RuntimeType{
		{ID: "first", GoType: firstType, Expression: representation},
		{ID: "second", GoType: secondType, Expression: representation},
	}, &packages.Package{PkgPath: "example.com/schema"})
	require.NoError(t, err)
	require.Equal(t, "enttype1 \"example.com/one/schema\"\nenttype2 \"example.com/two/schema\"\n", registry.Imports)
	require.Contains(t, registry.Entries, "reflect.TypeFor[enttype1.Value]()")
	require.Contains(t, registry.Entries, "reflect.TypeFor[enttype2.Value]()")
}

func TestLoadWrongPath(t *testing.T) {
	cfg := &Config{Path: "./boring"}
	plg, err := cfg.Load()
	require.Error(t, err)
	require.Nil(t, plg)
}

func TestLoadSpecific(t *testing.T) {
	cfg := &Config{Path: "./testdata/valid", Names: []string{"User"}}
	spec, err := cfg.Load()
	require.NoError(t, err)
	require.Len(t, spec.Schemas, 1)
	require.Equal(t, "User", spec.Schemas[0].Name)
	require.Equal(t, "github.com/neko-sc/ent/entc/load/testdata/valid", spec.PkgPath)
}

func TestLoadNoSchema(t *testing.T) {
	cfg := &Config{Path: "./testdata/invalid"}
	schemas, err := cfg.Load()
	require.Error(t, err)
	require.Empty(t, schemas)
}

func TestLoadSchemaFailure(t *testing.T) {
	cfg := &Config{Path: "./testdata/failure"}
	spec, err := cfg.Load()
	require.Error(t, err)
	require.Nil(t, spec)
}

func TestLoadBaseSchema(t *testing.T) {
	cfg := &Config{Path: "./testdata/base"}
	spec, err := cfg.Load()
	require.NoError(t, err)
	require.Len(t, spec.Schemas, 1)
	require.Len(t, spec.Schemas[0].Fields, 2, "embedded base schema")
	f1 := spec.Schemas[0].Fields[0]
	require.Equal(t, "base_field", f1.Name)
	require.Equal(t, field.TypeInt, f1.Type)
	f2 := spec.Schemas[0].Fields[1]
	require.Equal(t, "user_field", f2.Name)
	require.Equal(t, field.TypeString, f2.Type)
}

func TestLoadTags(t *testing.T) {
	all, err := (&Config{
		Path: "./testdata/buildflags",
	}).Load()
	require.NoError(t, err)

	require.Len(t, all.Schemas, 2)
	require.Equal(t, "Group", all.Schemas[0].Name, "ordered alphabetically")
	require.Equal(t, "User", all.Schemas[1].Name)

	notags, err := (&Config{
		Path:       "./testdata/buildflags",
		BuildFlags: []string{"-tags", "hidegroups"},
	}).Load()
	require.NoError(t, err)

	require.Len(t, notags.Schemas, 1)
	require.Equal(t, "User", notags.Schemas[0].Name)

	require.Equal(t, all.Schemas[1], notags.Schemas[0])
}

func TestLoadCycleError(t *testing.T) {
	cfg := &Config{Path: "./testdata/cycle"}
	spec, err := cfg.Load()
	require.Nil(t, spec)
	require.EqualError(t, err, `entc/load: parse schema dir: import cycle not allowed: import stack: [github.com/neko-sc/ent/entc/load/testdata/cycle github.com/neko-sc/ent/entc/load/testdata/cycle/fakent github.com/neko-sc/ent/entc/load/testdata/cycle]
To resolve this issue, move the custom types used by the generated code to a separate package: "Enum", "Used"`)
}
