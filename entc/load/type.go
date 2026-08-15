// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// TypeKind identifies a semantic Go type form.
type TypeKind string

const (
	TypeKindBasic         TypeKind = "basic"
	TypeKindNamed         TypeKind = "named"
	TypeKindAlias         TypeKind = "alias"
	TypeKindPointer       TypeKind = "pointer"
	TypeKindSlice         TypeKind = "slice"
	TypeKindArray         TypeKind = "array"
	TypeKindMap           TypeKind = "map"
	TypeKindStruct        TypeKind = "struct"
	TypeKindInterface     TypeKind = "interface"
	TypeKindSignature     TypeKind = "signature"
	TypeKindChannel       TypeKind = "channel"
	TypeKindTypeParameter TypeKind = "type_parameter"
	TypeKindTuple         TypeKind = "tuple"
	TypeKindUnion         TypeKind = "union"
)

// BasicKind identifies a predeclared basic Go type.
type BasicKind string

const (
	BasicKindInvalid        BasicKind = "invalid"
	BasicKindBool           BasicKind = "bool"
	BasicKindInt            BasicKind = "int"
	BasicKindInt8           BasicKind = "int8"
	BasicKindInt16          BasicKind = "int16"
	BasicKindInt32          BasicKind = "int32"
	BasicKindInt64          BasicKind = "int64"
	BasicKindUint           BasicKind = "uint"
	BasicKindUint8          BasicKind = "uint8"
	BasicKindUint16         BasicKind = "uint16"
	BasicKindUint32         BasicKind = "uint32"
	BasicKindUint64         BasicKind = "uint64"
	BasicKindUintptr        BasicKind = "uintptr"
	BasicKindFloat32        BasicKind = "float32"
	BasicKindFloat64        BasicKind = "float64"
	BasicKindComplex64      BasicKind = "complex64"
	BasicKindComplex128     BasicKind = "complex128"
	BasicKindString         BasicKind = "string"
	BasicKindByte           BasicKind = "byte"
	BasicKindRune           BasicKind = "rune"
	BasicKindUnsafePointer  BasicKind = "unsafe.Pointer"
	BasicKindUntypedBool    BasicKind = "untyped bool"
	BasicKindUntypedInt     BasicKind = "untyped int"
	BasicKindUntypedRune    BasicKind = "untyped rune"
	BasicKindUntypedFloat   BasicKind = "untyped float"
	BasicKindUntypedComplex BasicKind = "untyped complex"
	BasicKindUntypedString  BasicKind = "untyped string"
	BasicKindUntypedNil     BasicKind = "untyped nil"
)

// ChannelDirection identifies the direction of a channel type.
type ChannelDirection string

const (
	ChannelBoth    ChannelDirection = "both"
	ChannelSend    ChannelDirection = "send"
	ChannelReceive ChannelDirection = "receive"
)

// Package identifies a Go package. Path defines package identity and Name is
// retained for source rendering.
type Package struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// TypeName identifies a declared Go type.
type TypeName struct {
	Package Package `json:"package"`
	Name    string  `json:"name"`
}

// TypeParameter identifies a type parameter reference or declaration.
type TypeParameter struct {
	Package    Package         `json:"package"`
	Name       string          `json:"name"`
	Index      int             `json:"index"`
	Scope      int             `json:"scope,omitempty"`
	Owner      string          `json:"owner,omitempty"`
	Constraint *TypeExpression `json:"constraint,omitempty"`
}

// StructField describes one field in an anonymous struct type.
type StructField struct {
	Name     string          `json:"name"`
	Package  *Package        `json:"package,omitempty"`
	Type     *TypeExpression `json:"type"`
	Tag      string          `json:"tag,omitempty"`
	Embedded bool            `json:"embedded,omitempty"`
}

// InterfaceMethod describes one method in an interface method set.
type InterfaceMethod struct {
	Name      string          `json:"name"`
	Package   *Package        `json:"package,omitempty"`
	Signature *TypeExpression `json:"signature"`
}

// UnionTerm describes one term in an interface type-set union.
type UnionTerm struct {
	Type  *TypeExpression `json:"type"`
	Tilde bool            `json:"tilde,omitempty"`
}

