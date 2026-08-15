// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"encoding/json"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"net/url"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/neko-sc/ent/schema/field"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/packages"
)

func TestFieldTypeOf_InvalidInput(t *testing.T) {
	stringExpression, err := TypeExpressionFor[string]()
	require.NoError(t, err)
	tests := []struct {
		name           string
		logical        field.Type
		representation *TypeExpression
		error          string
	}{
		{
			name:           "invalid logical type",
			logical:        field.TypeInvalid,
			representation: stringExpression,
			error:          `load: invalid logical field type "invalid"`,
		},
		{
			name:    "nil representation",
			logical: field.TypeString,
			error:   `load: logical field type "string" has no representation`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FieldTypeOf(tt.logical, tt.representation)
			require.EqualError(t, err, tt.error)
		})
	}
}

func TestTypeExpressionFor_ReflectionTypes(t *testing.T) {
	tests := []struct {
		name       string
		goType     reflect.Type
		kind       TypeKind
		basic      BasicKind
		error      string
		assertType func(*testing.T, *TypeExpression)
	}{
		{name: "complex64", goType: reflect.TypeFor[complex64](), kind: TypeKindBasic, basic: BasicKindComplex64},
		{name: "complex128", goType: reflect.TypeFor[complex128](), kind: TypeKindBasic, basic: BasicKindComplex128},
		{name: "unsafe pointer", goType: reflect.TypeFor[unsafe.Pointer](), kind: TypeKindBasic, basic: BasicKindUnsafePointer},
		{
			name:   "atomic pointer",
			goType: reflect.TypeFor[atomicPointerURL](),
			kind:   TypeKindNamed,
			assertType: func(t *testing.T, expression *TypeExpression) {
				require.Equal(t, "sync/atomic", expression.Named.Package.Path)
				require.Equal(t, "atomic", expression.Named.Package.Name)
				require.Equal(t, "Pointer", expression.Named.Name)
				require.Len(t, expression.Arguments, 1)
				require.Equal(t, "net/url", expression.Arguments[0].Named.Package.Path)
				require.Equal(t, "URL", expression.Arguments[0].Named.Name)
			},
		},
		{
			name:   "nested generic composites",
			goType: reflect.TypeFor[reflectOuterGeneric](),
			kind:   TypeKindNamed,
			assertType: func(t *testing.T, expression *TypeExpression) {
				require.Equal(t, "reflectGeneric", expression.Named.Name)
				require.Len(t, expression.Arguments, 2)
				require.Equal(t, TypeKindMap, expression.Arguments[0].Kind)
				require.Equal(t, TypeKindSlice, expression.Arguments[0].Element.Kind)
				require.Equal(t, TypeKindPointer, expression.Arguments[0].Element.Element.Kind)
				require.Equal(t, "URL", expression.Arguments[0].Element.Element.Element.Named.Name)
				require.Equal(t, "Pointer", expression.Arguments[1].Named.Name)
				require.Equal(t, "URL", expression.Arguments[1].Arguments[0].Named.Name)
			},
		},
		{name: "channel", goType: reflect.TypeFor[chan<- int](), kind: TypeKindChannel},
		{name: "anonymous struct", goType: reflect.TypeFor[struct{ Value int }](), error: `load: reflected anonymous struct type "struct { Value int }" requires compiler metadata`},
		{name: "anonymous interface", goType: reflect.TypeFor[interface{ Value() }](), error: `load: reflected anonymous interface type "interface { Value() }" requires compiler metadata`},
		{name: "function", goType: reflect.TypeFor[func(int) string](), error: `load: reflected anonymous func type "func(int) string" requires compiler metadata`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expression, err := typeExpressionFor(tt.goType)
			if tt.error != "" {
				require.EqualError(t, err, tt.error)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.kind, expression.Kind)
			if tt.basic != "" {
				require.Equal(t, tt.basic, expression.Basic)
			}
			if tt.assertType != nil {
				tt.assertType(t, expression)
			}
		})
	}
}

type atomicPointerURL = atomic.Pointer[url.URL]

type reflectOuterGeneric = reflectGeneric[map[string][]*url.URL, atomic.Pointer[url.URL]]

type reflectGeneric[Value, Marker any] struct {
	Value Value
}

