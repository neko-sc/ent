// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"testing"

	"github.com/neko-sc/ent/entc/load"
	"github.com/neko-sc/ent/schema/field"

	"github.com/stretchr/testify/require"
)

func TestNewType_RejectsInvalidSemanticMetadata(t *testing.T) {
	tests := []struct {
		name     string
		semantic func() *load.FieldType
		error    string
	}{
		{
			name:  "missing metadata",
			error: `field "value" is missing semantic type metadata`,
		},
		{
			name: "missing logical base",
			semantic: func() *load.FieldType {
				semantic := builtinFieldType(field.TypeString)
				semantic.Base = nil
				return semantic
			},
			error: `field "value" semantic type is missing its logical base`,
		},
		{
			name: "missing representation",
			semantic: func() *load.FieldType {
				semantic := builtinFieldType(field.TypeString)
				semantic.Representation = nil
				return semantic
			},
			error: `field "value" semantic type is missing its representation`,
		},
		{
			name: "identity mismatch",
			semantic: func() *load.FieldType {
				semantic := builtinFieldType(field.TypeString)
				semantic.ID = "other"
				return semantic
			},
			error: `field "value" semantic type identity mismatch`,
		},
		{
			name: "logical mismatch",
			semantic: func() *load.FieldType {
				return builtinFieldType(field.TypeInt)
			},
			error: `field "value" semantic logical type "int" does not match "string"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var semantic *load.FieldType
			if tt.semantic != nil {
				semantic = tt.semantic()
			}
			_, err := NewType(&Config{}, &load.Schema{Name: "Record", Fields: []*load.Field{{Name: "value", Type: field.TypeString, Semantic: semantic}}})
			require.EqualError(t, err, tt.error)
		})
	}
}

func TestNewType_AcceptsByteArrayProjectionForFamilyValidators(t *testing.T) {
	semantic, err := load.FieldTypeOf(field.TypeBytes, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "ID"},
	})
	require.NoError(t, err)
	semantic.Capabilities.LogicalProjection = "[:]"
	typeInfo, err := NewType(&Config{}, &load.Schema{Name: "Record", Fields: []*load.Field{{
		Name:           "id",
		Type:           field.TypeBytes,
		Semantic:       semantic,
		Validators:     1,
		ValidatorKinds: []field.ValidatorKind{field.ValidatorLogical},
	}}})
	require.NoError(t, err)
	require.Equal(t, "value[:]", typeInfo.ID.BasicType("value"))
}

func TestNewType_AcceptsDefinedBytesForFamilyValidators(t *testing.T) {
	semantic, err := load.FieldTypeOf(field.TypeBytes, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "Bytes"},
	})
	require.NoError(t, err)
	semantic.Capabilities.ConvertibleToLogical = true
	_, err = NewType(&Config{}, &load.Schema{Name: "Record", Fields: []*load.Field{{
		Name:           "value",
		Type:           field.TypeBytes,
		Semantic:       semantic,
		Validators:     1,
		ValidatorKinds: []field.ValidatorKind{field.ValidatorLogical},
	}}})
	require.NoError(t, err)
}

func TestFieldOps_SemanticLogicalProjection(t *testing.T) {
	bytesSemantic := builtinFieldType(field.TypeBytes)
	bytesSemantic.Capabilities.AssignableToLogical = false
	bytesSemantic.Capabilities.ConvertibleToLogical = false
	bytesSemantic.Capabilities.LogicalProjection = "[:]"
	bytesOps := fieldOps(&Field{Type: field.TypeBytes, Semantic: bytesSemantic})
	require.Contains(t, bytesOps, EQ)
	require.Contains(t, bytesOps, In)
	require.NotContains(t, bytesOps, GT)

	timeOps := fieldOps(&Field{Type: field.TypeTime, Semantic: builtinFieldType(field.TypeTime)})
	require.Contains(t, timeOps, EQ)
	require.Contains(t, timeOps, GT)

	otherSemantic := builtinFieldType(field.TypeOther)
	otherSemantic.Capabilities.Valuer = true
	otherOps := fieldOps(&Field{Name: "id", Type: field.TypeOther, Semantic: otherSemantic})
	require.Contains(t, otherOps, EQ)
	require.Contains(t, otherOps, In)
	require.NotContains(t, otherOps, GT)
}

func TestFieldOps_StringID(t *testing.T) {
	drivers := []*Storage{
		{
			Ops: func(f *Field) []Op {
				if f.IsString() && f.RepresentationIsBase() {
					return []Op{EqualFold, ContainsFold}
				}
				return nil
			},
		},
	}
	alias, err := load.FieldTypeOf(field.TypeString, &load.TypeExpression{
		Kind:   load.TypeKindAlias,
		Alias:  &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "IDAlias"},
		Target: builtinFieldType(field.TypeString).Base,
	})
	require.NoError(t, err)
	alias.Capabilities.AssignableToLogical = true
	defined := func() *load.FieldType {
		semantic, err := load.FieldTypeOf(field.TypeString, &load.TypeExpression{
			Kind:  load.TypeKindNamed,
			Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "ID"},
		})
		require.NoError(t, err)
		semantic.Capabilities.ConvertibleToLogical = true
		return semantic
	}
	unsupported := defined()
	unsupported.Capabilities.ConvertibleToLogical = false
	codecOnly := defined()
	codecOnly.Capabilities.ConvertibleToLogical = false
	tests := []struct {
		name       string
		field      *Field
		contains   []Op
		notContain []Op
	}{
		{
			name:       "built-in",
			field:      &Field{Name: "id", Type: field.TypeString, Semantic: builtinFieldType(field.TypeString)},
			contains:   []Op{EQ, In, GT, GTE, LT, LTE, EqualFold, ContainsFold},
			notContain: []Op{Contains, HasPrefix, HasSuffix},
		},
		{
			name:       "alias",
			field:      &Field{Name: "id", Type: field.TypeString, Semantic: alias},
			contains:   []Op{EQ, In, GT, GTE, LT, LTE, EqualFold, ContainsFold},
			notContain: []Op{Contains, HasPrefix, HasSuffix},
		},
		{
			name:       "defined",
			field:      &Field{Name: "id", Type: field.TypeString, Semantic: defined()},
			contains:   []Op{EQ, In, GT, GTE, LT, LTE},
			notContain: []Op{EqualFold, Contains, ContainsFold, HasPrefix, HasSuffix},
		},
		{
			name:       "unsupported",
			field:      &Field{Name: "id", Type: field.TypeString, Semantic: unsupported},
			notContain: []Op{EQ, In, GT, GTE, LT, LTE, EqualFold, ContainsFold},
		},
		{
			name:       "codec only",
			field:      &Field{Name: "id", Type: field.TypeString, Semantic: codecOnly, def: &load.Field{ValueScanner: true}},
			notContain: []Op{EQ, In, GT, GTE, LT, LTE, EqualFold, Contains, ContainsFold, HasPrefix, HasSuffix},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.field.cfg = &Config{Storage: drivers[0]}
			ops := tt.field.Ops()
			for _, op := range tt.contains {
				require.Contains(t, ops, op)
			}
			for _, op := range tt.notContain {
				require.NotContains(t, ops, op)
			}
		})
	}
}

func TestField_BasicTypePrefersStringProjection(t *testing.T) {
	semantic, err := load.FieldTypeOf(field.TypeEnum, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "Priority"},
	})
	require.NoError(t, err)
	semantic.Capabilities.LogicalProjection = "String()"
	require.Equal(t, "value.String()", Field{Type: field.TypeEnum, Semantic: semantic}.BasicType("value"))
}

func TestField_DefinesEnumType(t *testing.T) {
	builtin := Field{Type: field.TypeEnum, Semantic: builtinFieldType(field.TypeEnum)}
	require.True(t, builtin.DefinesEnumType())

	customSemantic, err := load.FieldTypeOf(field.TypeEnum, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "Status"},
	})
	require.NoError(t, err)
	require.False(t, Field{Type: field.TypeEnum, Semantic: customSemantic}.DefinesEnumType())
}

func TestField_EnumValidatorName(t *testing.T) {
	fieldInfo := Field{Name: "state"}
	require.Equal(t, "StateValidator", fieldInfo.EnumValidator())

	fieldInfo.Validators = 1
	require.Equal(t, "StateValuesValidator", fieldInfo.EnumValidator())
}

func TestNewType_RejectsNonStringEnumDefault(t *testing.T) {
	semantic, err := load.FieldTypeOf(field.TypeEnum, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "State"},
	})
	require.NoError(t, err)
	_, err = NewType(&Config{}, &load.Schema{Name: "Record", Fields: []*load.Field{{
		Name:         "state",
		Type:         field.TypeEnum,
		Semantic:     semantic,
		Default:      true,
		DefaultValue: 1,
		Enums:        []struct{ N, V string }{{N: "on", V: "on"}},
	}}})
	require.EqualError(t, err, `enum field "state" default cannot be represented as a string enum value`)
}

func TestField_RepresentationIsBase(t *testing.T) {
	base := Field{Type: field.TypeString, Semantic: builtinFieldType(field.TypeString)}
	require.True(t, base.RepresentationIsBase())

	aliasSemantic, err := load.FieldTypeOf(field.TypeString, &load.TypeExpression{
		Kind:   load.TypeKindAlias,
		Alias:  &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "Alias"},
		Target: base.Semantic.Base,
	})
	require.NoError(t, err)
	require.True(t, Field{Type: field.TypeString, Semantic: aliasSemantic}.RepresentationIsBase())

	definedSemantic, err := load.FieldTypeOf(field.TypeString, &load.TypeExpression{
		Kind:  load.TypeKindNamed,
		Named: &load.TypeName{Package: load.Package{Path: "example.com/schema", Name: "schema"}, Name: "Defined"},
	})
	require.NoError(t, err)
	require.False(t, Field{Type: field.TypeString, Semantic: definedSemantic}.RepresentationIsBase())
}

func TestType(t *testing.T) {
	require := require.New(t)
	typ, err := NewType(&Config{Package: "entc/gen"}, T1)
	require.NoError(err)
	require.NotNil(typ)
	require.Equal("T1", typ.Name)
	require.Equal("t1", typ.Label())
	require.Equal("t1", typ.Package())
	require.Equal("_m", typ.Receiver())

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Fields: []*load.Field{
			{Name: "foo", Unique: true, Default: true, Type: field.TypeInt, Semantic: builtinFieldType(field.TypeInt)},
		},
	})
	require.EqualError(err, "unique field \"foo\" cannot have default value", "unique field can not have default")

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Fields: []*load.Field{
			{Name: "foo", Sensitive: true, Tag: `yaml:"pwd"`, Type: field.TypeString, Semantic: builtinFieldType(field.TypeString)},
		},
	})
	require.EqualError(err, "sensitive field \"foo\" cannot have struct tags", "sensitive field cannot have tags")

	typ, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Fields: []*load.Field{
			{Name: "id", Type: field.TypeString, Semantic: builtinFieldType(field.TypeString), Annotations: dict("EntSQL", dict("collation", "utf8_ci_bin"))},
		},
	})
	require.NoError(err)
	require.NotNil(typ)
	require.NotNil(t, typ.ID)
	pkCol := typ.ID.PK()
	require.NotNil(pkCol)
	require.Equal("utf8_ci_bin", pkCol.Collation)

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "foo", Unique: true, Type: field.TypeInt, Semantic: builtinFieldType(field.TypeInt)},
			{Name: "foo", Unique: true, Type: field.TypeInt, Semantic: builtinFieldType(field.TypeInt)},
		},
	})
	require.EqualError(err, "field \"foo\" redeclared for type \"T\"", "field foo redeclared")

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "enums", Type: field.TypeEnum, Semantic: builtinFieldType(field.TypeEnum), Enums: []struct{ N, V string }{{V: "v"}, {V: "v"}}},
		},
	})
	require.EqualError(err, "duplicate values \"v\" for enum field \"enums\"", "duplicate enums")

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "enums", Type: field.TypeEnum, Semantic: builtinFieldType(field.TypeEnum), Enums: []struct{ N, V string }{{}}},
		},
	})
	require.EqualError(err, "\"enums\" field value cannot be empty", "empty value for enums")

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "", Type: field.TypeInt, Semantic: builtinFieldType(field.TypeInt)},
		},
	})
	require.EqualError(err, "field name cannot be empty", "empty field name")

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "id", Type: field.TypeInt, Semantic: builtinFieldType(field.TypeInt), Optional: true},
		},
	})
	require.EqualError(err, "id field cannot be optional", "id field cannot be optional")

	typ, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{
		Name: "T",
		Fields: []*load.Field{
			{Name: "id", Type: field.TypeString, Semantic: builtinFieldType(field.TypeString), ValueScanner: true},
		},
	})
	require.NoError(err)
	require.True(typ.HasValueScanner())

	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{Name: "Type"})
	require.EqualError(err, "schema lowercase name conflicts with Go keyword \"type\"")
	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{Name: "Int"})
	require.EqualError(err, "schema lowercase name conflicts with Go predeclared identifier \"int\"")
	_, err = NewType(&Config{Package: "entc/gen"}, &load.Schema{Name: "Value"})
	require.EqualError(err, "schema name conflicts with ent predeclared identifier \"Value\"")
}

func TestType_Label(t *testing.T) {
	tests := []struct {
		name  string
		label string
	}{
		{"User", "user"},
		{"UserInfo", "user_info"},
		{"PHBOrg", "phb_org"},
		{"UserID", "user_id"},
		{"HTTPCode", "http_code"},
		{"UserIDs", "user_ids"},
	}
	for _, tt := range tests {
		typ := &Type{Name: tt.name}
		require.Equal(t, tt.label, typ.Label())
	}
}

func TestType_Table(t *testing.T) {
	tests := []struct {
		name  string
		label string
	}{
		{"User", "users"},
		{"Device", "devices"},
		{"UserInfo", "user_infos"},
		{"PHBOrg", "phb_orgs"},
		{"HTTPCode", "http_codes"},
	}
	for _, tt := range tests {
		typ := &Type{Name: tt.name}
		require.Equal(t, tt.label, typ.Table())
	}
}

func TestField_EnumName(t *testing.T) {
	tests := []struct {
		name string
		enum string
	}{
		{"GIF", "TypeGIF"},
		{"SVG", "TypeSVG"},
		{"PNG", "TypePNG"},
		{"MP4", "TypeMP4"},
		{"unknown", "TypeUnknown"},
		{"user_data", "TypeUserData"},
		{"test user", "TypeTestUser"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.enum, Field{Name: "Type"}.EnumName(tt.name))
	}
}

func TestType_HasUpdateCheckersWithNativeValidator(t *testing.T) {
	semantic := builtinFieldType(field.TypeString)
	semantic.Capabilities.Validator = true
	typeInfo := &Type{Fields: []*Field{{Type: field.TypeString, Semantic: semantic}}}
	require.True(t, typeInfo.HasUpdateCheckers())
}

func TestType_WithRuntimeMixin(t *testing.T) {
	position := &load.Position{MixedIn: true}
	typ := &Type{
		ID: &Field{},
		Fields: []*Field{
			{Default: true, Position: position},
			{UpdateDefault: true, Position: position},
			{Validators: 1, Position: position},
		},
	}
	require.True(t, typ.RuntimeMixin())
}

func TestType_TagTypes(t *testing.T) {
	typ := &Type{
		Fields: []*Field{
			{StructTag: `json:"age"`},
			{StructTag: `json:"name,omitempty`},
			{StructTag: `json:"name,omitempty" sql:"nothing"`},
			{StructTag: `sql:"nothing" yaml:"ignore"`},
			{StructTag: `sql:"nothing" yaml:"ignore"`},
			{StructTag: `invalid`},
			{StructTag: `"invalid"`},
		},
	}
	tags := typ.TagTypes()
	require.Equal(t, []string{"json", "sql", "yaml"}, tags)
}