// TypeExpression is a recursive, JSON-safe semantic representation of a Go type.
// Fields that do not apply to Kind are left empty.
type TypeExpression struct {
	Kind             TypeKind          `json:"kind"`
	Basic            BasicKind         `json:"basic,omitempty"`
	Named            *TypeName         `json:"named,omitempty"`
	Alias            *TypeName         `json:"alias,omitempty"`
	Target           *TypeExpression   `json:"target,omitempty"`
	Arguments        []*TypeExpression `json:"arguments,omitempty"`
	Element          *TypeExpression   `json:"element,omitempty"`
	Key              *TypeExpression   `json:"key,omitempty"`
	Length           int64             `json:"length,omitempty"`
	Fields           []StructField     `json:"fields,omitempty"`
	Methods          []InterfaceMethod `json:"methods,omitempty"`
	Embedded         []*TypeExpression `json:"embedded,omitempty"`
	TypeParameters   []TypeParameter   `json:"type_parameters,omitempty"`
	Parameters       []*TypeExpression `json:"parameters,omitempty"`
	Results          []*TypeExpression `json:"results,omitempty"`
	Variadic         bool              `json:"variadic,omitempty"`
	ChannelDirection ChannelDirection  `json:"channel_direction,omitempty"`
	TypeParameter    *TypeParameter    `json:"type_parameter,omitempty"`
	Variables        []*TypeExpression `json:"variables,omitempty"`
	Terms            []UnionTerm       `json:"terms,omitempty"`
}

// TypeExpressionFor returns a semantic expression for T when reflection exposes
// enough metadata to represent it accurately.
func TypeExpressionFor[T any]() (*TypeExpression, error) {
	return typeExpressionFor(reflect.TypeFor[T]())
}

func typeExpressionFor(goType reflect.Type) (*TypeExpression, error) {
	return reflectTypeExpression(goType)
}

func reflectTypeExpression(goType reflect.Type) (*TypeExpression, error) {
	if goType == nil {
		return nil, fmt.Errorf("load: cannot convert nil reflected type")
	}
	if goType.Kind() == reflect.UnsafePointer {
		return &TypeExpression{Kind: TypeKindBasic, Basic: BasicKindUnsafePointer}, nil
	}
	if goType.Name() != "" && goType.PkgPath() != "" {
		return reflectedNamedTypeExpression(goType)
	}
	switch goType.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return &TypeExpression{Kind: TypeKindBasic, Basic: BasicKind(goType.String())}, nil
	case reflect.UnsafePointer:
		return &TypeExpression{Kind: TypeKindBasic, Basic: BasicKindUnsafePointer}, nil
	case reflect.Pointer, reflect.Slice:
		element, err := reflectTypeExpression(goType.Elem())
		if err != nil {
			return nil, err
		}
		kind := TypeKindPointer
		if goType.Kind() == reflect.Slice {
			kind = TypeKindSlice
		}
		return &TypeExpression{Kind: kind, Element: element}, nil
	case reflect.Array:
		element, err := reflectTypeExpression(goType.Elem())
		if err != nil {
			return nil, err
		}
		return &TypeExpression{Kind: TypeKindArray, Length: int64(goType.Len()), Element: element}, nil
	case reflect.Map:
		key, err := reflectTypeExpression(goType.Key())
		if err != nil {
			return nil, err
		}
		element, err := reflectTypeExpression(goType.Elem())
		if err != nil {
			return nil, err
		}
		return &TypeExpression{Kind: TypeKindMap, Key: key, Element: element}, nil
	case reflect.Chan:
		element, err := reflectTypeExpression(goType.Elem())
		if err != nil {
			return nil, err
		}
		return &TypeExpression{Kind: TypeKindChannel, ChannelDirection: reflectedChannelDirection(goType.ChanDir()), Element: element}, nil
	case reflect.Struct, reflect.Interface, reflect.Func:
		return nil, fmt.Errorf("load: reflected anonymous %s type %q requires compiler metadata", goType.Kind(), goType)
	default:
		return nil, fmt.Errorf("load: unsupported reflected type %q (%s)", goType, goType.Kind())
	}
}