func TestParseReflectedNamedType_DeclaredPackageName(t *testing.T) {
	packageName, err := reflectedDeclaredPackageName("uuidc.UUIDC")
	require.NoError(t, err)
	require.Equal(t, "uuidc", packageName)
	require.NotEqual(t, reflectedPackageName("github.com/neko-sc/ent/entc/integration/customid/uuidcompatible"), packageName)
}

func TestTypeExpressionOf_NamedGenerics(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Box[A, B any] struct { Value A }
type Phantom[A, B any] struct { Value A }
type Nested = Box[Phantom[string, int], map[string][]*Phantom[bool, float64]]
`)

	nested := packageType(t, loadedPackage, "Nested")
	expression, err := TypeExpressionOf(nested)
	require.NoError(t, err)
	require.Equal(t, TypeKindAlias, expression.Kind)
	require.Equal(t, "Nested", expression.Alias.Name)
	require.Equal(t, TypeKindNamed, expression.Target.Kind)
	require.Equal(t, "Box", expression.Target.Named.Name)
	require.Len(t, expression.Target.Arguments, 2)
	require.Equal(t, "Phantom", expression.Target.Arguments[0].Named.Name)
	require.Len(t, expression.Target.Arguments[0].Arguments, 2)
	require.Equal(t, BasicKindString, expression.Target.Arguments[0].Arguments[0].Basic)
	require.Equal(t, BasicKindInt, expression.Target.Arguments[0].Arguments[1].Basic)
	require.Equal(t, TypeKindMap, expression.Target.Arguments[1].Kind)
	require.Equal(t, "Phantom", expression.Target.Arguments[1].Element.Element.Element.Named.Name)
	require.Len(t, expression.Target.Arguments[1].Element.Element.Element.Arguments, 2)
}

func TestTypeExpression_CanonicalAliasesAndDefinedTypes(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Original map[string]int
type Alias = Original
type Generic[A, B any] struct { Value A }
type GenericAlias[A, B any] = Generic[A, B]
type Defined map[string]int
`)

	original := semanticType(t, packageType(t, loadedPackage, "Original"))
	alias := semanticType(t, packageType(t, loadedPackage, "Alias"))
	defined := semanticType(t, packageType(t, loadedPackage, "Defined"))
	require.Equal(t, typeID(t, original), typeID(t, alias))
	require.NotEqual(t, typeID(t, original), typeID(t, defined))

	generic := instantiateType(t, loadedPackage, "Generic", types.Typ[types.String], types.Typ[types.Int])
	genericAlias := instantiateType(t, loadedPackage, "GenericAlias", types.Typ[types.String], types.Typ[types.Int])
	require.Equal(t, typeID(t, semanticType(t, generic)), typeID(t, semanticType(t, genericAlias)))

	phantomInt := instantiateType(t, loadedPackage, "Generic", types.Typ[types.String], types.Typ[types.Int])
	phantomBool := instantiateType(t, loadedPackage, "Generic", types.Typ[types.String], types.Typ[types.Bool])
	require.NotEqual(t, typeID(t, semanticType(t, phantomInt)), typeID(t, semanticType(t, phantomBool)))
}

func TestTypeExpression_JSONRoundTripAndComposites(t *testing.T) {
	loadedPackage := checkTypePackage(t, ""+
		"package semantic\n"+
		"type External interface { Read([]byte) (int, error) }\n"+
		"type Composite struct {\n"+
		"\tEmbedded\n"+
		"\tValues map[string]*[3]chan<- func(int, ...string) (<-chan bool, error) `json:\"values,omitempty\"`\n"+
		"}\n"+
		"type Embedded struct{}\n")

	expression := semanticType(t, packageType(t, loadedPackage, "Composite").Underlying())
	require.Equal(t, TypeKindStruct, expression.Kind)
	require.Len(t, expression.Fields, 2)
	require.True(t, expression.Fields[0].Embedded)
	require.Equal(t, `json:"values,omitempty"`, expression.Fields[1].Tag)
	require.Equal(t, TypeKindMap, expression.Fields[1].Type.Kind)
	require.Equal(t, TypeKindSignature, expression.Fields[1].Type.Element.Element.Element.Element.Kind)
	require.True(t, expression.Fields[1].Type.Element.Element.Element.Element.Variadic)

	encoded, err := json.Marshal(expression)
	require.NoError(t, err)
	var decoded TypeExpression
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, expression, &decoded)
	require.Equal(t, typeID(t, expression), typeID(t, &decoded))
}

