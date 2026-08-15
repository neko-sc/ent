// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"path"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/neko-sc/atlas/sql/postgres"
	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/entsql"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/neko-sc/ent/entc/load"
	entschema "github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
)

// The following types and their exported methods used by the codegen
// to generate the assets.
type (
	// Type represents one node-type in the graph, its relations and
	// the information it holds.
	Type struct {
		*Config
		schema *load.Schema
		// Name holds the type/ent name.
		Name string
		// alias, or local package name of the generated package.
		// Empty means no alias.
		alias string
		// ID holds the ID field of this type.
		ID *Field
		// Fields holds all the primitive fields of this type.
		Fields []*Field
		fields map[string]*Field
		// Edge holds all the edges of this type.
		Edges []*Edge
		// Indexes are the configured indexes for this type.
		Indexes []*Index
		// ForeignKeys are the foreign-keys that resides in the type table.
		ForeignKeys []*ForeignKey
		foreignKeys map[string]struct{}
		// Annotations that were defined for the field in the schema.
		// The mapping is from the Annotation.Name() to a JSON decoded object.
		Annotations     Annotations
		semanticImports []Import
		// EdgeSchema indicates that this type (schema) is being used as an "edge schema".
		// The To and From fields holds references to the edges that go "through" this type.
		EdgeSchema struct {
			ID       []*Field
			To, From *Edge
		}
	}

	// Field holds the information of a type field used for the templates.
	Field struct {
		cfg *Config
		def *load.Field
		typ *Type
		// Name is the name of this field in the database schema.
		Name string
		// Type holds the logical field family.
		Type field.Type
		// Semantic holds compiler-authored type information for typed fields.
		Semantic *load.FieldType
		// Unique indicate if this field is a unique field.
		Unique bool
		// Optional indicates is this field is optional on create.
		Optional bool
		// Nillable indicates that this field can be null in the
		// database and pointer in the generated entities.
		Nillable bool
		// Default indicates if this field has a default value for creation.
		Default bool
		// Enums information for enum fields.
		Enums []Enum
		// UpdateDefault indicates if this field has a default value for update.
		UpdateDefault bool
		// Immutable indicates is this field cannot be updated.
		Immutable bool
		// StructTag of the field. default to "json".
		StructTag string
		// Validators holds the number of validators the field has.
		Validators int
		// ValidatorKinds identifies each validator's accepted value in fluent call order.
		ValidatorKinds []field.ValidatorKind
		// Position info of the field.
		Position *load.Position
		// UserDefined indicates that this field was defined explicitly by the user in
		// the schema. Unlike the default id field, which is defined by the generator.
		UserDefined bool
		// Annotations that were defined for the field in the schema.
		// The mapping is from the Annotation.Name() to a JSON decoded object.
		Annotations  Annotations
		semanticType string
		baseType     string
		// referenced foreign-key.
		fk *ForeignKey
	}

	// Edge of a graph between two types.
	Edge struct {
		def *load.Edge
		// Name holds the name of the edge.
		Name string
		// Type holds a reference to the type this edge is directed to.
		Type *Type
		// Optional indicates is this edge is optional on create.
		Optional bool
		// Immutable indicates is this edge cannot be updated.
		Immutable bool
		// Unique indicates if this edge is a unique edge.
		Unique bool
		// Inverse holds the name of the reference edge declared in the schema.
		Inverse string
		// Ref points to the reference edge. For Inverse edges (edge.From),
		// its points to the Assoc (edge.To). For Assoc edges, it points to
		// the inverse edge if it exists.
		Ref *Edge
		// Owner holds the type of the edge-owner. For assoc-edges it's the
		// type that holds the edge, for inverse-edges, it's the assoc type.
		Owner *Type
		// Through edge schema type.
		Through *Type
		// StructTag of the edge-field in the struct. default to "json".
		StructTag string
		// Relation holds the relation info of an edge.
		Rel Relation
		// Bidi indicates if this edge is a bidirectional edge. A self-reference
		// to the same type with the same name (symmetric relation). For example,
		// a User type have one of following edges:
		//
		//	edge.To("friends", User.Type)           // many 2 many.
		//	edge.To("spouse", User.Type).Unique()   // one 2 one.
		//
		Bidi bool
		// Annotations that were defined for the edge in the schema.
		// The mapping is from the Annotation.Name() to a JSON decoded object.
		Annotations Annotations
	}

	// Relation holds the relational database information for edges.
	Relation struct {
		// Type holds the relation type of the edge.
		Type Rel
		// Table holds the relation table for this edge.
		// For O2O and O2M, it's the table name of the type we're this edge point to.
		// For M2O, this is the owner's type, and for M2M this is the join table.
		Table string
		// Columns holds the relation column(s) in the relation table above.
		// For O2M, M2O and O2O, it contains one element with the column name.
		// For M2M edges, it contains two columns defined in the join table with
		// the same order as defined in the schema: (owner_id, reference_id).
		Columns []string
		// foreign-key information for non-M2M edges.
		fk *ForeignKey
	}

	// Index represents a database index used for either increasing speed
	// on database operations or defining constraints such as "UNIQUE INDEX".
	// Note that some indexes are created implicitly like table foreign keys.
	Index struct {
		// Name of the index. One column index is simply the column name.
		Name string
		// Unique index or not.
		Unique bool
		// Columns are the table columns.
		Columns []string
		// Annotations that were defined for the index in the schema.
		// The mapping is from the Annotation.Name() to a JSON decoded object.
		Annotations Annotations
	}

	// ForeignKey holds the information for foreign-key columns of types.
	// It's exported only because it's used by the codegen templates and
	// should not be used beside that.
	ForeignKey struct {
		// Field information for the foreign-key column.
		Field *Field
		// Edge that is associated with this foreign-key.
		Edge *Edge
		// UserDefined indicates that this foreign-key was defined explicitly as a field in the schema,
		// and was referenced by an edge. For example:
		//
		//	field.Int("owner_id").
		//		Optional()
		//
		//	edge.From("owner", User.Type).
		//		Ref("pets").
		//		Field("owner_id")
		//
		UserDefined bool
	}
	// Enum holds the enum information for schema enums in codegen.
	Enum struct {
		// Name is the Go name of the enum.
		Name string
		// Value in the schema.
		Value string
	}
)

// NewType creates a new type and its fields from the given schema.
func NewType(c *Config, schema *load.Schema) (*Type, error) {
	idType := field.TypeInt
	if c.IDType != nil {
		idType = *c.IDType
	}
	if !idType.Valid() {
		return nil, fmt.Errorf("invalid id type %q", idType)
	}
	typ := &Type{
		Config:      c,
		schema:      schema,
		Name:        schema.Name,
		Annotations: schema.Annotations,
		Fields:      make([]*Field, 0, len(schema.Fields)),
		fields:      make(map[string]*Field, len(schema.Fields)),
		foreignKeys: make(map[string]struct{}),
	}
	if !typ.IsView() {
		typ.ID = &Field{
			cfg:  c,
			typ:  typ,
			Name: "id",
			def: &load.Field{
				Name: "id",
			},
			Type:      idType,
			Semantic:  builtinFieldType(idType),
			StructTag: structTag("id", ""),
		}
	}
	if err := ValidSchemaName(typ.Name); err != nil {
		return nil, err
	}
	for _, f := range schema.Fields {
		tf := &Field{
			cfg:            c,
			def:            f,
			typ:            typ,
			Name:           f.Name,
			Type:           f.Type,
			Semantic:       f.Semantic,
			Unique:         f.Unique,
			Position:       f.Position,
			Nillable:       f.Nillable,
			Optional:       f.Optional,
			Default:        f.Default,
			UpdateDefault:  f.UpdateDefault,
			Immutable:      f.Immutable,
			StructTag:      structTag(f.Name, f.Tag),
			Validators:     f.Validators,
			ValidatorKinds: append([]field.ValidatorKind(nil), f.ValidatorKinds...),
			UserDefined:    true,
			Annotations:    f.Annotations,
		}
		if err := validateFieldSemantic(tf.Name, tf.Type, tf.Semantic); err != nil {
			return nil, err
		}
		if err := typ.checkField(tf, f); err != nil {
			return nil, err
		}
		// User defined id field.
		if typ.ID != nil && tf.Name == typ.ID.Name {
			if tf.Optional {
				return nil, errors.New("id field cannot be optional")
			}
			typ.ID = tf
		} else {
			typ.Fields = append(typ.Fields, tf)
			typ.fields[f.Name] = tf
		}
	}
	if typ.ID != nil {
		if err := validateFieldSemantic(typ.ID.Name, typ.ID.Type, typ.ID.Semantic); err != nil {
			return nil, err
		}
	}
	return typ, nil
}

func validateFieldSemantic(name string, logical field.Type, semantic *load.FieldType) error {
	if !logical.Valid() {
		return fmt.Errorf("field %q has invalid logical type %q", name, logical)
	}
	if semantic == nil {
		return fmt.Errorf("field %q is missing semantic type metadata", name)
	}
	if semantic.Base == nil {
		return fmt.Errorf("field %q semantic type is missing its logical base", name)
	}
	if _, err := semantic.Base.ID(); err != nil {
		return fmt.Errorf("field %q semantic logical base: %w", name, err)
	}
	if semantic.Representation == nil {
		return fmt.Errorf("field %q semantic type is missing its representation", name)
	}
	id, err := semantic.Representation.ID()
	if err != nil {
		return fmt.Errorf("field %q semantic representation: %w", name, err)
	}
	if semantic.ID != id {
		return fmt.Errorf("field %q semantic type identity mismatch", name)
	}
	if semantic.Logical != fieldLogicalType(logical) {
		return fmt.Errorf("field %q semantic logical type %q does not match %q", name, semantic.Logical, logical)
	}
	return nil
}

func fieldLogicalType(typ field.Type) load.LogicalType {
	switch typ {
	case field.TypeBool:
		return load.LogicalTypeBool
	case field.TypeTime:
		return load.LogicalTypeTime
	case field.TypeJSON:
		return load.LogicalTypeJSON
	case field.TypeUUID:
		return load.LogicalTypeUUID
	case field.TypeBytes:
		return load.LogicalTypeBytes
	case field.TypeEnum:
		return load.LogicalTypeEnum
	case field.TypeString:
		return load.LogicalTypeString
	case field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeInt, field.TypeInt64:
		return load.LogicalTypeInt
	case field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint, field.TypeUint64:
		return load.LogicalTypeUint
	case field.TypeFloat32, field.TypeFloat64:
		return load.LogicalTypeFloat
	case field.TypeOther:
		return load.LogicalTypeOther
	default:
		return ""
	}
}