func TestType_Package(t *testing.T) {
	tests := []struct {
		name string
		pkg  string
	}{
		{"User", "user"},
		{"UserInfo", "userinfo"},
		{"PHBOrg", "phborg"},
		{"UserID", "userid"},
		{"HTTPCode", "httpcode"},
	}
	for _, tt := range tests {
		typ := &Type{Name: tt.name}
		require.Equal(t, tt.pkg, typ.Package())
	}
}

func TestType_AddIndex(t *testing.T) {
	size := int64(1024)
	typ, err := NewType(&Config{}, &load.Schema{
		Name: "User",
		Fields: []*load.Field{
			{Name: "name", Type: field.TypeString, Semantic: builtinFieldType(field.TypeString)},
			{Name: "text", Type: field.TypeString, Semantic: builtinFieldType(field.TypeString), Size: &size},
		},
	})
	require.NoError(t, err)
	typ.Edges = append(typ.Edges,
		&Edge{Name: "next", Rel: Relation{Type: O2O, Columns: []string{"prev_id"}}},
		&Edge{Name: "prev", Inverse: "next", Rel: Relation{Type: O2O, Columns: []string{"prev_id"}}},
		&Edge{Name: "owner", Inverse: "files", Rel: Relation{Type: M2O, Columns: []string{"file_id"}}},
	)

	err = typ.AddIndex(&load.Index{Unique: true})
	require.Error(t, err, "missing fields or edges")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"unknown"}})
	require.Error(t, err, "unknown field for index")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"id"}})
	require.NoError(t, err, "valid index for ID field")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"name"}, Edges: []string{"parent"}})
	require.Error(t, err, "missing edge")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"name"}, Edges: []string{"next"}})
	require.Error(t, err, "not an inverse edge for O2O relation")

	err = typ.AddIndex(&load.Index{Unique: true, Edges: []string{"prev", "owner"}})
	require.NoError(t, err, "valid index defined only on edges")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"name"}, Edges: []string{"prev"}})
	require.NoError(t, err, "valid index on O2O relation and field")

	err = typ.AddIndex(&load.Index{Unique: true, Fields: []string{"name"}, Edges: []string{"owner"}})
	require.NoError(t, err, "valid index on M2O relation and field")
}