func TestTypeExpressionOf_InterfaceAndTypeParameters(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Constraint interface { ~int | ~string; String() string }
type Generic[T Constraint] interface { Convert(T) (T, error) }
`)

	expression := semanticType(t, packageType(t, loadedPackage, "Generic").Underlying())
	require.Equal(t, TypeKindInterface, expression.Kind)
	require.Len(t, expression.Methods, 1)
	signature := expression.Methods[0].Signature
	require.Equal(t, TypeKindSignature, signature.Kind)
	require.Equal(t, TypeKindTypeParameter, signature.Parameters[0].Kind)
	require.Equal(t, "T", signature.Parameters[0].TypeParameter.Name)

	constraint := semanticType(t, packageType(t, loadedPackage, "Constraint").Underlying())
	require.Equal(t, TypeKindInterface, constraint.Kind)
	require.Len(t, constraint.Embedded, 1)
	require.Equal(t, TypeKindUnion, constraint.Embedded[0].Kind)
	require.Len(t, constraint.Embedded[0].Terms, 2)
	require.True(t, constraint.Embedded[0].Terms[0].Tilde)
}

func TestContainsTypeParameter_CompositeLocations(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Generic[T any] struct{}
func Function[T any](T) T { panic("unreachable") }
`)
	parameter := packageType(t, loadedPackage, "Generic").(*types.Named).TypeParams().At(0)
	function := loadedPackage.Scope().Lookup("Function").Type().(*types.Signature)
	tests := []struct {
		name   string
		goType types.Type
	}{
		{name: "map key", goType: types.NewMap(parameter, types.Typ[types.String])},
		{name: "struct field", goType: types.NewStruct([]*types.Var{types.NewField(0, loadedPackage, "Value", parameter, false)}, nil)},
		{name: "signature parameter and result", goType: function},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.True(t, containsTypeParameter(tt.goType))
		})
	}
}

func TestCollectRuntimeTypes_GenericHelperMixinAndDeterministicDedup(t *testing.T) {
	loadedPackage := loadTestPackage(t, "./testdata/semantic")
	first, err := CollectRuntimeTypes([]*packages.Package{loadedPackage})
	require.NoError(t, err)
	second, err := CollectRuntimeTypes([]*packages.Package{loadedPackage})
	require.NoError(t, err)
	require.Equal(t, runtimeTypeIdentity(first), runtimeTypeIdentity(second))
	require.True(t, slices.IsSortedFunc(first, func(a, b RuntimeType) int { return compareTypeID(a.ID, b.ID) }))
	for _, runtimeType := range first {
		require.Equal(t, typeID(t, runtimeType.Expression), runtimeType.ID)
		require.NotEmpty(t, runtimeType.Source)
		require.False(t, containsTypeParameter(runtimeType.GoType))
	}

	names := make([]string, len(first))
	for index, runtimeType := range first {
		names[index] = types.TypeString(runtimeType.GoType, func(imported *types.Package) string {
			return imported.Name()
		})
	}
	slices.Sort(names)
	require.Equal(t, []string{
		"*semantic.Link",
		"[]byte",
		"[]float64",
		"[]int",
		"[]schema_types.RestrictionType",
		"[]string",
		"any",
		"bool",
		"float32",
		"float64",
		"int",
		"int16",
		"int32",
		"int64",
		"int8",
		"json.RawMessage",
		"map[string]string",
		"semantic.BigInt",
		"semantic.DirectTime",
		"semantic.EncodedBytes",
		"semantic.Envelope[map[string][]*semantic.Phantom[url.URL, [2]int], chan<- func(url.URL) error]",
		"semantic.FormattedTime",
		"semantic.Link",
		"semantic.Phantom[url.URL, [3]bool]",
		"semantic.Status",
		"semantic.Validated",
		"string",
		"time.Duration",
		"time.Time",
		"uint",
		"uint16",
		"uint32",
		"uint64",
		"uint8",
		"uuid.UUID",
	}, names)
}