func builtinFieldType(logical field.Type) *load.FieldType {
	semantic, err := load.FieldTypeOf(logical, builtinTypeExpr(logical))
	if err != nil {
		panic(err)
	}
	semantic.Capabilities.AssignableToLogical = true
	semantic.Capabilities.ConvertibleToLogical = true
	semantic.Capabilities.Comparable = logical != field.TypeBytes && logical != field.TypeJSON
	semantic.Capabilities.Nillable = logical == field.TypeBytes || logical == field.TypeJSON
	return semantic
}

func builtinTypeExpr(logical field.Type) *load.TypeExpression {
	basic := func(kind load.BasicKind) *load.TypeExpression {
		return &load.TypeExpression{Kind: load.TypeKindBasic, Basic: kind}
	}
	switch logical {
	case field.TypeBool:
		return basic(load.BasicKindBool)
	case field.TypeString, field.TypeEnum:
		return basic(load.BasicKindString)
	case field.TypeInt:
		return basic(load.BasicKindInt)
	case field.TypeInt8:
		return basic(load.BasicKindInt8)
	case field.TypeInt16:
		return basic(load.BasicKindInt16)
	case field.TypeInt32:
		return basic(load.BasicKindInt32)
	case field.TypeInt64:
		return basic(load.BasicKindInt64)
	case field.TypeUint:
		return basic(load.BasicKindUint)
	case field.TypeUint8:
		return basic(load.BasicKindUint8)
	case field.TypeUint16:
		return basic(load.BasicKindUint16)
	case field.TypeUint32:
		return basic(load.BasicKindUint32)
	case field.TypeUint64:
		return basic(load.BasicKindUint64)
	case field.TypeFloat32:
		return basic(load.BasicKindFloat32)
	case field.TypeFloat64:
		return basic(load.BasicKindFloat64)
	case field.TypeBytes, field.TypeJSON:
		return &load.TypeExpression{Kind: load.TypeKindSlice, Element: basic(load.BasicKindUint8)}
	case field.TypeTime:
		return &load.TypeExpression{Kind: load.TypeKindNamed, Named: &load.TypeName{Package: load.Package{Path: "time", Name: "time"}, Name: "Time"}}
	case field.TypeUUID:
		return &load.TypeExpression{Kind: load.TypeKindArray, Length: 16, Element: basic(load.BasicKindUint8)}
	case field.TypeOther:
		return &load.TypeExpression{Kind: load.TypeKindInterface}
	default:
		panic(fmt.Sprintf("unsupported built-in ID type %s", logical))
	}
}

// IsView indicates if the type (schema) is a view.
func (t Type) IsView() bool {
	return t.schema != nil && t.schema.View
}

// IsEdgeSchema indicates if the type (schema) is used as an edge-schema.
// i.e. is being used by an edge (or its inverse) with edge.Through modifier.
func (t Type) IsEdgeSchema() bool {
	return t.EdgeSchema.To != nil || t.EdgeSchema.From != nil
}

// HasCompositeID indicates if the type has a composite ID field.
func (t Type) HasCompositeID() bool {
	return t.IsEdgeSchema() && len(t.EdgeSchema.ID) > 1
}

// HasOneFieldID indicates if the type has an ID with one field (not composite).
func (t Type) HasOneFieldID() bool {
	return !t.HasCompositeID() && t.ID != nil
}

// Label returns the snake_case label name of the node/type.
func (t Type) Label() string {
	return snake(t.Name)
}

// Table returns SQL table name of the node/type.
func (t Type) Table() string {
	if ant := t.EntSQL(); ant != nil && ant.Table != "" {
		return ant.Table
	}
	if t.schema != nil && t.schema.Config.Table != "" {
		return t.schema.Config.Table
	}
	return snake(rules.Pluralize(t.Name))
}

// EntSQL returns the EntSQL annotation if exists.
func (t Type) EntSQL() *entsql.Annotation {
	return sqlAnnotate(t.Annotations)
}

// Package returns the package name of this node.
func (t Type) Package() string {
	if name := t.PackageAlias(); name != "" {
		return name
	}
	return t.PackageDir()
}

// PackageDir returns the name of the package directory.
func (t Type) PackageDir() string { return strings.ToLower(t.Name) }

// PackageAlias returns local package name of a type if there is one.
// A package has an alias if its generated name conflicts with
// one of the imports of the user-defined or ent builtin types.
func (t Type) PackageAlias() string { return t.alias }

// Receiver returns the receiver name of this node. It makes sure the
// receiver names doesn't conflict with import names.
func (t Type) Receiver() string {
	return "_m"
}

// Pos returns the filename:line position information of this type in the schema.
func (t Type) Pos() string {
	return t.schema.Pos
}

// hasEdge returns true if this type as an edge (reverse or assoc)
// with the given name.
func (t Type) hasEdge(name string) bool {
	for _, e := range t.Edges {
		if name == e.Name {
			return true
		}
	}
	return false
}

// HasAssoc returns true if this type has an assoc-edge (edge.To)
// with the given name. faster than map access for most cases.
func (t Type) HasAssoc(name string) (*Edge, bool) {
	for _, e := range t.Edges {
		if name == e.Name && !e.IsInverse() {
			return e, true
		}
	}
	return nil, false
}

// HasValidators reports if any of the type's field has validators.
func (t Type) HasValidators() bool {
	fields := t.Fields
	if t.HasOneFieldID() && t.ID.UserDefined {
		fields = append(fields, t.ID)
	}
	for _, f := range fields {
		if f.Validators > 0 {
			return true
		}
	}
	return false
}

// HasDefault reports if any of this type's fields has default value on creation.
func (t Type) HasDefault() bool {
	fields := t.Fields
	if t.HasOneFieldID() && t.ID.UserDefined {
		fields = append(fields, t.ID)
	}
	for _, f := range fields {
		if f.Default {
			return true
		}
	}
	return false
}

// HasUpdateDefault reports if any of this type's fields has default value on update.
func (t Type) HasUpdateDefault() bool {
	for _, f := range t.Fields {
		if f.UpdateDefault {
			return true
		}
	}
	return false
}

// HasOptional reports if this type has an optional field.
func (t Type) HasOptional() bool {
	for _, f := range t.Fields {
		if f.Optional {
			return true
		}
	}
	return false
}

// HasNumeric reports if this type has a numeric field.
func (t Type) HasNumeric() bool {
	for _, f := range t.Fields {
		if f.Type.Numeric() {
			return true
		}
	}
	return false
}

// HasUpdateCheckers reports if this type has any checkers to run on update(one).
func (t Type) HasUpdateCheckers() bool {
	for _, f := range t.Fields {
		if (f.Validators > 0 || f.IsEnum() || f.TypeValidator()) && !f.Immutable {
			return true
		}
	}
	for _, e := range t.Edges {
		if e.Unique && !e.Optional {
			return true
		}
	}
	return false
}

// FKEdges returns all edges that reside on the type table as foreign-keys.
func (t Type) FKEdges() (edges []*Edge) {
	for _, e := range t.Edges {
		if e.OwnFK() {
			edges = append(edges, e)
		}
	}
	return
}

// EdgesWithID returns all edges that point to entities with non-composite identifiers.
// These types of edges can be created, updated and deleted by their identifiers.
func (t Type) EdgesWithID() (edges []*Edge) {
	for _, e := range t.Edges {
		if !e.Type.HasCompositeID() {
			edges = append(edges, e)
		}
	}
	return
}

// RuntimeMixin returns schema mixin that needs to be loaded at
// runtime. For example, for default values, validators or hooks.
func (t Type) RuntimeMixin() bool {
	return len(t.MixedInFields()) > 0 || len(t.MixedInHooks()) > 0 || len(t.MixedInPolicies()) > 0 || len(t.MixedInInterceptors()) > 0
}

// MixedInFields returns the indices of mixin holds runtime code.
func (t Type) MixedInFields() []int {
	idx := make(map[int]struct{})
	fields := t.Fields
	if t.HasOneFieldID() && t.ID.UserDefined {
		fields = append(fields, t.ID)
	}
	for _, f := range fields {
		if f.Position != nil && f.Position.MixedIn && (f.Default || f.UpdateDefault || f.Validators > 0) {
			idx[f.Position.MixinIndex] = struct{}{}
		}
	}
	return sortedKeys(idx)
}

// MixedInHooks returns the indices of mixin with hooks.
func (t Type) MixedInHooks() []int {
	if t.schema == nil {
		return nil
	}
	idx := make(map[int]struct{})
	for _, h := range t.schema.Hooks {
		if h.MixedIn {
			idx[h.MixinIndex] = struct{}{}
		}
	}
	return sortedKeys(idx)
}

// MixedInInterceptors returns the indices of mixin with interceptors.
func (t Type) MixedInInterceptors() []int {
	if t.schema == nil {
		return nil
	}
	idx := make(map[int]struct{})
	for _, h := range t.schema.Interceptors {
		if h.MixedIn {
			idx[h.MixinIndex] = struct{}{}
		}
	}
	return sortedKeys(idx)
}

// MixedInPolicies returns the indices of mixin with policies.
func (t Type) MixedInPolicies() []int {
	if t.schema == nil {
		return nil
	}
	idx := make(map[int]struct{})
	for _, h := range t.schema.Policy {
		if h.MixedIn {
			idx[h.MixinIndex] = struct{}{}
		}
	}
	return sortedKeys(idx)
}

// NumMixin returns the type's mixin count.
func (t Type) NumMixin() int {
	m := make(map[int]struct{})
	for _, f := range t.Fields {
		if p := f.Position; p != nil && p.MixedIn {
			m[p.MixinIndex] = struct{}{}
		}
	}
	return len(m)
}

// NumConstraint returns the type's constraint count. Used for slice allocation.
func (t Type) NumConstraint() int {
	var n int
	for _, f := range t.Fields {
		if f.Unique {
			n++
		}
	}
	for _, e := range t.Edges {
		if e.HasConstraint() {
			n++
		}
	}
	return n
}

// MutableFields returns all type fields that are mutable (on update).
func (t Type) MutableFields() []*Field {
	fields := make([]*Field, 0, len(t.Fields))
	for _, f := range t.Fields {
		if f.Immutable {
			continue
		}
		if e, err := f.Edge(); err == nil && e.Immutable {
			continue
		}
		fields = append(fields, f)
	}
	return fields
}

