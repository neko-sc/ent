// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
)

// Schema represents an ent.Schema that was loaded from a complied user package.
type Schema struct {
	Name        string         `json:"name,omitempty"`
	Pos         string         `json:"-"`
	View        bool           `json:"view,omitempty"`
	Config      ent.Config     `json:"config,omitempty"`
	Edges       []*Edge        `json:"edges,omitempty"`
	Fields      []*Field       `json:"fields,omitempty"`
	Indexes     []*Index       `json:"indexes,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// Position describes a position in the schema.
type Position struct {
	Index      int  // Index in the field list.
	MixedIn    bool // Indicates if the schema object was mixed-in.
	MixinIndex int  // Mixin index in the mixin list.
}

// Field represents an ent.Field that was loaded from a complied user package.
type Field struct {
	Name             string                  `json:"name,omitempty"`
	Type             field.Type              `json:"type"`
	Semantic         *FieldType              `json:"semantic_type"`
	ValueScanner     bool                    `json:"value_scanner,omitempty"`
	Tag              string                  `json:"tag,omitempty"`
	Size             *int64                  `json:"size,omitempty"`
	Enums            []struct{ N, V string } `json:"enums,omitempty"`
	Unique           bool                    `json:"unique,omitempty"`
	Nillable         bool                    `json:"nillable,omitempty"`
	Optional         bool                    `json:"optional,omitempty"`
	Default          bool                    `json:"default,omitempty"`
	DefaultValue     any                     `json:"default_value,omitempty"`
	DefaultKind      reflect.Kind            `json:"default_kind,omitempty"`
	UpdateDefault    bool                    `json:"update_default,omitempty"`
	Immutable        bool                    `json:"immutable,omitempty"`
	Validators       int                     `json:"validators,omitempty"`
	ValidatorKinds   []field.ValidatorKind   `json:"validator_kinds,omitempty"`
	StorageKey       string                  `json:"storage_key,omitempty"`
	Position         *Position               `json:"position,omitempty"`
	Sensitive        bool                    `json:"sensitive,omitempty"`
	SchemaType       map[string]string       `json:"schema_type,omitempty"`
	Annotations      map[string]any          `json:"annotations,omitempty"`
	Comment          string                  `json:"comment,omitempty"`
	Deprecated       bool                    `json:"deprecated,omitempty"`
	DeprecatedReason string                  `json:"deprecated_reason,omitempty"`
}

// Edge represents an ent.Edge that was loaded from a complied user package.
type Edge struct {
	Name        string                 `json:"name,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Tag         string                 `json:"tag,omitempty"`
	Field       string                 `json:"field,omitempty"`
	RefName     string                 `json:"ref_name,omitempty"`
	Ref         *Edge                  `json:"ref,omitempty"`
	Through     *struct{ N, T string } `json:"through,omitempty"`
	Unique      bool                   `json:"unique,omitempty"`
	Inverse     bool                   `json:"inverse,omitempty"`
	Required    bool                   `json:"required,omitempty"`
	Immutable   bool                   `json:"immutable,omitempty"`
	StorageKey  *edge.StorageKey       `json:"storage_key,omitempty"`
	Annotations map[string]any         `json:"annotations,omitempty"`
	Comment     string                 `json:"comment,omitempty"`
}