func TestCachedRuntimeType_ReusesAnalysisAndTracksEarliestSource(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Value struct{}
`)
	goType := packageType(t, loadedPackage, "Value")
	cache := make(map[types.Type]RuntimeType)
	first, err := cachedRuntimeType(cache, goType, "schema.go:100:2")
	require.NoError(t, err)
	second, err := cachedRuntimeType(cache, goType, "schema.go:9:20")
	require.NoError(t, err)
	third, err := cachedRuntimeType(cache, goType, "schema.go:9:3")
	require.NoError(t, err)
	require.Same(t, first.Expression, second.Expression)
	require.Same(t, second.Expression, third.Expression)
	require.Equal(t, "schema.go:9:3", third.Source)
	require.Equal(t, "schema.go:9:3", cache[goType].Source)
}

func TestTypeExpression_FreeTypeParameterIdentityIsPositionIndependent(t *testing.T) {
	first := freeTypeParameterExpression(t, `package semantic; type Generic[T any] struct{ Value T }`)
	second := freeTypeParameterExpression(t, `package semantic









type Generic[T any] struct{ Value T }`)
	require.Equal(t, "type:Generic", first.TypeParameter.Owner)
	require.Equal(t, 0, first.TypeParameter.Index)
	require.Equal(t, typeID(t, first), typeID(t, second))
}

func TestTypeCompatibility_StringRepresentations(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type DefinedString string

type Priority int
func (Priority) String() string { return "priority" }
`)

	defined := typeCompatibility(packageType(t, loadedPackage, "DefinedString"))[field.TypeString.String()]
	require.False(t, defined.Assignable)
	require.True(t, defined.Convertible)
	require.True(t, defined.ReverseConvertible)
	require.Empty(t, defined.Projection)

	priority := typeCompatibility(packageType(t, loadedPackage, "Priority"))[field.TypeEnum.String()]
	require.False(t, priority.Assignable)
	require.False(t, priority.Convertible)
	require.False(t, priority.ReverseConvertible)
	require.Equal(t, "String()", priority.Projection)
}