// ImmutableFields returns all type fields that are immutable (for update).
func (t Type) ImmutableFields() []*Field {
	fields := make([]*Field, 0, len(t.Fields))
	for _, f := range t.Fields {
		if f.Immutable {
			fields = append(fields, f)
		}
	}
	return fields
}

// MutationFields returns all the fields that are available on the typed-mutation.
func (t Type) MutationFields() []*Field {
	fields := make([]*Field, 0, len(t.Fields))
	for _, f := range t.Fields {
		if !f.IsEdgeField() {
			fields = append(fields, f)
		}
	}
	return fields
}

// EnumFields returns the enum fields of the schema, if any.
func (t Type) EnumFields() []*Field {
	var fields []*Field
	for _, f := range t.Fields {
		if f.IsEnum() {
			fields = append(fields, f)
		}
	}
	return fields
}

// FieldBy returns the first field that the given function returns true on it.
func (t Type) FieldBy(fn func(*Field) bool) (*Field, bool) {
	if fn(t.ID) {
		return t.ID, true
	}
	for _, f := range t.Fields {
		if fn(f) {
			return f, true
		}
	}
	return nil, false
}

// NumM2M returns the type's many-to-many edge count
func (t Type) NumM2M() int {
	var n int
	for _, e := range t.Edges {
		if e.M2M() {
			n++
		}
	}
	return n
}

// TagTypes returns all struct-tag types of the type fields.
func (t Type) TagTypes() []string {
	tags := make(map[string]bool)
	for _, f := range t.Fields {
		tag := reflect.StructTag(f.StructTag)
		fields := strings.FieldsFunc(f.StructTag, func(r rune) bool {
			return r == ':' || unicode.IsSpace(r)
		})
		for _, name := range fields {
			_, ok := tag.Lookup(name)
			if ok && !tags[name] {
				tags[name] = true
			}
		}
	}
	r := make([]string, 0, len(tags))
	for tag := range tags {
		r = append(r, tag)
	}
	sort.Strings(r)
	return r
}

// AddIndex adds a new index for the type.
// It fails if the schema index is invalid.
func (t *Type) AddIndex(idx *load.Index) error {
	index := &Index{Name: idx.StorageKey, Unique: idx.Unique, Annotations: idx.Annotations}
	if len(idx.Fields) == 0 && len(idx.Edges) == 0 {
		return errors.New("missing fields or edges")
	}
	switch ant := sqlIndexAnnotate(idx.Annotations); {
	case ant == nil:
	case len(ant.PrefixColumns) != 0 && ant.Prefix != 0:
		return fmt.Errorf("index %q cannot contain both entsql.Prefix and entsql.PrefixColumn in annotation", index.Name)
	case ant.Prefix != 0 && len(idx.Fields)+len(idx.Edges) != 1:
		return fmt.Errorf("entsql.Prefix is used in a multicolumn index %q. Use entsql.PrefixColumn instead", index.Name)
	case len(ant.PrefixColumns) > len(idx.Fields)+len(idx.Fields):
		return fmt.Errorf("index %q has more entsql.PrefixColumn than column in its definitions", index.Name)
	}
	for _, name := range idx.Fields {
		var f *Field
		if t.HasOneFieldID() && name == t.ID.Name {
			f = t.ID
		} else if f = t.fields[name]; f == nil {
			return fmt.Errorf("unknown index field %q", name)
		}
		index.Columns = append(index.Columns, f.StorageKey())
	}
	for _, name := range idx.Edges {
		var ed *Edge
		for _, e := range t.Edges {
			if e.Name == name {
				ed = e
				break
			}
		}
		switch {
		case ed == nil:
			return fmt.Errorf("unknown index edge %q", name)
		case ed.Rel.Type == O2O && !ed.IsInverse():
			return fmt.Errorf("non-inverse edge (edge.From) for index %q on O2O relation", name)
		case ed.Rel.Type != M2O && ed.Rel.Type != O2O:
			return fmt.Errorf("relation %s for inverse edge %q is not one of (O2O, M2O)", ed.Rel.Type, name)
		default:
			index.Columns = append(index.Columns, ed.Rel.Column())
		}
	}
	// If no storage-key was defined for this index, generate one.
	if idx.StorageKey == "" {
		// Add the type name as a prefix to the index parts, because
		// multiple types can share the same index attributes.
		parts := append([]string{strings.ToLower(t.Name)}, index.Columns...)
		index.Name = strings.Join(parts, "_")
	}
	t.Indexes = append(t.Indexes, index)
	return nil
}

// setupFKs makes sure all edge-fks are created for the edges.
func (t *Type) setupFKs() error {
	for _, e := range t.Edges {
		if err := e.setStorageKey(); err != nil {
			return fmt.Errorf("%q edge: %w", e.Name, err)
		}
		if ef := e.def.Field; ef != "" && !e.OwnFK() {
			return fmt.Errorf("edge %q has a field %q but it is not holding a foreign key", e.Name, ef)
		}
		if e.IsInverse() || e.M2M() {
			continue
		}
		owner, refid := t, e.Type.ID
		if !e.OwnFK() {
			owner, refid = e.Type, t.ID
		}
		if refid == nil {
			return fmt.Errorf("edge %q references a type without a single-field id", e.Name)
		}
		if err := validateFieldSemantic(refid.Name, refid.Type, refid.Semantic); err != nil {
			return fmt.Errorf("edge %q foreign key: %w", e.Name, err)
		}
		fk := &ForeignKey{
			Edge: e,
			Field: &Field{
				typ:         owner,
				Name:        builderField(e.Rel.Column()),
				Type:        refid.Type,
				Semantic:    refid.Semantic,
				Nillable:    true,
				Optional:    true,
				Unique:      e.Unique,
				UserDefined: refid.UserDefined,
			},
		}
		// Update the foreign-key/edge-field info of the assoc-edge.
		e.Rel.fk = fk
		if edgeField := e.def.Field; edgeField != "" {
			if err := owner.setupFieldEdge(fk, e, edgeField); err != nil {
				return err
			}
		}
		// Update inverse-edge info as well (optional).
		if ref := e.Ref; ref != nil {
			ref.Rel.fk = fk
			if edgeField := e.Ref.def.Field; edgeField != "" {
				if err := owner.setupFieldEdge(fk, e.Ref, edgeField); err != nil {
					return err
				}
			}
		}
		// Special case for checking if the FK is already defined as the ID field (Issue 1288).
		if key, _ := e.StorageKey(); key != nil && len(key.Columns) == 1 && key.Columns[0] == refid.StorageKey() {
			fk.Field = refid
			fk.UserDefined = true
		}
		owner.addFK(fk)
		// In case the user wants to set the column name using the StorageKey option, make sure they
		// do it using the edge-field option if both back-ref edge and field are defined (Issue 1288).
		if e.def.StorageKey != nil && len(e.def.StorageKey.Columns) > 0 && !e.OwnFK() && e.Ref != nil && e.Type.fields[e.Rel.Column()] != nil {
			return fmt.Errorf(
				"column %q definition on edge %[2]q should be replaced with Field(%[1]q) on its reference %[3]q",
				e.Rel.Column(), e.Name, e.Ref.Name,
			)
		}
	}
	return nil
}

// setupFieldEdge check the field-edge validity and configures it and its foreign-key.
func (t *Type) setupFieldEdge(fk *ForeignKey, fkOwner *Edge, fkName string) error {
	tf, ok := t.fields[fkName]
	if !ok {
		return fmt.Errorf("field %q was not found in %s.Fields() for edge %q", fkName, t.Name, fkOwner.Name)
	}
	switch tf, ok := t.fields[fkName]; {
	case !ok:
		return fmt.Errorf("field %q was not found in %s.Fields() for edge %q", fkName, t.Name, fkOwner.Name)
	case tf.Optional && !fkOwner.Optional:
		return fmt.Errorf("edge-field %q was set as Optional, but edge %q is not", fkName, fkOwner.Name)
	case !tf.Optional && fkOwner.Optional:
		return fmt.Errorf("edge %q was set as Optional, but edge-field %q is not", fkOwner.Name, fkName)
	case tf.Immutable && !fkOwner.Immutable:
		return fmt.Errorf("edge-field %q was set as Immutable, but edge %q is not", fkName, fkOwner.Name)
	case !tf.Immutable && fkOwner.Immutable:
		return fmt.Errorf("edge %q was set as Immutable, but edge-field %q is not", fkOwner.Name, fkName)
	case tf.HasValueScanner():
		return fmt.Errorf("edge-field %q cannot have an external ValueScanner", fkName)
	}
	if t1, t2 := tf.Type, fkOwner.Type.ID.Type; t1 != t2 {
		return fmt.Errorf("mismatch field type between edge field %q and id of type %q (%s != %s)", fkName, fkOwner.Type.Name, t1, t2)
	}
	fk.UserDefined = true
	tf.fk, fk.Field = fk, tf
	ekey, err := fkOwner.StorageKey()
	if err != nil {
		return err
	}
	if ekey != nil && len(ekey.Columns) == 1 {
		if fkey := tf.def.StorageKey; fkey != "" && fkey != ekey.Columns[0] {
			return fmt.Errorf("mismatch storage-key for edge %q and field %q", fkOwner.Name, fkName)
		}
		// Update the field storage key.
		tf.def.StorageKey = ekey.Columns[0]
	}
	fkOwner.Rel.Columns = []string{tf.StorageKey()}
	if ref := fkOwner.Ref; ref != nil {
		ref.Rel.Columns = []string{tf.StorageKey()}
	}
	return nil
}

// addFK adds a foreign-key for the type if it doesn't exist.
func (t *Type) addFK(fk *ForeignKey) {
	if _, ok := t.foreignKeys[fk.Field.Name]; ok {
		return
	}
	t.foreignKeys[fk.Field.Name] = struct{}{}
	t.ForeignKeys = append(t.ForeignKeys, fk)
}

// ClientName returns the struct name denoting the client of this type.
func (t Type) ClientName() string {
	return pascal(t.Name) + "Client"
}

// QueryName returns the struct name denoting the query-builder for this type.
func (t Type) QueryName() string {
	return pascal(t.Name) + "Query"
}

// QueryReceiver returns the receiver name of the query-builder for this type.
func (t Type) QueryReceiver() string {
	return "_q"
}

// FilterName returns the struct name denoting the filter-builder for this type.
func (t Type) FilterName() string {
	return pascal(t.Name) + "Filter"
}

// CreateName returns the struct name denoting the create-builder for this type.
func (t Type) CreateName() string {
	return pascal(t.Name) + "Create"
}

// CreateReceiver returns the receiver name of the create-builder for this type.
func (t Type) CreateReceiver() string {
	return "_c"
}