func reflectedNamedTypeExpression(goType reflect.Type) (*TypeExpression, error) {
	declaredName, arguments, err := parseReflectedNamedType(goType.Name())
	if err != nil {
		return nil, fmt.Errorf("load: parse reflected type %q: %w", goType, err)
	}
	packageName, err := reflectedDeclaredPackageName(goType.String())
	if err != nil {
		return nil, fmt.Errorf("load: reflected type %q: %w", goType, err)
	}
	return &TypeExpression{
		Kind: TypeKindNamed,
		Named: &TypeName{
			Package: Package{Path: goType.PkgPath(), Name: packageName},
			Name:    declaredName,
		},
		Arguments: arguments,
	}, nil
}

func reflectedDeclaredPackageName(typeString string) (string, error) {
	separator := strings.IndexByte(typeString, '.')
	if separator < 1 {
		return "", fmt.Errorf("has no declared package name")
	}
	packageName := typeString[:separator]
	if !tokenIdentifier(packageName) {
		return "", fmt.Errorf("has invalid declared package name %q", packageName)
	}
	return packageName, nil
}

func parseReflectedNamedType(name string) (string, []*TypeExpression, error) {
	bracket := strings.IndexByte(name, '[')
	if bracket < 0 {
		if !tokenIdentifier(name) {
			return "", nil, fmt.Errorf("invalid declared name %q", name)
		}
		return name, nil, nil
	}
	if !tokenIdentifier(name[:bracket]) || name[len(name)-1] != ']' {
		return "", nil, fmt.Errorf("invalid instantiated name %q", name)
	}
	parser := reflectedTypeParser{source: name[bracket+1 : len(name)-1]}
	arguments, err := parser.typeList(0)
	if err != nil {
		return "", nil, err
	}
	if parser.offset != len(parser.source) {
		return "", nil, fmt.Errorf("unexpected %q", parser.source[parser.offset:])
	}
	return name[:bracket], arguments, nil
}

type reflectedTypeParser struct {
	source string
	offset int
}

func (p *reflectedTypeParser) typeList(closing byte) ([]*TypeExpression, error) {
	var expressions []*TypeExpression
	for {
		if p.offset == len(p.source) {
			if closing != 0 {
				return nil, fmt.Errorf("missing %q", closing)
			}
			return expressions, nil
		}
		if closing != 0 && p.source[p.offset] == closing {
			p.offset++
			return expressions, nil
		}
		expression, err := p.typeExpression()
		if err != nil {
			return nil, err
		}
		expressions = append(expressions, expression)
		if p.offset == len(p.source) {
			if closing != 0 {
				return nil, fmt.Errorf("missing %q", closing)
			}
			return expressions, nil
		}
		switch p.source[p.offset] {
		case ',':
			p.offset++
		case closing:
			p.offset++
			return expressions, nil
		default:
			return nil, fmt.Errorf("unexpected %q", p.source[p.offset:])
		}
	}
}

func (p *reflectedTypeParser) typeExpression() (*TypeExpression, error) {
	if strings.HasPrefix(p.source[p.offset:], "*") {
		p.offset++
		return p.unaryTypeExpression(TypeKindPointer)
	}
	if strings.HasPrefix(p.source[p.offset:], "[]") {
		p.offset += 2
		return p.unaryTypeExpression(TypeKindSlice)
	}
	if strings.HasPrefix(p.source[p.offset:], "map[") {
		p.offset += len("map[")
		key, err := p.typeExpression()
		if err != nil {
			return nil, err
		}
		if p.offset == len(p.source) || p.source[p.offset] != ']' {
			return nil, fmt.Errorf("map key has no closing bracket")
		}
		p.offset++
		element, err := p.typeExpression()
		if err != nil {
			return nil, err
		}
		return &TypeExpression{Kind: TypeKindMap, Key: key, Element: element}, nil
	}
	if p.source[p.offset] == '[' {
		end := strings.IndexByte(p.source[p.offset:], ']')
		if end < 0 {
			return nil, fmt.Errorf("array length has no closing bracket")
		}
		length, err := strconv.ParseInt(p.source[p.offset+1:p.offset+end], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse array length: %w", err)
		}
		p.offset += end + 1
		element, err := p.typeExpression()
		if err != nil {
			return nil, err
		}
		return &TypeExpression{Kind: TypeKindArray, Length: length, Element: element}, nil
	}
	start := p.offset
	depth := 0
	for p.offset < len(p.source) {
		switch p.source[p.offset] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				goto parsed
			}
			depth--
		case ',':
			if depth == 0 {
				goto parsed
			}
		}
		p.offset++
	}