func TestCollectRuntimeTypes_Capabilities(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type BigInt struct{}
func (BigInt) Add(BigInt) BigInt { return BigInt{} }

type Raw []byte
type ID [64]byte

type Link struct{}
func (*Link) Scan(any) error { return nil }
`)

	bigInt := packageType(t, loadedPackage, "BigInt")
	capabilities := typeCapabilities(bigInt)
	require.True(t, capabilities.Adder)
	require.Equal(t, TypeKindStruct, capabilities.Underlying.Kind)

	rawType := packageType(t, loadedPackage, "Raw")
	raw := typeCapabilities(rawType)
	require.Equal(t, TypeKindSlice, raw.Underlying.Kind)
	require.Equal(t, BasicKindByte, raw.Underlying.Element.Basic)
	require.True(t, typeCompatibility(rawType)[field.TypeBytes.String()].Convertible)

	idCompatibility := typeCompatibility(packageType(t, loadedPackage, "ID"))[field.TypeBytes.String()]
	require.False(t, idCompatibility.Assignable)
	require.False(t, idCompatibility.Convertible)
	require.True(t, idCompatibility.ReverseConvertible)
	require.Equal(t, "[:]", idCompatibility.Projection)

	linkType := types.NewPointer(packageType(t, loadedPackage, "Link"))
	link := typeCapabilities(linkType)
	require.True(t, link.Scanner)
	require.False(t, link.Valuer)
	require.True(t, link.NullableScanner)
}

func TestTypeCapabilities_ExactInterfacesRejectLookalikes(t *testing.T) {
	loadedPackage := checkTypePackage(t, `
package semantic

type Exact struct{}
func (*Exact) Scan(any) error { return nil }
func (Exact) Validate() error { return nil }
func (Exact) String() string { return "" }
func (Exact) Add(Exact) Exact { return Exact{} }

type LookalikeScanner struct{}
func (*LookalikeScanner) Scan(string) error { return nil }

type LookalikeValuer struct{}
func (LookalikeValuer) Value() (string, error) { return "", nil }

type LookalikeValidator struct{}
func (LookalikeValidator) Validate() string { return "" }

type LookalikeStringer struct{}
func (LookalikeStringer) String() any { return "" }

type LookalikeAdder struct{}
func (LookalikeAdder) Add(string) LookalikeAdder { return LookalikeAdder{} }
`)

	exactType := packageType(t, loadedPackage, "Exact")
	exact := typeCapabilities(types.NewPointer(exactType))
	require.True(t, exact.Scanner)
	require.False(t, exact.Valuer)
	require.True(t, exact.Validator)
	require.True(t, exact.Stringer)
	require.False(t, exact.Adder, "Add is declared on Exact, not *Exact")
	require.True(t, typeCapabilities(exactType).Adder)

	require.False(t, typeCapabilities(types.NewPointer(packageType(t, loadedPackage, "LookalikeScanner"))).Scanner)
	require.False(t, typeCapabilities(packageType(t, loadedPackage, "LookalikeValuer")).Valuer)
	require.False(t, typeCapabilities(packageType(t, loadedPackage, "LookalikeValidator")).Validator)
	require.False(t, typeCapabilities(packageType(t, loadedPackage, "LookalikeStringer")).Stringer)
	require.False(t, typeCapabilities(packageType(t, loadedPackage, "LookalikeAdder")).Adder)
}

func TestTypeCapabilities_DatabaseInterfacesUsePackageIdentity(t *testing.T) {
	loadedPackage := loadTestPackage(t, "./testdata/semantic")
	link := typeCapabilities(types.NewPointer(loadedPackage.Types.Scope().Lookup("Link").Type()))
	require.True(t, link.Scanner)
	require.True(t, link.Valuer)

	uuid := typeCapabilities(loadedPackage.Imports["github.com/google/uuid"].Types.Scope().Lookup("UUID").Type())
	require.True(t, uuid.Scanner)
	require.True(t, uuid.Valuer)
}

func TestCollectRuntimeTypes_BuiltinRegistry(t *testing.T) {
	runtimeTypes, err := CollectRuntimeTypes([]*packages.Package{loadTestPackage(t, "./testdata/valid")})
	require.NoError(t, err)
	require.NotEmpty(t, runtimeTypes)
}

func runtimeTypeIdentity(runtimeTypes []RuntimeType) []struct {
	ID         TypeID
	Expression *TypeExpression
	Source     string
} {
	identity := make([]struct {
		ID         TypeID
		Expression *TypeExpression
		Source     string
	}, len(runtimeTypes))
	for index, runtimeType := range runtimeTypes {
		identity[index] = struct {
			ID         TypeID
			Expression *TypeExpression
			Source     string
		}{runtimeType.ID, runtimeType.Expression, runtimeType.Source}
	}
	return identity
}

func loadTestPackage(t *testing.T, pattern string) *packages.Package {
	t.Helper()
	loadedPackages, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes,
	}, pattern)
	require.NoError(t, err)
	require.Len(t, loadedPackages, 1)
	require.Empty(t, loadedPackages[0].Errors)
	return loadedPackages[0]
}

func checkTypePackage(t *testing.T, source string) *types.Package {
	t.Helper()
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "semantic.go", source, parser.AllErrors)
	require.NoError(t, err)
	loadedPackage, err := (&types.Config{Importer: importer.Default()}).Check("example.com/semantic", fileset, []*ast.File{file}, nil)
	require.NoError(t, err)
	return loadedPackage
}

func packageType(t *testing.T, loadedPackage *types.Package, name string) types.Type {
	t.Helper()
	object, ok := loadedPackage.Scope().Lookup(name).(*types.TypeName)
	require.True(t, ok)
	return object.Type()
}

func instantiateType(t *testing.T, loadedPackage *types.Package, name string, arguments ...types.Type) types.Type {
	t.Helper()
	instantiated, err := types.Instantiate(nil, packageType(t, loadedPackage, name), arguments, true)
	require.NoError(t, err)
	return instantiated
}

func semanticType(t *testing.T, goType types.Type) *TypeExpression {
	t.Helper()
	expression, err := TypeExpressionOf(goType)
	require.NoError(t, err)
	return expression
}

func freeTypeParameterExpression(t *testing.T, source string) *TypeExpression {
	t.Helper()
	named := packageType(t, checkTypePackage(t, source), "Generic").(*types.Named)
	return semanticType(t, named.TypeParams().At(0))
}

func typeID(t *testing.T, goType *TypeExpression) TypeID {
	t.Helper()
	id, err := goType.ID()
	require.NoError(t, err)
	return id
}