func TestField_Constant(t *testing.T) {
	tests := []struct {
		name     string
		constant string
	}{
		{"user", "FieldUser"},
		{"user_id", "FieldUserID"},
		{"user_name", "FieldUserName"},
	}
	for _, tt := range tests {
		typ := &Field{Name: tt.name}
		require.Equal(t, tt.constant, typ.Constant())
	}
}

func TestField_DefaultName(t *testing.T) {
	tests := []struct {
		name     string
		constant string
	}{
		{"active", "DefaultActive"},
		{"expired_at", "DefaultExpiredAt"},
		{"group_name", "DefaultGroupName"},
	}
	for _, tt := range tests {
		typ := &Field{Name: tt.name}
		require.Equal(t, tt.constant, typ.DefaultName())
	}
}

func TestField_incremental(t *testing.T) {
	tests := []struct {
		annotations map[string]any
		def         bool
		expected    bool
	}{
		{dict("EntSQL", nil), false, false},
		{dict("EntSQL", nil), true, true},
		{dict("EntSQL", dict("incremental", true)), false, true},
		{dict("EntSQL", dict("incremental", false)), true, false},
	}
	for _, tt := range tests {
		typ := &Field{Annotations: tt.annotations}
		require.Equal(t, tt.expected, typ.incremental(tt.def))
	}
}