parsed:
	token := p.source[start:p.offset]
	if token == "" {
		return nil, fmt.Errorf("empty type argument")
	}
	if basic := reflectedBasicKind(token); basic != BasicKindInvalid {
		return &TypeExpression{Kind: TypeKindBasic, Basic: basic}, nil
	}
	bracket := strings.IndexByte(token, '[')
	identity := token
	if bracket >= 0 {
		identity = token[:bracket]
	}
	separator := strings.LastIndexByte(identity, '.')
	if separator < 1 || separator == len(identity)-1 {
		return nil, fmt.Errorf("type argument %q has no package path", token)
	}
	packagePath, declaredName := identity[:separator], identity[separator+1:]
	arguments := []*TypeExpression(nil)
	if bracket >= 0 {
		if token[len(token)-1] != ']' {
			return nil, fmt.Errorf("instantiated type argument %q has no closing bracket", token)
		}
		nested := reflectedTypeParser{source: token[bracket+1 : len(token)-1]}
		var err error
		if arguments, err = nested.typeList(0); err != nil {
			return nil, err
		}
	}
	return &TypeExpression{
		Kind: TypeKindNamed,
		Named: &TypeName{
			Package: Package{Path: packagePath, Name: reflectedPackageName(packagePath)},
			Name:    declaredName,
		},
		Arguments: arguments,
	}, nil
}

func (p *reflectedTypeParser) unaryTypeExpression(kind TypeKind) (*TypeExpression, error) {
	element, err := p.typeExpression()
	if err != nil {
		return nil, err
	}
	return &TypeExpression{Kind: kind, Element: element}, nil
}

func reflectedBasicKind(name string) BasicKind {
	switch BasicKind(name) {
	case BasicKindBool, BasicKindInt, BasicKindInt8, BasicKindInt16, BasicKindInt32, BasicKindInt64,
		BasicKindUint, BasicKindUint8, BasicKindUint16, BasicKindUint32, BasicKindUint64,
		BasicKindUintptr, BasicKindFloat32, BasicKindFloat64, BasicKindComplex64, BasicKindComplex128,
		BasicKindString, BasicKindByte, BasicKindRune, BasicKindUnsafePointer:
		return BasicKind(name)
	default:
		return BasicKindInvalid
	}
}

func reflectedPackageName(packagePath string) string {
	if separator := strings.LastIndexByte(packagePath, '/'); separator >= 0 {
		packagePath = packagePath[separator+1:]
	}
	return packagePath
}

func reflectedChannelDirection(direction reflect.ChanDir) ChannelDirection {
	switch direction {
	case reflect.BothDir:
		return ChannelBoth
	case reflect.SendDir:
		return ChannelSend
	case reflect.RecvDir:
		return ChannelReceive
	default:
		panic(fmt.Sprintf("unexpected reflected channel direction %d", direction))
	}
}

func tokenIdentifier(value string) bool {
	for index, character := range value {
		if character != '_' && !unicode.IsLetter(character) && (index == 0 || !unicode.IsDigit(character)) {
			return false
		}
	}
	return value != ""
}

// TypeID is a stable SHA-256 identity for a semantic Go type.
type TypeID string

// LogicalType identifies the field family independently from its Go representation.
type LogicalType string

const (
	LogicalTypeBool   LogicalType = "bool"
	LogicalTypeTime   LogicalType = "time"
	LogicalTypeJSON   LogicalType = "json"
	LogicalTypeUUID   LogicalType = "uuid"
	LogicalTypeBytes  LogicalType = "bytes"
	LogicalTypeEnum   LogicalType = "enum"
	LogicalTypeString LogicalType = "string"
	LogicalTypeInt    LogicalType = "int"
	LogicalTypeUint   LogicalType = "uint"
	LogicalTypeFloat  LogicalType = "float"
	LogicalTypeOther  LogicalType = "other"
)