// CreateBulkName returns the struct name denoting the create-bulk-builder for this type.
func (t Type) CreateBulkName() string {
	return pascal(t.Name) + "CreateBulk"
}

// CreateBulReceiver returns the receiver name of the create-bulk-builder for this type.
func (t Type) CreateBulReceiver() string {
	return "_c"
}

// UpdateName returns the struct name denoting the update-builder for this type.
func (t Type) UpdateName() string {
	return pascal(t.Name) + "Update"
}

// UpdateReceiver returns the receiver name of the update-builder for this type.
func (t Type) UpdateReceiver() string {
	return "_u"
}

// UpdateOneName returns the struct name denoting the update-one-builder for this type.
func (t Type) UpdateOneName() string {
	return pascal(t.Name) + "UpdateOne"
}

// UpdateOneReceiver returns the receiver name of the update-one-builder for this type.
func (t Type) UpdateOneReceiver() string {
	return "_u"
}

// DeleteName returns the struct name denoting the delete-builder for this type.
func (t Type) DeleteName() string {
	return pascal(t.Name) + "Delete"
}

// DeleteReceiver returns the receiver name of the delete-builder for this type.
func (t Type) DeleteReceiver() string {
	return "_d"
}

// DeleteOneName returns the struct name denoting the delete-one-builder for this type.
func (t Type) DeleteOneName() string {
	return pascal(t.Name) + "DeleteOne"
}

// DeleteOneReceiver returns the receiver name of the delete-one-builder for this type.
func (t Type) DeleteOneReceiver() string {
	return "_d"
}

// MutationName returns the struct name of the mutation builder for this type.
func (t Type) MutationName() string {
	return pascal(t.Name) + "Mutation"
}

// GroupReceiver returns the receiver name of the group-by builder for this type.
func (t Type) GroupReceiver() string {
	return "_g"
}

// SelectReceiver returns the receiver name of the selector builder for this type.
func (t Type) SelectReceiver() string {
	return "_s"
}

// TypeName returns the constant name of the type defined in mutation.go.
func (t Type) TypeName() string {
	return "Type" + pascal(t.Name)
}

// ValueName returns the name of the value method for this type.
func (t Type) ValueName() string {
	if t.fields["Value"] == nil && t.fields["value"] == nil {
		return "Value"
	}
	return "GetValue"
}

// SiblingImports returns all sibling packages that are needed for the different builders.
func (t Type) SiblingImports() []struct{ Alias, Path string } {
	var (
		imports = []struct{ Alias, Path string }{{Alias: t.PackageAlias(), Path: path.Join(t.Config.Package, t.PackageDir())}}
		seen    = map[string]bool{imports[0].Path: true}
	)
	for _, e := range t.Edges {
		p := path.Join(t.Config.Package, e.Type.PackageDir())
		if !seen[p] {
			seen[p] = true
			imports = append(imports, struct{ Alias, Path string }{Alias: e.Type.PackageAlias(), Path: p})
		}
	}
	return imports
}

// NumHooks returns the number of hooks declared in the type schema.
func (t Type) NumHooks() int {
	if t.schema != nil {
		return len(t.schema.Hooks)
	}
	return 0
}

// HookPositions returns the position information of hooks declared in the type schema.
func (t Type) HookPositions() []*load.Position {
	if t.schema != nil {
		return t.schema.Hooks
	}
	return nil
}

// NumInterceptors returns the number of interceptors declared in the type schema.
func (t Type) NumInterceptors() int {
	if t.schema != nil {
		return len(t.schema.Interceptors)
	}
	return 0
}

// InterceptorPositions returns the position information of interceptors declared in the type schema.
func (t Type) InterceptorPositions() []*load.Position {
	if t.schema != nil {
		return t.schema.Interceptors
	}
	return nil
}

// NumPolicy returns the number of privacy-policy declared in the type schema.
func (t Type) NumPolicy() int {
	if t.schema != nil {
		return len(t.schema.Policy)
	}
	return 0
}

// PolicyPositions returns the position information of privacy policy declared in the type schema.
func (t Type) PolicyPositions() []*load.Position {
	if t.schema != nil {
		return t.schema.Policy
	}
	return nil
}

// RelatedTypes returns all the types (nodes) that
// are related (with edges) to this type.
func (t Type) RelatedTypes() []*Type {
	seen := make(map[string]struct{})
	related := make([]*Type, 0, len(t.Edges))
	for _, e := range t.Edges {
		if _, ok := seen[e.Type.Name]; !ok {
			related = append(related, e.Type)
			seen[e.Type.Name] = struct{}{}
		}
	}
	return related
}

// ValidSchemaName will determine if a name is going to conflict with any
// pre-defined names
func ValidSchemaName(name string) error {
	// Schema package is lower-cased (see Type.Package).
	pkg := strings.ToLower(name)
	if token.Lookup(pkg).IsKeyword() {
		return fmt.Errorf("schema lowercase name conflicts with Go keyword %q", pkg)
	}
	if types.Universe.Lookup(pkg) != nil {
		return fmt.Errorf("schema lowercase name conflicts with Go predeclared identifier %q", pkg)
	}
	if _, ok := globalIdent[pkg]; ok {
		return fmt.Errorf("schema lowercase name conflicts ent predeclared identifier %q", pkg)
	}
	if _, ok := globalIdent[name]; ok {
		return fmt.Errorf("schema name conflicts with ent predeclared identifier %q", name)
	}
	return nil
}

// checkField checks the schema field.
func (t *Type) checkField(tf *Field, f *load.Field) (err error) {
	switch ant := tf.EntSQL(); {
	case f.Name == "":
		err = fmt.Errorf("field name cannot be empty")
	case f.Semantic == nil || !f.Type.Valid():
		err = fmt.Errorf("invalid type for field %s", f.Name)
	case f.Unique && f.Default && f.DefaultKind != reflect.Func:
		err = fmt.Errorf("unique field %q cannot have default value", f.Name)
	case t.fields[f.Name] != nil:
		err = fmt.Errorf("field %q redeclared for type %q", f.Name, t.Name)
	case f.Sensitive && f.Tag != "":
		err = fmt.Errorf("sensitive field %q cannot have struct tags", f.Name)
	case f.Type == field.TypeEnum:
		tf.Enums, err = tf.enums(f)
	case tf.HasLogicalValidators() && tf.Semantic.Capabilities.LogicalProjection == "" && !tf.ConvertedToBasic():
		err = fmt.Errorf("semantic field %q representation cannot encode its logical base for family validators", f.Name)
	case ant != nil && ant.Default != "" && (ant.DefaultExpr != "" || ant.DefaultExprs != nil):
		err = fmt.Errorf("field %q cannot have both default value and default expression annotations", f.Name)
	case tf.HasValueScanner() && tf.IsJSON():
		err = fmt.Errorf("json field %q cannot have an external ValueScanner", f.Name)
	}
	return err
}

// UnexportedForeignKeys returns all foreign-keys that belong to the type
// but are not exported (not defined with field). i.e. generated by ent.
func (t Type) UnexportedForeignKeys() []*ForeignKey {
	fks := make([]*ForeignKey, 0, len(t.ForeignKeys))
	for _, fk := range t.ForeignKeys {
		if !fk.UserDefined {
			fks = append(fks, fk)
		}
	}
	return fks
}

// aliases adds package aliases (local names) for all type-packages that
// their import identifier conflicts with user-defined packages (i.e. GoType).
func aliases(g *Graph) {
	for _, node := range g.Nodes {
		if err := node.renderSemanticTypes(); err != nil {
			panic(graphError{fmt.Sprintf("render semantic types for %q: %s", node.Name, err)})
		}
	}
	mayAlias := make(map[string]*Type)
	for _, n := range g.Nodes {
		if pkg := n.PackageDir(); importPkg[pkg] != "" {
			// By default, a package named "pet" will be named as "entpet".
			n.alias = path.Base(g.Package) + pkg
		} else {
			mayAlias[n.PackageDir()] = n
		}
	}
}

func (t *Type) renderSemanticTypes() error {
	current := load.Package{Path: path.Join(t.Config.Package, t.PackageDir()), Name: t.PackageDir()}
	renderer, err := t.semanticRenderer(current)
	if err != nil {
		return err
	}
	t.semanticImports = renderer.Imports()
	for _, field := range t.semanticFields() {
		if field.DefinesEnumType() {
			field.semanticType = t.PackageDir() + "." + field.StructField()
		} else if field.semanticType, err = renderer.Render(field.Semantic.Representation); err != nil {
			return fmt.Errorf("field %q representation: %w", field.Name, err)
		}
		if field.baseType, err = renderer.Render(field.Semantic.Base); err != nil {
			return fmt.Errorf("field %q logical base: %w", field.Name, err)
		}
	}
	return nil
}

func (t Type) semanticFields() []*Field {
	fields := append([]*Field(nil), t.Fields...)
	if t.HasOneFieldID() {
		fields = append(fields, t.ID)
	}
	for _, foreignKey := range t.UnexportedForeignKeys() {
		fields = append(fields, foreignKey.Field)
	}
	return fields
}

func (t Type) semanticRenderer(current load.Package) (*TypeRenderer, error) {
	renderer := NewTypeRenderer(current, current.Name)
	for _, field := range t.semanticFields() {
		if err := renderer.Add(field.Semantic.Representation, field.Semantic.Base); err != nil {
			return nil, fmt.Errorf("field %q: %w", field.Name, err)
		}
	}
	if current.Path == path.Join(t.Config.Package, t.PackageDir()) {
		for _, edge := range t.EdgesWithID() {
			if err := renderer.Add(edge.Type.ID.Semantic.Representation); err != nil {
				return nil, fmt.Errorf("edge %q id: %w", edge.Name, err)
			}
		}
	}
	return renderer, nil
}

// SemanticImports returns deterministic semantic imports for this type package.
func (t Type) SemanticImports() []Import {
	return t.semanticImports
}

// ClientDependencies holds jointly rendered dependency types and imports for
// the generated client.
type ClientDependencies struct {
	Dependencies Dependencies
	Imports      []Import
}