// Index represents an ent.Index that was loaded from a complied user package.
type Index struct {
	Unique      bool           `json:"unique,omitempty"`
	Edges       []string       `json:"edges,omitempty"`
	Fields      []string       `json:"fields,omitempty"`
	StorageKey  string         `json:"storage_key,omitempty"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

// NewEdge creates an loaded edge from edge descriptor.
func NewEdge(ed *edge.Descriptor) *Edge {
	ne := &Edge{
		Tag:         ed.Tag,
		Type:        ed.Type,
		Name:        ed.Name,
		Field:       ed.Field,
		Unique:      ed.Unique,
		Inverse:     ed.Inverse,
		Required:    ed.Required,
		Immutable:   ed.Immutable,
		RefName:     ed.RefName,
		Through:     ed.Through,
		StorageKey:  ed.StorageKey,
		Comment:     ed.Comment,
		Annotations: make(map[string]any),
	}
	for _, at := range ed.Annotations {
		ne.addAnnotation(at)
	}
	if ref := ed.Ref; ref != nil {
		ne.Ref = NewEdge(ref)
		ne.StorageKey = ne.Ref.StorageKey
	}
	return ne
}

// TypeResolver resolves an opaque runtime key to compiler-authored field metadata.
type TypeResolver func(reflect.Type, field.Type) (*FieldType, error)

// NewField creates a loaded field from field descriptor.
func NewField(fd *field.Descriptor, resolver TypeResolver) (*Field, error) {
	if fd.Err != nil {
		return nil, fmt.Errorf("field %q: %w", fd.Name, fd.Err)
	}
	sf := &Field{
		Name:          fd.Name,
		Type:          fd.Type,
		ValueScanner:  fd.ValueScanner != nil,
		Tag:           fd.Tag,
		Enums:         fd.Enums,
		Unique:        fd.Unique,
		Nillable:      fd.Nillable,
		Optional:      fd.Optional,
		Default:       fd.Default != nil,
		UpdateDefault: fd.UpdateDefault != nil,
		Immutable:     fd.Immutable,
		StorageKey:    fd.StorageKey,
		Validators:    len(fd.Validators),
		ValidatorKinds: append(
			[]field.ValidatorKind(nil),
			fd.ValidatorKinds...,
		),
		Sensitive:        fd.Sensitive,
		SchemaType:       fd.SchemaType,
		Annotations:      make(map[string]any),
		Comment:          fd.Comment,
		Deprecated:       fd.Deprecated,
		DeprecatedReason: fd.DeprecatedReason,
	}
	for _, at := range fd.Annotations {
		sf.addAnnotation(at)
	}
	if fd.RuntimeType == nil {
		return nil, fmt.Errorf("field %q: missing semantic runtime type", sf.Name)
	}
	var resolveError error
	if sf.Semantic, resolveError = resolver(fd.RuntimeType, fd.Type); resolveError != nil {
		return nil, fmt.Errorf("field %q: %w", sf.Name, resolveError)
	}
	if sf.Semantic == nil {
		return nil, fmt.Errorf("field %q: resolver returned no semantic type", sf.Name)
	}
	if fd.Type == field.TypeArray {
		if fd.RuntimeType.Kind() != reflect.Slice {
			return nil, fmt.Errorf("field %q: array representation must be a slice", sf.Name)
		}
		element := fd.RuntimeType.Elem()
		logical := LogicalTypeJSON
		switch {
		case element.PkgPath() == "time" && element.Name() == "Time":
			logical = LogicalTypeTime
		case element.Name() == "UUID" && element.Kind() == reflect.Array && element.Len() == 16 && element.Elem().Kind() == reflect.Uint8:
			logical = LogicalTypeUUID
		default:
			switch element.Kind() {
			case reflect.String:
				logical = LogicalTypeString
			case reflect.Bool:
				logical = LogicalTypeBool
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				logical = LogicalTypeInt
			case reflect.Float32, reflect.Float64:
				logical = LogicalTypeFloat
			case reflect.Map, reflect.Struct, reflect.Slice, reflect.Array, reflect.Interface:
			default:
				return nil, fmt.Errorf("field %q: unsupported array element %s", sf.Name, element)
			}
		}
		elementExpression, err := typeExpressionFor(element)
		if err != nil {
			return nil, err
		}
		elementExpression.Logical = logical
		sf.Semantic.Base = &TypeExpression{Kind: TypeKindSlice, Element: elementExpression}
		sf.Semantic.Capabilities.Comparable = false
		sf.Semantic.Capabilities.Adder = false
		sf.Semantic.Capabilities.Nillable = true
	}
	sf.Semantic.Storage.Dialects = sf.SchemaType
	if len(sf.ValidatorKinds) != sf.Validators {
		return nil, fmt.Errorf("field %q: validator metadata count does not match validators", sf.Name)
	}
	if slices.Contains(sf.ValidatorKinds, field.ValidatorLogical) && sf.Semantic.Capabilities.LogicalProjection == "" && !sf.Semantic.Capabilities.ConvertibleToLogical {
		return nil, fmt.Errorf("field %q: representation cannot encode its logical base for family validators", sf.Name)
	}
	if err := validateFieldRepresentation(sf); err != nil {
		return nil, fmt.Errorf("field %q: %w", sf.Name, err)
	}
	if size := int64(fd.Size); size != 0 {
		sf.Size = &size
	}
	if sf.Default {
		sf.DefaultKind = reflect.TypeOf(fd.Default).Kind()
	}
	// If the default value can be encoded to the generator.
	// For example, not a function like time.Now.
	if _, err := json.Marshal(fd.Default); err == nil {
		sf.DefaultValue = fd.Default
	}
	return sf, nil
}

func validateFieldRepresentation(loadedField *Field) error {
	capabilities := loadedField.Semantic.Capabilities
	if loadedField.ValueScanner || loadedField.Type == field.TypeJSON || loadedField.Type == field.TypeArray {
		return nil
	}
	if capabilities.Scanner && capabilities.Valuer {
		return nil
	}
	if capabilities.Scanner != capabilities.Valuer {
		return errors.New("representation must implement both Scanner and Valuer or provide an external Codec")
	}
	if loadedField.Type == field.TypeOther {
		return errors.New("representation requires Scanner and Valuer or an external Codec")
	}
	if (capabilities.AssignableToLogical || capabilities.ConvertibleToLogical || capabilities.LogicalProjection != "") && capabilities.LogicalReverseConvertible {
		return nil
	}
	return fmt.Errorf("representation cannot round-trip logical %s; use a reverse-convertible representation, Scanner and Valuer, or an external Codec", loadedField.Type)
}

// NewIndex creates an loaded index from index descriptor.
func NewIndex(idx *index.Descriptor) *Index {
	ni := &Index{
		Edges:       idx.Edges,
		Fields:      idx.Fields,
		Unique:      idx.Unique,
		StorageKey:  idx.StorageKey,
		Annotations: make(map[string]any),
	}
	for _, at := range idx.Annotations {
		ni.addAnnotation(at)
	}
	return ni
}

// MarshalSchema encodes the ent.Schema interface into a JSON
// that can be decoded into the Schema objects declared above.
func MarshalSchema(schema ent.Interface, resolver TypeResolver) (b []byte, err error) {
	if resolver == nil {
		return nil, errors.New("semantic runtime type has no resolver")
	}
	s := &Schema{
		Config:      schema.Config(),
		Name:        indirect(reflect.TypeOf(schema)).Name(),
		Annotations: make(map[string]any),
	}
	_, s.View = schema.(ent.Viewer)
	if err := s.loadMixin(schema, resolver); err != nil {
		return nil, fmt.Errorf("schema %q: %w", s.Name, err)
	}
	// Schema annotations override mixed-in annotations.
	for _, at := range schema.Annotations() {
		if e, ok := at.(interface{ Err() error }); ok && e.Err() != nil {
			return nil, fmt.Errorf("schema %q: %w", s.Name, e.Err())
		}
		s.addAnnotation(at)
	}
	if err := s.loadFields(schema, resolver); err != nil {
		return nil, fmt.Errorf("schema %q: %w", s.Name, err)
	}
	edges, err := safeEdges(schema)
	if err != nil {
		return nil, fmt.Errorf("schema %q: %w", s.Name, err)
	}
	for _, e := range edges {
		s.Edges = append(s.Edges, NewEdge(e.Descriptor()))
	}
	indexes, err := safeIndexes(schema)
	if err != nil {
		return nil, fmt.Errorf("schema %q: %w", s.Name, err)
	}
	for _, idx := range indexes {
		s.Indexes = append(s.Indexes, NewIndex(idx.Descriptor()))
	}
	return json.Marshal(s)
}

// UnmarshalSchema decodes the given buffer to a loaded schema.
func UnmarshalSchema(buf []byte) (*Schema, error) {
	s := &Schema{}
	if err := json.Unmarshal(buf, s); err != nil {
		return nil, err
	}
	for _, f := range s.Fields {
		if err := f.defaults(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// loadMixin loads mixin to schema from ent.Interface.
func (s *Schema) loadMixin(schema ent.Interface, resolver TypeResolver) error {
	mixin, err := safeMixin(schema)
	if err != nil {
		return err
	}
	for i, mx := range mixin {
		name := indirect(reflect.TypeOf(mx)).Name()
		fields, err := safeFields(mx)
		if err != nil {
			return fmt.Errorf("mixin %q: %w", name, err)
		}
		for j, f := range fields {
			sf, err := NewField(f.Descriptor(), resolver)
			if err != nil {
				return fmt.Errorf("mixin %q: %w", name, err)
			}
			sf.Position = &Position{
				Index:      j,
				MixedIn:    true,
				MixinIndex: i,
			}
			s.Fields = append(s.Fields, sf)
		}
		edges, err := safeEdges(mx)
		if err != nil {
			return fmt.Errorf("mixin %q: %w", name, err)
		}
		for _, e := range edges {
			s.Edges = append(s.Edges, NewEdge(e.Descriptor()))
		}
		indexes, err := safeIndexes(mx)
		if err != nil {
			return fmt.Errorf("mixin %q: %w", name, err)
		}
		for _, idx := range indexes {
			s.Indexes = append(s.Indexes, NewIndex(idx.Descriptor()))
		}
		for _, at := range mx.Annotations() {
			s.addAnnotation(at)
		}
	}
	return nil
}

// loadFields loads field to schema from ent.Interface.
func (s *Schema) loadFields(schema ent.Interface, resolver TypeResolver) error {
	fields, err := safeFields(schema)
	if err != nil {
		return err
	}
	for i, f := range fields {
		sf, err := NewField(f.Descriptor(), resolver)
		if err != nil {
			return err
		}
		sf.Position = &Position{Index: i}
		s.Fields = append(s.Fields, sf)
	}
	return nil
}

func (s *Schema) addAnnotation(an schema.Annotation) {
	curr, ok := s.Annotations[an.Name()]
	if !ok {
		s.Annotations[an.Name()] = an
		return
	}
	if m, ok := curr.(schema.Merger); ok {
		s.Annotations[an.Name()] = m.Merge(an)
	}
}

func (e *Edge) addAnnotation(an schema.Annotation) {
	addAnnotation(e.Annotations, an)
}

func (i *Index) addAnnotation(an schema.Annotation) {
	addAnnotation(i.Annotations, an)
}

func (f *Field) addAnnotation(an schema.Annotation) {
	addAnnotation(f.Annotations, an)
}

func addAnnotation(annotations map[string]any, an schema.Annotation) {
	curr, ok := annotations[an.Name()]
	if !ok {
		annotations[an.Name()] = an
		return
	}
	if m, ok := curr.(schema.Merger); ok {
		annotations[an.Name()] = m.Merge(an)
	}
}

func (f *Field) defaults() error {
	if !f.Default || !f.Type.Numeric() || f.DefaultKind == reflect.Func {
		return nil
	}
	n, ok := f.DefaultValue.(float64)
	if !ok {
		return fmt.Errorf("unexpected default value type for field: %q", f.Name)
	}
	switch t := f.Type; {
	case t >= field.TypeInt8 && t <= field.TypeInt64:
		f.DefaultValue = int64(n)
	case t >= field.TypeUint8 && t <= field.TypeUint64:
		f.DefaultValue = uint64(n)
	}
	return nil
}

// safeFields wraps the schema.Fields and mixin.Fields method with recover to ensure no panics in marshaling.
func safeFields(fd interface{ Fields() []ent.Field }) (fields []ent.Field, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("%T.Fields panics: %v", fd, v)
			fields = nil
		}
	}()
	return fd.Fields(), nil
}

// safeEdges wraps the schema.Edges method with recover to ensure no panics in marshaling.
func safeEdges(schema interface{ Edges() []ent.Edge }) (edges []ent.Edge, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("schema.Edges panics: %v", v)
			edges = nil
		}
	}()
	return schema.Edges(), nil
}

// safeIndexes wraps the schema.Indexes method with recover to ensure no panics in marshaling.
func safeIndexes(schema interface{ Indexes() []ent.Index }) (indexes []ent.Index, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("schema.Indexes panics: %v", v)
			indexes = nil
		}
	}()
	return schema.Indexes(), nil
}

// safeMixin wraps the schema.Mixin method with recover to ensure no panics in marshaling.
func safeMixin(schema ent.Interface) (mixin []ent.Mixin, err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("schema.Mixin panics: %v", v)
			mixin = nil
		}
	}()
	return schema.Mixin(), nil
}

func indirect(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