// StorageType describes the default SQL storage family and dialect overrides.
type StorageType struct {
	Family   string            `json:"family,omitempty"`
	Dialects map[string]string `json:"dialects,omitempty"`
}

// TypeCapabilities records compiler-derived properties used by generation.
type TypeCapabilities struct {
	Comparable                bool            `json:"comparable,omitempty"`
	Nillable                  bool            `json:"nillable,omitempty"`
	Scanner                   bool            `json:"scanner,omitempty"`
	Valuer                    bool            `json:"valuer,omitempty"`
	NullableScanner           bool            `json:"nullable_scanner,omitempty"`
	Validator                 bool            `json:"validator,omitempty"`
	Stringer                  bool            `json:"stringer,omitempty"`
	Adder                     bool            `json:"adder,omitempty"`
	Underlying                *TypeExpression `json:"underlying,omitempty"`
	AssignableToLogical       bool            `json:"assignable_to_logical,omitempty"`
	ConvertibleToLogical      bool            `json:"convertible_to_logical,omitempty"`
	LogicalReverseConvertible bool            `json:"logical_reverse_convertible,omitempty"`
	LogicalProjection         string          `json:"logical_projection,omitempty"`
}

// TypeCompatibility records compiler-derived conversion properties between a
// representation and one logical field base type.
type TypeCompatibility struct {
	Assignable         bool   `json:"assignable,omitempty"`
	Convertible        bool   `json:"convertible,omitempty"`
	ReverseConvertible bool   `json:"reverse_convertible,omitempty"`
	Projection         string `json:"projection,omitempty"`
}

// FieldType connects a logical field family to its compiler-authored Go type.
type FieldType struct {
	ID             TypeID           `json:"id"`
	Logical        LogicalType      `json:"logical"`
	Base           *TypeExpression  `json:"base"`
	Representation *TypeExpression  `json:"representation"`
	Storage        StorageType      `json:"storage,omitempty"`
	Capabilities   TypeCapabilities `json:"capabilities,omitempty"`
}

// Canonical returns the deterministic, alias-independent identity encoding of t.
func (t *TypeExpression) Canonical() ([]byte, error) {
	var encoder canonicalTypeEncoder
	if err := encoder.typeExpression(t); err != nil {
		return nil, err
	}
	return encoder.buffer.Bytes(), nil
}

// ID returns the stable identity hash of t.
func (t *TypeExpression) ID() (TypeID, error) {
	canonical, err := t.Canonical()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(canonical)
	return TypeID(hex.EncodeToString(hash[:])), nil
}

type canonicalTypeEncoder struct {
	buffer bytes.Buffer
}

func (e *canonicalTypeEncoder) token(value string) {
	e.buffer.WriteString(strconv.Itoa(len(value)))
	e.buffer.WriteByte(':')
	e.buffer.WriteString(value)
}

func (e *canonicalTypeEncoder) integer(value int64) {
	e.token(strconv.FormatInt(value, 10))
}

func (e *canonicalTypeEncoder) boolean(value bool) {
	if value {
		e.token("1")
	} else {
		e.token("0")
	}
}

func (e *canonicalTypeEncoder) packagePath(packageValue *Package) {
	if packageValue == nil {
		e.token("")
		return
	}
	e.token(packageValue.Path)
}