// ClientDependencies renders all dependency types using one import namespace.
func (g Graph) ClientDependencies() (*ClientDependencies, error) {
	data := &ClientDependencies{}
	if g.Annotations != nil {
		data.Dependencies, _ = g.Annotations[(Dependencies{}).Name()].(Dependencies)
	}
	if len(data.Dependencies) == 0 {
		return data, nil
	}
	generatedNames := make([]string, 0, len(g.Nodes)+2)
	generatedNames = append(generatedNames, path.Base(g.Package), "migrate")
	for _, node := range g.Nodes {
		generatedNames = append(generatedNames, node.Package())
	}
	renderer := NewTypeRenderer(load.Package{Path: g.Package, Name: path.Base(g.Package)}, generatedNames...)
	for _, dependency := range data.Dependencies {
		if err := renderer.Add(dependency.Type); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", dependency.Field, err)
		}
	}
	for _, dependency := range data.Dependencies {
		var err error
		if dependency.RenderedType, err = renderer.Render(dependency.Type); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", dependency.Field, err)
		}
	}
	data.Imports = renderer.Imports()
	return data, nil
}

// RuntimeSemanticImports returns deterministic semantic imports for runtime stitching.
func (g Graph) RuntimeSemanticImports(currentPackage string) []Import {
	generatedPackages := make([]string, 0, len(g.Nodes)+1)
	generatedPackages = append(generatedPackages, currentPackage)
	for _, node := range g.Nodes {
		generatedPackages = append(generatedPackages, node.PackageDir())
	}
	renderer := NewTypeRenderer(load.Package{Path: path.Join(g.Package, currentPackage), Name: currentPackage}, generatedPackages...)
	for _, node := range g.Nodes {
		for _, field := range node.semanticFields() {
			if field.Default || field.UpdateDefault || field.Validators > 0 || field.HasValueScanner() {
				if err := renderer.Add(field.Semantic.Representation, field.Semantic.Base); err != nil {
					panic(err)
				}
			}
		}
	}
	return renderer.Imports()
}

// sqlComment returns the SQL database comment for the node (table), if defined and enabled.
func (t Type) sqlComment() string {
	if ant := t.EntSQL(); ant == nil || ant.WithComments == nil || !*ant.WithComments {
		return ""
	}
	ant := &entschema.CommentAnnotation{}
	if t.Annotations == nil || t.Annotations[ant.Name()] == nil {
		return ""
	}
	if b, err := json.Marshal(t.Annotations[ant.Name()]); err == nil {
		_ = json.Unmarshal(b, &ant)
	}
	return ant.Text
}

// Constant returns the constant name of the field.
func (f Field) Constant() string {
	return "Field" + pascal(f.Name)
}

// GoType returns the generated representation type for this field.
func (f Field) GoType() string {
	if f.semanticType == "" {
		panic(fmt.Sprintf("field %q semantic type was not rendered", f.Name))
	}
	return f.semanticType
}

// BaseType returns the logical field-family type used by base validators.
func (f Field) BaseType() string {
	return f.baseType
}

// TypeConst returns the schema field.Type constant suffix.
func (f Field) TypeConst() string { return f.Type.ConstName() }

// TypeNillable reports whether the representation itself accepts nil.
func (f Field) TypeNillable() bool { return f.Semantic.Capabilities.Nillable }

// TypeNumeric reports whether the logical field family is numeric.
func (f Field) TypeNumeric() bool { return f.Type.Numeric() }

// TypeComparable reports whether representation values are comparable in Go.
func (f Field) TypeComparable() bool { return f.Semantic.Capabilities.Comparable }

// StorageComparable reports whether the logical storage family supports SQL equality predicates.
func (f Field) StorageComparable() bool {
	return !f.IsJSON()
}

// StorageOrderable reports whether the logical storage family supports SQL ordering.
func (f Field) StorageOrderable() bool {
	switch f.Type {
	case field.TypeBool, field.TypeTime, field.TypeUUID, field.TypeEnum, field.TypeString:
		return true
	default:
		return f.Type.Numeric()
	}
}

// TypeValuer reports whether the representation implements driver.Valuer.
func (f Field) TypeValuer() bool { return f.Semantic.Capabilities.Valuer }

// TypeValueScanner reports whether the representation implements Scanner and Valuer.
func (f Field) TypeValueScanner() bool {
	return f.Semantic.Capabilities.Scanner && f.Semantic.Capabilities.Valuer
}

// TypeNullableScanner reports whether a pointer representation uses its pointee
// as the direct scanner and therefore needs an explicit SQL validity wrapper.
func (f Field) TypeNullableScanner() bool {
	return f.Semantic.Capabilities.NullableScanner
}

// TypeValidator reports whether the representation has Validate() error.
func (f Field) TypeValidator() bool { return f.Semantic.Capabilities.Validator }

// TypeStringer reports whether the representation implements fmt.Stringer.
func (f Field) TypeStringer() bool { return f.Semantic.Capabilities.Stringer }

// HasLogicalValidators reports whether logical family validators were attached.
func (f Field) HasLogicalValidators() bool {
	return slices.Contains(f.ValidatorKinds, field.ValidatorLogical)
}

// ValidatorUsesLogicalValue reports whether the validator at the given index
// accepts the logical field-family value rather than its Go representation.
func (f Field) ValidatorUsesLogicalValue(index int) bool {
	return f.ValidatorKinds[index] == field.ValidatorLogical
}

// DefaultName returns the variable name of the default value of this field.
func (f Field) DefaultName() string { return "Default" + pascal(f.Name) }

// UpdateDefaultName returns the variable name of the update default value of this field.
func (f Field) UpdateDefaultName() string { return "Update" + f.DefaultName() }

// DefaultValue returns the default value of the field. Invoked by the template.
func (f Field) DefaultValue() any { return f.def.DefaultValue }

// DefaultFunc returns a bool stating if the default value is a func. Invoked by the template.
func (f Field) DefaultFunc() bool { return f.def.DefaultKind == reflect.Func }

// OrderName returns the function/option name for ordering by this field.
func (f Field) OrderName() string {
	name := "By" + pascal(f.Name)
	// Some users store associations count as a separate field.
	// In this case, we suffix the order name with "Field".
	if f.typ == nil || !strings.HasSuffix(name, "Count") {
		return name
	}
	for _, e := range f.typ.Edges {
		if nameE, err := e.OrderCountName(); err == nil && nameE == name {
			return name + "Field"
		}
	}
	return name
}

// BuilderField returns the struct member of the field in the builder.
func (f Field) BuilderField() string {
	if f.IsEdgeField() {
		e, err := f.Edge()
		if err != nil {
			panic(err)
		}
		return e.BuilderField()
	}
	return builderField(f.Name)
}

// StructField returns the struct member of the field in the model.
func (f Field) StructField() string {
	return pascal(f.Name)
}

// EnumNames returns the enum values of a field.
func (f Field) EnumNames() []string {
	names := make([]string, 0, len(f.Enums))
	for _, e := range f.Enums {
		names = append(names, e.Name)
	}
	return names
}

// EnumValues returns the values of the enum field.
func (f Field) EnumValues() []string {
	values := make([]string, 0, len(f.Enums))
	for _, e := range f.Enums {
		values = append(values, e.Value)
	}
	return values
}

// EnumName returns the constant name for the enum.
func (f Field) EnumName(enum string) string {
	if !token.IsExported(enum) || !token.IsIdentifier(enum) {
		enum = pascal(enum)
	}
	return pascal(f.Name) + enum
}

// Validator returns the user validator name.
func (f Field) Validator() string {
	return pascal(f.Name) + "Validator"
}

// EnumValidator returns the enum-membership validator name. Enum fields without
// user validators retain the established generated name.
func (f Field) EnumValidator() string {
	if f.Validators == 0 {
		return f.Validator()
	}
	return pascal(f.Name) + "ValuesValidator"
}

// EntSQL returns the EntSQL annotation if exists.
func (f Field) EntSQL() *entsql.Annotation {
	return sqlAnnotate(f.Annotations)
}

// mutMethods returns the method names of mutation interface.
var mutMethods = func() map[string]bool {
	names := map[string]bool{"Client": true, "Tx": true, "Where": true, "SetOp": true}
	for method := range reflect.TypeFor[ent.Mutation]().Methods() {
		names[method.Name] = true
	}
	return names
}()

// MutationGet returns the method name for getting the field value.
// The default name is just a pascal format. If the method conflicts
// with the mutation methods, prefix the method with "Get".
func (f Field) MutationGet() string {
	name := pascal(f.Name)
	if mutMethods[name] || (name == "SetID" && f.typ.ID.UserDefined) {
		name = "Get" + name
	}
	return name
}

// MutationGetOld returns the method name for getting the old value of a field.
func (f Field) MutationGetOld() string {
	name := "Old" + pascal(f.Name)
	if mutMethods[name] {
		name = "Get" + name
	}
	return name
}