func TestBuilderField(t *testing.T) {
	tests := []struct {
		name  string
		field string
	}{
		{"active", "active"},
		{"type", "_type"},
		{"config", "_config"},
		{"SSOCert", "_SSOCert"},
		{"driver", "_driver"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.field, Edge{Name: tt.name}.BuilderField())
		require.Equal(t, tt.field, Field{Name: tt.name}.BuilderField())
	}
}

func TestEdge(t *testing.T) {
	u, g := &Type{Name: "User"}, &Type{Name: "Group"}
	groups := &Edge{Name: "groups", Type: g, Owner: u, Rel: Relation{Type: M2M}}
	users := &Edge{Name: "users", Inverse: "groups", Type: u, Owner: u, Rel: Relation{Type: M2M}}

	require.True(t, users.IsInverse())
	require.False(t, groups.IsInverse())

	require.Equal(t, "GroupsLabel", users.LabelConstant())
	require.Equal(t, "GroupsLabel", groups.LabelConstant())

	require.Equal(t, "UsersInverseLabel", users.InverseLabelConstant())
	require.Equal(t, "user_groups", users.Label())
	require.Equal(t, "user_groups", groups.Label())
}

func TestValidSchemaName(t *testing.T) {
	err := ValidSchemaName("Config")
	require.Error(t, err)
	err = ValidSchemaName("Mutation")
	require.Error(t, err)
	err = ValidSchemaName("Boring")
	require.NoError(t, err)
	err = ValidSchemaName("Order")
	require.NoError(t, err)
}