func (e *canonicalTypeEncoder) typeExpression(expression *TypeExpression) error {
	if expression == nil {
		return fmt.Errorf("load: nil semantic type")
	}
	if expression.Kind == TypeKindAlias {
		if expression.Target == nil {
			return fmt.Errorf("load: alias type has no target")
		}
		return e.typeExpression(expression.Target)
	}
	e.token(string(expression.Kind))
	switch expression.Kind {
	case TypeKindBasic:
		e.token(string(expression.Basic))
	case TypeKindNamed:
		if expression.Named == nil || expression.Named.Name == "" {
			return fmt.Errorf("load: named type has no name")
		}
		e.token(expression.Named.Package.Path)
		e.token(expression.Named.Name)
		if err := e.typeExpressions(expression.Arguments); err != nil {
			return err
		}
	case TypeKindPointer, TypeKindSlice:
		return e.typeExpression(expression.Element)
	case TypeKindArray:
		e.integer(expression.Length)
		return e.typeExpression(expression.Element)
	case TypeKindMap:
		if err := e.typeExpression(expression.Key); err != nil {
			return err
		}
		return e.typeExpression(expression.Element)
	case TypeKindStruct:
		e.integer(int64(len(expression.Fields)))
		for _, field := range expression.Fields {
			e.token(field.Name)
			e.packagePath(field.Package)
			e.token(field.Tag)
			e.boolean(field.Embedded)
			if err := e.typeExpression(field.Type); err != nil {
				return err
			}
		}
	case TypeKindInterface:
		methods := make([][]byte, 0, len(expression.Methods))
		for _, method := range expression.Methods {
			var methodEncoder canonicalTypeEncoder
			methodEncoder.token(method.Name)
			methodEncoder.packagePath(method.Package)
			if err := methodEncoder.typeExpression(method.Signature); err != nil {
				return err
			}
			methods = append(methods, methodEncoder.buffer.Bytes())
		}
		slices.SortFunc(methods, bytes.Compare)
		e.integer(int64(len(methods)))
		for _, method := range methods {
			e.token(string(method))
		}
		embedded := make([][]byte, 0, len(expression.Embedded))
		for _, embeddedExpression := range expression.Embedded {
			var embeddedEncoder canonicalTypeEncoder
			if err := embeddedEncoder.typeExpression(embeddedExpression); err != nil {
				return err
			}
			embedded = append(embedded, embeddedEncoder.buffer.Bytes())
		}
		slices.SortFunc(embedded, bytes.Compare)
		e.integer(int64(len(embedded)))
		for _, encodedEmbedded := range embedded {
			e.token(string(encodedEmbedded))
		}
	case TypeKindSignature:
		e.integer(int64(len(expression.TypeParameters)))
		for _, parameter := range expression.TypeParameters {
			if parameter.Constraint == nil {
				return fmt.Errorf("load: type parameter %q has no constraint", parameter.Name)
			}
			if err := e.typeExpression(parameter.Constraint); err != nil {
				return err
			}
		}
		if err := e.typeExpressions(expression.Parameters); err != nil {
			return err
		}
		if err := e.typeExpressions(expression.Results); err != nil {
			return err
		}
		e.boolean(expression.Variadic)
	case TypeKindChannel:
		e.token(string(expression.ChannelDirection))
		return e.typeExpression(expression.Element)
	case TypeKindTypeParameter:
		if expression.TypeParameter == nil {
			return fmt.Errorf("load: type parameter has no identity")
		}
		if expression.TypeParameter.Scope != 0 {
			e.token("bound")
			e.integer(int64(expression.TypeParameter.Scope))
			e.integer(int64(expression.TypeParameter.Index))
		} else {
			if expression.TypeParameter.Owner == "" {
				return fmt.Errorf("load: free type parameter %q has no owner", expression.TypeParameter.Name)
			}
			e.token("free")
			e.token(expression.TypeParameter.Package.Path)
			e.token(expression.TypeParameter.Owner)
			e.integer(int64(expression.TypeParameter.Index))
		}
	case TypeKindTuple:
		return e.typeExpressions(expression.Variables)
	case TypeKindUnion:
		terms := make([][]byte, 0, len(expression.Terms))
		for _, term := range expression.Terms {
			var termEncoder canonicalTypeEncoder
			termEncoder.boolean(term.Tilde)
			if err := termEncoder.typeExpression(term.Type); err != nil {
				return err
			}
			terms = append(terms, termEncoder.buffer.Bytes())
		}
		slices.SortFunc(terms, bytes.Compare)
		e.integer(int64(len(terms)))
		for _, term := range terms {
			e.token(string(term))
		}
	default:
		return fmt.Errorf("load: unsupported semantic type kind %q", expression.Kind)
	}
	return nil
}

func (e *canonicalTypeEncoder) typeExpressions(expressions []*TypeExpression) error {
	e.integer(int64(len(expressions)))
	for _, expression := range expressions {
		if err := e.typeExpression(expression); err != nil {
			return err
		}
	}
	return nil
}