// MutationReset returns the method name for resetting the field value.
// The default name is "Reset<FieldName>". If the method conflicts
// with the mutation methods, suffix the method with "Field".
func (f Field) MutationReset() string {
	name := "Reset" + pascal(f.Name)
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// MutationSet returns the method name for setting the field value.
// The default name is "Set<FieldName>". If the method conflicts
// with the mutation methods, suffix the method with "Field".
func (f Field) MutationSet() string {
	name := "Set" + f.StructField()
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// MutationClear returns the method name for clearing the field value.
func (f Field) MutationClear() string {
	return "Clear" + f.StructField()
}

// MutationCleared returns the method name for indicating if the field
// was cleared in the mutation.
func (f Field) MutationCleared() string {
	return f.StructField() + "Cleared"
}

// MutationAdd returns the method name for adding a value to the field.
// The default name is "Add<FieldName>". If the method conflicts with
// the mutation methods, suffix the method with "Field".
func (f Field) MutationAdd() string {
	name := "Add" + f.StructField()
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// MutationAdded returns the method name for getting the field value
// that was added to the field.
func (f Field) MutationAdded() string {
	name := "Added" + f.StructField()
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// MutationAppend returns the method name for appending a list of values to the field.
// The default name is "Append<FieldName>". If the method conflicts with the mutation methods,
// suffix the method with "Field".
func (f Field) MutationAppend() string {
	name := "Append" + f.StructField()
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// MutationAppended returns the method name for getting the field value
// that was added to the field.
func (f Field) MutationAppended() string {
	name := "Appended" + f.StructField()
	if mutMethods[name] {
		name += "Field"
	}
	return name
}

// RequiredFor returns a list of dialects that this field is required for.
// A field can be required in one database, but optional in the other. e.g.,
// in case a SchemaType was defined as "serial" for PostgreSQL, but "int" for SQLite.
func (f Field) RequiredFor() (dialects []string) {
	seen := make(map[string]struct{})
	switch f.def.SchemaType[dialect.Postgres] {
	case postgres.TypeSerial, postgres.TypeBigSerial, postgres.TypeSmallSerial:
		seen[dialect.Postgres] = struct{}{}
	}
	switch d := f.Column().Default.(type) {
	// Static values (or nil) are set by
	// the builders, unless explicitly set.
	case nil:
	// Database default values for all dialects.
	case schema.Expr:
		return nil
	case map[string]schema.Expr:
		for k := range d {
			seen[k] = struct{}{}
		}
	}
	for _, d := range f.cfg.Storage.Dialects {
		if _, ok := seen[strings.ToLower(strings.TrimPrefix(d, "dialect."))]; !ok {
			dialects = append(dialects, d)
		}
	}
	return dialects
}

// IsBool returns true if the field is a bool field.
func (f Field) IsBool() bool { return f.Type == field.TypeBool }

// IsBytes returns true if the field is a bytes field.
func (f Field) IsBytes() bool { return f.Type == field.TypeBytes }

// IsTime returns true if the field is a timestamp field.
func (f Field) IsTime() bool { return f.Type == field.TypeTime }

// IsJSON returns true if the field is a JSON field.
func (f Field) IsJSON() bool { return f.Type == field.TypeJSON }

// IsOther returns true if the field is an Other field.
func (f Field) IsOther() bool { return f.Type == field.TypeOther }

// IsString returns true if the field is a string field.
func (f Field) IsString() bool { return f.Type == field.TypeString }

// IsUUID returns true if the field is a UUID field.
func (f Field) IsUUID() bool { return f.Type == field.TypeUUID }

// IsInt returns true if the field is an int field.
func (f Field) IsInt() bool { return f.Type == field.TypeInt }

// IsInt64 returns true if the field is an int64 field.
func (f Field) IsInt64() bool { return f.Type == field.TypeInt64 }

// IsEnum returns true if the field is an enum field.
func (f Field) IsEnum() bool { return f.Type == field.TypeEnum }

// IsEdgeField reports if the given field is an edge-field (i.e. a foreign-key)
// that was referenced by one of the edges.
func (f Field) IsEdgeField() bool { return f.fk != nil }

// IsDeprecated returns true if the field is deprecated.
func (f Field) IsDeprecated() bool { return f.def != nil && f.def.Deprecated }

// DeprecationReason returns the deprecation reason of the field.
func (f Field) DeprecationReason() string {
	if f.def != nil {
		return f.def.DeprecatedReason
	}
	return ""
}

// Edge returns the edge this field is point to.
func (f Field) Edge() (*Edge, error) {
	if !f.IsEdgeField() {
		return nil, fmt.Errorf("field %q is not an edge-field (missing foreign-key)", f.Name)
	}
	if e := f.fk.Edge; e.OwnFK() {
		return e, nil
	}
	return f.fk.Edge.Ref, nil
}

// Sensitive returns true if the field is a sensitive field.
func (f Field) Sensitive() bool { return f.def != nil && f.def.Sensitive }

// Comment returns the comment of the field,
func (f Field) Comment() string {
	if f.def != nil {
		return f.def.Comment
	}
	return ""
}

// NillableValue reports if the field holds a Go value (not a pointer), but the field is nillable.
// It's used by the templates to prefix values with pointer operators (e.g. &intValue or *intValue).
func (f Field) NillableValue() bool {
	return f.Nillable && !f.TypeNillable()
}

// ScanType returns the Go type used by rows.Scan.
func (f Field) ScanType() string {
	if f.TypeValueScanner() {
		if f.Nillable && (!f.TypeNillable() || f.TypeNullableScanner()) {
			return "sql.NullScanner"
		}
		return strings.TrimPrefix(f.GoType(), "*")
	}
	switch f.Type {
	case field.TypeJSON:
		return "[]byte"
	case field.TypeBytes:
		if f.TypeNillable() {
			return f.GoType()
		}
		return "[]byte"
	case field.TypeString, field.TypeEnum:
		return "sql.NullString"
	case field.TypeBool:
		return "sql.NullBool"
	case field.TypeTime:
		return "sql.NullTime"
	case field.TypeInt, field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeInt64, field.TypeUint, field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint64:
		return "sql.NullInt64"
	case field.TypeFloat32, field.TypeFloat64:
		return "sql.NullFloat64"
	default:
		return f.GoType()
	}
}

// HasValueScanner reports if any of the fields has (an external) ValueScanner.
func (t Type) HasValueScanner() bool {
	sqlStorage := t.Config == nil || t.Storage == nil || t.Storage.Name == "sql"
	if sqlStorage && t.ID != nil && t.ID.HasValueScanner() {
		return true
	}
	for _, f := range t.Fields {
		if f.HasValueScanner() {
			return true
		}
	}
	return false
}

// DeprecatedFields returns all deprecated fields of the type.
func (t Type) DeprecatedFields() []*Field {
	fs := make([]*Field, 0, len(t.Fields))
	for _, f := range t.Fields {
		if f.IsDeprecated() {
			fs = append(fs, f)
		}
	}
	return fs
}

// HasValueScanner indicates if the field has (an external) ValueScanner.
func (f Field) HasValueScanner() bool {
	return f.def != nil && f.def.ValueScanner
}

// ValueFunc returns a path to the Value field (func) of the external ValueScanner.
func (f Field) ValueFunc() (string, error) {
	if !f.HasValueScanner() {
		return "", fmt.Errorf("%q does not have an external ValueScanner", f.Name)
	}
	return fmt.Sprintf("%s.ValueScanner.%s.Value", f.typ.Package(), f.StructField()), nil
}

// ScanValueFunc returns a path to the ScanValue field (func) of the external ValueScanner.
func (f Field) ScanValueFunc() (string, error) {
	if !f.HasValueScanner() {
		return "", fmt.Errorf("%q does not have an external ValueScanner", f.Name)
	}
	return fmt.Sprintf("%s.ValueScanner.%s.ScanValue", f.typ.Package(), f.StructField()), nil
}

// FromValueFunc returns a path to the FromValue field (func) of the external ValueScanner.
func (f Field) FromValueFunc() (string, error) {
	if !f.HasValueScanner() {
		return "", fmt.Errorf("%q does not have an external ValueScanner", f.Name)
	}
	return fmt.Sprintf("%s.ValueScanner.%s.FromValue", f.typ.Package(), f.StructField()), nil
}

// NewScanType returns an expression for creating a rows.Scan destination.
func (f Field) NewScanType() string {
	if f.TypeValueScanner() {
		expr := fmt.Sprintf("new(%s)", strings.TrimPrefix(f.GoType(), "*"))
		if f.Nillable && (!f.TypeNillable() || f.TypeNullableScanner()) {
			expr = fmt.Sprintf("&sql.NullScanner{S: %s}", expr)
		}
		return expr
	}
	if f.IsOther() || f.IsBytes() && f.TypeNillable() {
		return fmt.Sprintf("new(%s)", strings.TrimPrefix(f.GoType(), "*"))
	}
	return fmt.Sprintf("new(%s)", f.ScanType())
}

// ScanTypeField extracts and converts a scanned nullable value.
func (f Field) ScanTypeField(receiver string) string {
	if f.TypeValueScanner() {
		if f.Nillable && f.TypeNullableScanner() {
			return fmt.Sprintf("%s.S.(*%s)", receiver, strings.TrimPrefix(f.GoType(), "*"))
		}
		if f.Nillable && !f.TypeNillable() {
			return fmt.Sprintf("*%s.S.(*%s)", receiver, f.GoType())
		}
		if !f.TypeNillable() {
			return "*" + receiver
		}
		return receiver
	}
	if (f.IsBytes() || f.IsOther()) && f.TypeNillable() {
		return "*" + receiver
	}
	switch f.Type {
	case field.TypeEnum:
		return fmt.Sprintf("%s(%s.String)", f.GoType(), receiver)
	case field.TypeString:
		return f.goType(receiver + ".String")
	case field.TypeBool:
		return f.goType(receiver + ".Bool")
	case field.TypeInt64:
		return f.goType(receiver + ".Int64")
	case field.TypeFloat64:
		return f.goType(receiver + ".Float64")
	case field.TypeTime:
		return f.goType(fmt.Sprintf("%s.Time", receiver))
	case field.TypeFloat32:
		return f.goType(fmt.Sprintf("%s.Float64", receiver))
	case field.TypeInt, field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeUint, field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint64:
		return f.goType(fmt.Sprintf("%s.Int64", receiver))
	default:
		return receiver
	}
}

// Column returns the table column. It sets it as a primary key (auto_increment)
// in case of ID field, unless stated otherwise.
func (f Field) Column() *schema.Column {
	c := &schema.Column{
		Name:     f.StorageKey(),
		Type:     f.Type,
		Unique:   f.Unique,
		Nullable: f.Optional,
		Size:     f.size(),
		Enums:    f.EnumValues(),
		Comment:  f.sqlComment(),
	}
	switch {
	case f.Default && (f.Type.Numeric() || f.Type == field.TypeBool):
		c.Default = f.DefaultValue()
	case f.Default && (f.IsString() || f.IsEnum()):
		if s, ok := f.DefaultValue().(string); ok {
			c.Default = s
		}
	}
	// Override the default-value defined in the
	// schema if it was provided by an annotation.
	switch ant := f.EntSQL(); {
	case ant == nil:
	case ant.Default != "":
		c.Default = ant.Default
	case ant.DefaultExpr != "":
		c.Default = schema.Expr(ant.DefaultExpr)
	case ant.DefaultExprs != nil:
		x := make(map[string]schema.Expr)
		for k, v := range ant.DefaultExprs {
			x[k] = schema.Expr(v)
		}
		c.Default = x
	}
	// Override the collation defined in the
	// schema if it was provided by an annotation.
	if ant := f.EntSQL(); ant != nil && ant.Collation != "" {
		c.Collation = ant.Collation
	}
	if f.def != nil {
		c.SchemaType = f.def.SchemaType
	}
	return c
}

// incremental returns if the column has an incremental behavior.
// If no value is defined externally, we use a provided def flag.
func (f Field) incremental(def bool) bool {
	if ant := f.EntSQL(); ant != nil && ant.Incremental != nil {
		return *ant.Incremental
	}
	return def
}

// size returns the field size defined in the schema.
func (f Field) size() int64 {
	if ant := f.EntSQL(); ant != nil && ant.Size != 0 {
		return ant.Size
	}
	if f.def != nil && f.def.Size != nil {
		return *f.def.Size
	}
	return 0
}

// PK is like Column, but for table primary key.
func (f Field) PK() *schema.Column {
	c := &schema.Column{
		Name:      f.StorageKey(),
		Type:      f.Type,
		Key:       schema.PrimaryKey,
		Comment:   f.sqlComment(),
		Increment: f.incremental(f.Type.Integer()),
	}
	// If the PK was defined by the user, and it is UUID or string.
	if f.UserDefined && !f.Type.Numeric() {
		c.Increment = false
		c.Type = f.Type
		c.Unique = f.Unique
		if f.def != nil && f.def.Size != nil {
			c.Size = *f.def.Size
		}
	}
	// Override the default-value defined in the
	// schema if it was provided by an annotation.
	ant := f.EntSQL()
	switch {
	case ant == nil:
	case ant.Default != "":
		c.Default = ant.Default
	case ant.DefaultExpr != "":
		c.Default = schema.Expr(ant.DefaultExpr)
	case ant.DefaultExprs != nil:
		x := make(map[string]schema.Expr)
		for k, v := range ant.DefaultExprs {
			x[k] = schema.Expr(v)
		}
		c.Default = x
	}

	// Override collation with annotation value
	if ant != nil && ant.Collation != "" {
		c.Collation = ant.Collation
	}

	if f.def != nil {
		c.SchemaType = f.def.SchemaType
	}
	return c
}

// sqlComment returns the SQL database comment for the field, if defined and enabled.
func (f Field) sqlComment() string {
	fa, ta := f.EntSQL(), f.typ.EntSQL()
	switch c := f.Comment(); {
	// Field annotation gets precedence over type annotation.
	case fa != nil && fa.WithComments != nil:
		if *fa.WithComments {
			return c
		}
	case ta != nil && ta.WithComments != nil:
		if *ta.WithComments {
			return c
		}
	}
	return ""
}

// StorageKey returns the storage name of the field.
// SQL column name.
func (f Field) StorageKey() string {
	if f.def != nil && f.def.StorageKey != "" {
		return f.def.StorageKey
	}
	return snake(f.Name)
}

// DefinesEnumType reports whether this field's enum type is declared by the generator.
func (f Field) DefinesEnumType() bool {
	return f.IsEnum() && f.Semantic.Representation.Kind == load.TypeKindBasic
}

// RepresentationIsBase reports whether the field representation has the canonical
// identity of its logical base type.
func (f Field) RepresentationIsBase() bool {
	baseID, err := f.Semantic.Base.ID()
	if err != nil {
		panic(err)
	}
	return f.Semantic.ID == baseID
}

// ConvertedToBasic indicates if the Go type of the field
// can be converted to basic type (string, int, etc.).
func (f Field) ConvertedToBasic() bool {
	return f.Semantic.Capabilities.AssignableToLogical || f.Semantic.Capabilities.ConvertibleToLogical
}

// LogicalValueSupported reports whether representation values can be encoded for logical storage predicates.
func (f Field) LogicalValueSupported() bool {
	return f.ConvertedToBasic() || f.Semantic.Capabilities.LogicalProjection != "" || f.TypeValuer() || f.HasValueScanner()
}

// MutationAddType returns the operand type for numeric mutation additions.
// Represented numeric fields preserve their Go type. Unsigned fields use the
// corresponding signed logical base so subtraction remains expressible.
func (f Field) MutationAddType() (string, error) {
	if !f.SupportsMutationAdd() {
		return "", fmt.Errorf("field %q does not support MutationAdd", f.Name)
	}
	if f.Semantic.Logical != load.LogicalTypeUint || f.implementsAdder() {
		return f.GoType(), nil
	}
	switch f.Type {
	case field.TypeUint8:
		return field.TypeInt8.String(), nil
	case field.TypeUint16:
		return field.TypeInt16.String(), nil
	case field.TypeUint32:
		return field.TypeInt32.String(), nil
	case field.TypeUint64:
		return field.TypeInt64.String(), nil
	default:
		return field.TypeInt.String(), nil
	}
}

// SupportsMutationAdd reports if the field supports the mutation "Add(T) T" interface.
func (f Field) SupportsMutationAdd() bool {
	if !f.Type.Numeric() || f.IsEdgeField() {
		return false
	}
	return f.ConvertedToBasic() || f.implementsAdder()
}

// MutationAddAssignExpr returns the expression for summing to identifiers and assigning to the mutation field.
//
//	MutationAddAssignExpr(a, b) => *m.a += b		// Basic Go type.
//	MutationAddAssignExpr(a, b) => *m.a = m.Add(b)	// Custom Go types that implement the (Add(T) T) interface.
func (f Field) MutationAddAssignExpr(ident1, ident2 string) (string, error) {
	if !f.SupportsMutationAdd() {
		return "", fmt.Errorf("field %q does not support the add operation (a + b)", f.Name)
	}
	expr := "*%s += %s"
	if f.implementsAdder() {
		expr = "*%[1]s = %[1]s.Add(%[2]s)"
	}
	return fmt.Sprintf(expr, ident1, ident2), nil
}

func (f Field) implementsAdder() bool { return f.Semantic.Capabilities.Adder }

// SupportsMutationAppend reports if the field supports the mutation append operation.
func (f Field) SupportsMutationAppend() bool {
	return f.IsJSON() && f.Semantic.Capabilities.Underlying != nil && f.Semantic.Capabilities.Underlying.Kind == load.TypeKindSlice
}

// BasicType returns an expression that projects a representation to its logical base.
func (f Field) BasicType(identifier string) string {
	if f.Semantic.Capabilities.AssignableToLogical {
		return identifier
	}
	if f.Semantic.Capabilities.ConvertibleToLogical {
		return fmt.Sprintf("%s(%s)", f.BaseType(), identifier)
	}
	if projection := f.Semantic.Capabilities.LogicalProjection; projection != "" {
		if strings.HasPrefix(projection, "[") {
			return identifier + projection
		}
		return fmt.Sprintf("%s.%s", identifier, projection)
	}
	return ""
}

// goType returns the Go expression for the given logical scan value in the
// field representation.
func (f Field) goType(identifier string) string {
	return fmt.Sprintf("%s(%s)", f.GoType(), identifier)
}

func (f Field) enums(lf *load.Field) ([]Enum, error) {
	if len(lf.Enums) == 0 {
		return nil, fmt.Errorf("missing values for enum field %q", f.Name)
	}
	enums := make([]Enum, 0, len(lf.Enums))
	values := make(map[string]bool, len(lf.Enums))
	for i := range lf.Enums {
		switch name, value := f.EnumName(lf.Enums[i].N), lf.Enums[i].V; {
		case value == "":
			return nil, fmt.Errorf("%q field value cannot be empty", f.Name)
		case values[value]:
			return nil, fmt.Errorf("duplicate values %q for enum field %q", value, f.Name)
		case !token.IsIdentifier(name) && f.DefinesEnumType():
			return nil, fmt.Errorf("enum %q does not have a valid Go identifier (%q)", value, name)
		default:
			values[value] = true
			enums = append(enums, Enum{Name: name, Value: value})
		}
	}
	if lf.DefaultValue == nil {
		return enums, nil
	}
	value, ok := lf.DefaultValue.(string)
	if !ok {
		return nil, fmt.Errorf("enum field %q default cannot be represented as a string enum value", f.Name)
	}
	if !values[value] {
		return nil, fmt.Errorf("invalid default value for enum field %q", f.Name)
	}
	return enums, nil
}

// Ops returns all predicate operations of the field.
func (f *Field) Ops() []Op {
	ops := fieldOps(f)
	if f.cfg != nil && f.cfg.Storage.Ops != nil {
		ops = append(ops, f.cfg.Storage.Ops(f)...)
	}
	return ops
}

// Label returns the label name of the edge.
func (e Edge) Label() string {
	if e.IsInverse() {
		return fmt.Sprintf("%s_%s", e.Owner.Label(), snake(e.Inverse))
	}
	return fmt.Sprintf("%s_%s", e.Owner.Label(), snake(e.Name))
}

// Constant returns the constant name of the edge.
func (e Edge) Constant() string {
	return "Edge" + pascal(e.Name)
}

// M2M indicates if this edge is M2M edge.
func (e Edge) M2M() bool { return e.Rel.Type == M2M }

// M2O indicates if this edge is M2O edge.
func (e Edge) M2O() bool { return e.Rel.Type == M2O }

// O2M indicates if this edge is O2M edge.
func (e Edge) O2M() bool { return e.Rel.Type == O2M }

// O2O indicates if this edge is O2O edge.
func (e Edge) O2O() bool { return e.Rel.Type == O2O }

// IsInverse returns if this edge is an inverse edge.
func (e Edge) IsInverse() bool { return e.Inverse != "" }

// LabelConstant returns the constant name of the edge label.
func (e Edge) LabelConstant() string {
	name := e.Name
	if e.IsInverse() {
		name = e.Inverse
	}
	return pascal(name) + "Label"
}

// InverseLabelConstant returns the inverse constant name of the edge.
func (e Edge) InverseLabelConstant() string { return pascal(e.Name) + "InverseLabel" }

// TableConstant returns the constant name of the relation table.
// The value id Edge.Rel.Table, which is table that holds the relation/edge.
func (e Edge) TableConstant() string { return pascal(e.Name) + "Table" }

// InverseTableConstant returns the constant name of the other/inverse type of the relation.
func (e Edge) InverseTableConstant() string { return pascal(e.Name) + "InverseTable" }

// ColumnConstant returns the constant name of the relation column.
func (e Edge) ColumnConstant() string { return pascal(e.Name) + "Column" }

// PKConstant returns the constant name of the primary key. Used for M2M edges.
func (e Edge) PKConstant() string { return pascal(e.Name) + "PrimaryKey" }

// HasConstraint indicates if this edge has a unique constraint check.
// We check uniqueness when both-directions are unique or one of them.
func (e Edge) HasConstraint() bool {
	return e.Rel.Type == O2O || e.Rel.Type == O2M
}

// BuilderField returns the struct member of the edge in the builder.
func (e Edge) BuilderField() string {
	return builderField(e.Name)
}

// EagerLoadField returns the struct field (of query builder)
// for storing the eager-loading info.
func (e Edge) EagerLoadField() string {
	return "with" + e.StructField()
}

// EagerLoadNamedField returns the struct field (of query builder)
// for storing the eager-loading info for named edges.
func (e Edge) EagerLoadNamedField() string {
	return "withNamed" + e.StructField()
}

// StructField returns the struct member of the edge in the model.
func (e Edge) StructField() string {
	return pascal(e.Name)
}

// OwnFK indicates if the foreign-key of this edge is owned by the edge
// column (reside in the type's table). Used by the SQL storage-driver.
func (e Edge) OwnFK() bool {
	switch {
	case e.M2O():
		return true
	case e.O2O() && (e.IsInverse() || e.Bidi):
		return true
	}
	return false
}

// ForeignKey returns the foreign-key of the inverse-field.
func (e *Edge) ForeignKey() (*ForeignKey, error) {
	if e.Rel.fk != nil {
		return e.Rel.fk, nil
	}
	return nil, fmt.Errorf("foreign-key was not found for edge %q of type %s", e.Name, e.Rel.Type)
}

// Field returns the field that was referenced in the schema. For example:
//
//	edge.From("owner", User.Type).
//		Ref("pets").
//		Field("owner_id")
//
// Note that the zero value is returned if no field was defined in the schema.
func (e Edge) Field() *Field {
	if !e.OwnFK() {
		return nil
	}
	if fk, err := e.ForeignKey(); err == nil && fk.Field.IsEdgeField() {
		return fk.Field
	}
	return nil
}

// Comment returns the comment of the edge.
func (e Edge) Comment() string {
	if e.def != nil {
		return e.def.Comment
	}
	return ""
}

// HasFieldSetter reports if this edge already has a field-edge setters for its mutation API.
// It's used by the codegen templates to avoid generating duplicate setters for id APIs (e.g. SetOwnerID).
func (e Edge) HasFieldSetter() bool {
	if !e.OwnFK() {
		return false
	}
	fk, err := e.ForeignKey()
	if err != nil {
		return false
	}
	return fk.UserDefined && fk.Field.MutationSet() == e.MutationSet()
}

// MutationSet returns the method name for setting the edge id.
func (e Edge) MutationSet() string {
	return "Set" + pascal(e.Name) + "ID"
}

// MutationAdd returns the method name for adding edge ids.
func (e Edge) MutationAdd() string {
	return "Add" + pascal(rules.Singularize(e.Name)) + "IDs"
}

// MutationReset returns the method name for resetting the edge value.
// The default name is "Reset<EdgeName>". If the method conflicts
// with the mutation methods, suffix the method with "Edge".
func (e Edge) MutationReset() string {
	name := "Reset" + pascal(e.Name)
	if _, ok := mutMethods[name]; ok {
		name += "Edge"
	}
	return name
}

// MutationClear returns the method name for clearing the edge value.
// The default name is "Clear<EdgeName>". If the method conflicts
// with the mutation methods, suffix the method with "Edge".
func (e Edge) MutationClear() string {
	name := "Clear" + pascal(e.Name)
	if _, ok := mutMethods[name]; ok {
		name += "Edge"
	}
	return name
}

// MutationRemove returns the method name for removing edge ids.
func (e Edge) MutationRemove() string {
	return "Remove" + pascal(rules.Singularize(e.Name)) + "IDs"
}

// MutationCleared returns the method name for indicating if the edge
// was cleared in the mutation. The default name is "<EdgeName>Cleared".
// If the method conflicts with the mutation methods, add "Edge" the
// after the edge name.
func (e Edge) MutationCleared() string {
	name := pascal(e.Name) + "Cleared"
	if _, ok := mutMethods[name]; ok {
		return pascal(e.Name) + "EdgeCleared"
	}
	return name
}

// OrderCountName returns the function/option name for ordering by the edge count.
func (e Edge) OrderCountName() (string, error) {
	if e.Unique {
		return "", fmt.Errorf("edge %q is unique", e.Name)
	}
	return fmt.Sprintf("By%sCount", pascal(e.Name)), nil
}

// OrderTermsName returns the function/option name for ordering by any term.
func (e Edge) OrderTermsName() (string, error) {
	if e.Unique {
		return "", fmt.Errorf("edge %q is unique", e.Name)
	}
	return fmt.Sprintf("By%s", pascal(e.Name)), nil
}

// OrderFieldName returns the function/option name for ordering by edge field.
func (e Edge) OrderFieldName() (string, error) {
	if !e.Unique {
		return "", fmt.Errorf("edge %q is not-unique", e.Name)
	}
	return fmt.Sprintf("By%sField", pascal(e.Name)), nil
}

// setStorageKey sets the storage-key option in the schema or fail.
func (e *Edge) setStorageKey() error {
	key, err := e.StorageKey()
	if err != nil || key == nil {
		return err
	}
	switch rel := e.Rel; {
	case key.Table != "" && rel.Type != M2M:
		return fmt.Errorf("StorageKey.Table is allowed only for M2M edges (got %s)", e.Rel.Type)
	case len(key.Columns) == 1 && rel.Type == M2M:
		return fmt.Errorf("%s edge have 2 columns. Use edge.Columns(to, from) instead", e.Rel.Type)
	case len(key.Columns) > 1 && rel.Type != M2M:
		return fmt.Errorf("%s edge does not have 2 columns. Use edge.Column(%s) instead", e.Rel.Type, key.Columns[0])
	}
	if key.Table != "" {
		e.Rel.Table = key.Table
	}
	if len(key.Columns) > 0 {
		e.Rel.Columns[0] = key.Columns[0]
	}
	if len(key.Columns) > 1 {
		e.Rel.Columns[1] = key.Columns[1]
	}
	return nil
}

// StorageKey returns the storage-key defined on the schema if exists.
func (e Edge) StorageKey() (*edge.StorageKey, error) {
	key := e.def.StorageKey
	if !e.IsInverse() {
		return key, nil
	}
	assoc, ok := e.Owner.HasAssoc(e.Inverse)
	if !ok || assoc.def.StorageKey == nil {
		return key, nil
	}
	// Assoc/To edge found with storage-key configured.
	if key != nil {
		return nil, fmt.Errorf("multiple storage-keys defined for edge %q<->%q", e.Name, assoc.Name)
	}
	return assoc.def.StorageKey, nil
}

// EntSQL returns the EntSQL annotation if exists.
func (e Edge) EntSQL() *entsql.Annotation {
	return sqlAnnotate(e.Annotations)
}

// Index returns the index of the edge in the schema.
// Used mainly to extract its position in the "loadedTypes" array.
func (e Edge) Index() (int, error) {
	// "owner" is the type that holds the edge.
	owner := e.Owner
	if e.IsInverse() {
		owner = e.Ref.Type
	}
	for i, e1 := range owner.Edges {
		if e1.Name == e.Name {
			return i, nil
		}
	}
	return 0, fmt.Errorf("edge %q was not found in its owner schema %q", e.Name, e.Owner.Name)
}

// Column returns the first element from the columns slice.
func (r Relation) Column() string {
	if len(r.Columns) == 0 {
		panic(fmt.Sprintf("missing column for Relation.Table: %s", r.Table))
	}
	return r.Columns[0]
}

// StructField returns the struct member of the foreign-key in the generated model.
func (f ForeignKey) StructField() string {
	if f.UserDefined {
		return f.Field.StructField()
	}
	return f.Field.Name
}

// Rel is a relation type of an edge.
type Rel int

// Relation types.
const (
	Unk Rel = iota // Unknown.
	O2O            // One to one / has one.
	O2M            // One to many / has many.
	M2O            // Many to one (inverse perspective for O2M).
	M2M            // Many to many.
)

// String returns the relation name.
func (r Rel) String() string {
	s := "Unknown"
	switch r {
	case O2O:
		s = "O2O"
	case O2M:
		s = "O2M"
	case M2O:
		s = "M2O"
	case M2M:
		s = "M2M"
	}
	return s
}

func structTag(name, tag string) string {
	t := fmt.Sprintf(`json:"%s,omitempty"`, name)
	if tag == "" {
		return t
	}
	if _, ok := reflect.StructTag(tag).Lookup("json"); !ok {
		tag = t + " " + tag
	}
	return tag
}

// builderField returns the struct field for the given name
// and ensures it doesn't conflict with Go keywords and other
// builder fields, and it is not exported.
func builderField(name string) string {
	_, ok := privateField[name]
	if ok || token.Lookup(name).IsKeyword() || strings.ToUpper(name[:1]) == name[:1] {
		return "_" + name
	}
	return name
}

// fieldAnnotate extracts the field annotation from a loaded annotation format.
func fieldAnnotate(annotation map[string]any) *field.Annotation {
	annotate := &field.Annotation{}
	if annotation == nil || annotation[annotate.Name()] == nil {
		return nil
	}
	if buf, err := json.Marshal(annotation[annotate.Name()]); err == nil {
		_ = json.Unmarshal(buf, &annotate)
	}
	return annotate
}

// sqlAnnotate extracts the entsql.Annotation from a loaded annotation format.
func sqlAnnotate(annotation map[string]any) *entsql.Annotation {
	annotate := &entsql.Annotation{}
	if annotation == nil || annotation[annotate.Name()] == nil {
		return nil
	}
	if buf, err := json.Marshal(annotation[annotate.Name()]); err == nil {
		_ = json.Unmarshal(buf, &annotate)
	}
	return annotate
}

// sqlIndexAnnotate extracts the entsql annotation from a loaded annotation format.
func sqlIndexAnnotate(annotation map[string]any) *entsql.IndexAnnotation {
	annotate := &entsql.IndexAnnotation{}
	if annotation == nil || annotation[annotate.Name()] == nil {
		return nil
	}
	if buf, err := json.Marshal(annotation[annotate.Name()]); err == nil {
		_ = json.Unmarshal(buf, &annotate)
	}
	return annotate
}

var (
	// global identifiers used by the generated package.
	globalIdent = names(
		"AggregateFunc",
		"As",
		"Asc",
		"Client",
		"config",
		"Count",
		"Debug",
		"Desc",
		"Driver",
		"Hook",
		"Interceptor",
		"Log",
		"MutateFunc",
		"Mutation",
		"Mutator",
		"Op",
		"Option",
		"OrderFunc",
		"Max",
		"Mean",
		"Min",
		"Schema",
		"Sum",
		"Policy",
		"Query",
		"Value",
	)
	// private fields used by the different builders.
	privateField = names(
		"config",
		"ctx",
		"done",
		"hooks",
		"inters",
		"limit",
		"mutation",
		"offset",
		"oldValue",
		"order",
		"op",
		"path",
		"predicates",
		"typ",
		"unique",
		"driver",
	)
)

func names(ids ...string) map[string]struct{} {
	m := make(map[string]struct{})
	for i := range ids {
		m[ids[i]] = struct{}{}
	}
	return m
}

func sortedKeys(m map[int]struct{}) []int {
	s := make([]int, 0, len(m))
	for k := range m {
		s = append(s, k)
	}
	sort.Ints(s)
	return s
}
