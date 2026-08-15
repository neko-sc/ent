// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"fmt"
	"go/token"
	"go/types"
	"slices"
	"strconv"
	"strings"

	"github.com/neko-sc/ent/schema/field"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// RuntimeType describes one concrete Go type collected from typed field calls.
type RuntimeType struct {
	ID            TypeID
	GoType        types.Type
	Expression    *TypeExpression
	Capabilities  TypeCapabilities
	Compatibility map[string]TypeCompatibility
	Source        string
}

// CollectRuntimeTypes uses SSA generic instantiation to collect the concrete
// representation passed to field.newTypedBuilder. Instantiated helper and mixin
// bodies are included, preserving nested and phantom arguments.
func CollectRuntimeTypes(packagesToCollect []*packages.Package) ([]RuntimeType, error) {
	constructor, err := typedBuilderConstructor(packagesToCollect)
	if err != nil {
		return nil, err
	}
	program, _ := ssautil.AllPackages(packagesToCollect, ssa.InstantiateGenerics)
	program.Build()
	byType := make(map[types.Type]RuntimeType)
	byID := make(map[TypeID]RuntimeType)
	for function := range ssautil.AllFunctions(program) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				call, ok := instruction.(ssa.CallInstruction)
				if !ok {
					continue
				}
				callee := call.Common().StaticCallee()
				if callee == nil || originObject(callee) != constructor || len(callee.TypeArgs()) < 1 {
					continue
				}
				goType := callee.TypeArgs()[0]
				if containsTypeParameter(goType) {
					continue
				}
				position := function.Prog.Fset.Position(call.Pos()).String()
				runtimeType, err := cachedRuntimeType(byType, goType, position)
				if err != nil {
					return nil, err
				}
				if existing, ok := byID[runtimeType.ID]; !ok || compareSourcePosition(position, existing.Source) < 0 {
					runtimeType.Source = position
					byID[runtimeType.ID] = runtimeType
				}
			}
		}
	}
	result := make([]RuntimeType, 0, len(byID))
	for _, runtimeType := range byID {
		result = append(result, runtimeType)
	}
	slices.SortFunc(result, func(a, b RuntimeType) int { return compareTypeID(a.ID, b.ID) })
	return result, nil
}

func cachedRuntimeType(cache map[types.Type]RuntimeType, goType types.Type, position string) (RuntimeType, error) {
	if runtimeType, ok := cache[goType]; ok {
		if compareSourcePosition(position, runtimeType.Source) < 0 {
			runtimeType.Source = position
			cache[goType] = runtimeType
		}
		return runtimeType, nil
	}
	expression, err := TypeExpressionOf(goType)
	if err != nil {
		return RuntimeType{}, fmt.Errorf("%s: collect typed field representation: %w", position, err)
	}
	id, err := expression.ID()
	if err != nil {
		return RuntimeType{}, fmt.Errorf("%s: identify typed field representation: %w", position, err)
	}
	runtimeType := RuntimeType{
		ID:            id,
		GoType:        goType,
		Expression:    expression,
		Capabilities:  typeCapabilities(goType),
		Compatibility: typeCompatibility(goType),
		Source:        position,
	}
	cache[goType] = runtimeType
	return runtimeType, nil
}

func compareSourcePosition(first, second string) int {
	firstPosition := parseSourcePosition(first)
	secondPosition := parseSourcePosition(second)
	if comparison := strings.Compare(firstPosition.Filename, secondPosition.Filename); comparison != 0 {
		return comparison
	}
	if firstPosition.Line != secondPosition.Line {
		return firstPosition.Line - secondPosition.Line
	}
	return firstPosition.Column - secondPosition.Column
}

func parseSourcePosition(position string) token.Position {
	result := token.Position{Filename: position}
	lastSeparator := strings.LastIndexByte(position, ':')
	if lastSeparator < 0 {
		return result
	}
	last, err := strconv.Atoi(position[lastSeparator+1:])
	if err != nil {
		return result
	}
	previousSeparator := strings.LastIndexByte(position[:lastSeparator], ':')
	if previousSeparator < 0 {
		result.Filename = position[:lastSeparator]
		result.Line = last
		return result
	}
	line, err := strconv.Atoi(position[previousSeparator+1 : lastSeparator])
	if err != nil {
		result.Filename = position[:lastSeparator]
		result.Line = last
		return result
	}
	result.Filename = position[:previousSeparator]
	result.Line = line
	result.Column = last
	return result
}

func typedBuilderConstructor(packagesToSearch []*packages.Package) (*types.Func, error) {
	const fieldPackagePath = "github.com/neko-sc/ent/schema/field"
	visited := make(map[*packages.Package]bool)
	var find func(*packages.Package) *packages.Package
	find = func(loadedPackage *packages.Package) *packages.Package {
		if loadedPackage == nil || visited[loadedPackage] {
			return nil
		}
		visited[loadedPackage] = true
		if loadedPackage.PkgPath == fieldPackagePath {
			return loadedPackage
		}
		for _, imported := range loadedPackage.Imports {
			if found := find(imported); found != nil {
				return found
			}
		}
		return nil
	}
	for _, loadedPackage := range packagesToSearch {
		if fieldPackage := find(loadedPackage); fieldPackage != nil {
			constructor, ok := fieldPackage.Types.Scope().Lookup("newTypedBuilder").(*types.Func)
			if !ok {
				return nil, fmt.Errorf("load: field.newTypedBuilder is unavailable")
			}
			return constructor, nil
		}
	}
	return nil, fmt.Errorf("load: package %q is unavailable", fieldPackagePath)
}

func originObject(function *ssa.Function) types.Object {
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	return function.Object()
}

func containsTypeParameter(goType types.Type) bool {
	return typeHasParameter(goType, make(map[types.Type]bool))
}

func typeHasParameter(goType types.Type, visited map[types.Type]bool) bool {
	if goType == nil || visited[goType] {
		return false
	}
	visited[goType] = true
	switch goType := goType.(type) {
	case *types.TypeParam:
		return true
	case *types.Named:
		for argument := range goType.TypeArgs().Types() {
			if typeHasParameter(argument, visited) {
				return true
			}
		}
	case *types.Alias:
		for argument := range goType.TypeArgs().Types() {
			if typeHasParameter(argument, visited) {
				return true
			}
		}
		return typeHasParameter(goType.Rhs(), visited)
	case *types.Pointer:
		return typeHasParameter(goType.Elem(), visited)
	case *types.Slice:
		return typeHasParameter(goType.Elem(), visited)
	case *types.Array:
		return typeHasParameter(goType.Elem(), visited)
	case *types.Chan:
		return typeHasParameter(goType.Elem(), visited)
	case *types.Map:
		return typeHasParameter(goType.Key(), visited) || typeHasParameter(goType.Elem(), visited)
	case *types.Struct:
		for index := range goType.NumFields() {
			if typeHasParameter(goType.Field(index).Type(), visited) {
				return true
			}
		}
	case *types.Signature:
		return typeHasParameter(goType.Params(), visited) || typeHasParameter(goType.Results(), visited)
	case *types.Tuple:
		for index := range goType.Len() {
			if typeHasParameter(goType.At(index).Type(), visited) {
				return true
			}
		}
	case *types.Interface:
		goType = goType.Complete()
		for index := range goType.NumExplicitMethods() {
			if typeHasParameter(goType.ExplicitMethod(index).Type(), visited) {
				return true
			}
		}
		for index := range goType.NumEmbeddeds() {
			if typeHasParameter(goType.EmbeddedType(index), visited) {
				return true
			}
		}
	case *types.Union:
		for index := range goType.Len() {
			if typeHasParameter(goType.Term(index).Type(), visited) {
				return true
			}
		}
	}
	return false
}

func compareTypeID(a, b TypeID) int {
	return strings.Compare(string(a), string(b))
}

func typeCapabilities(goType types.Type) TypeCapabilities {
	underlying, err := TypeExpressionOf(goType.Underlying())
	if err != nil {
		panic(fmt.Sprintf("convert underlying type %s: %s", goType, err))
	}
	pointer := types.NewPointer(goType)
	if _, ok := goType.(*types.Pointer); ok {
		pointer = nil
	}
	return TypeCapabilities{
		Comparable:      types.Comparable(goType),
		Nillable:        isNillable(goType),
		Scanner:         implementsScanner(goType) || pointer != nil && implementsScanner(pointer),
		Valuer:          implementsValuer(goType),
		NullableScanner: isNullableScanner(goType),
		Validator:       types.Implements(goType, semanticInterfaces.validator),
		Stringer:        types.Implements(goType, semanticInterfaces.stringer),
		Adder:           implementsAdder(goType),
		Underlying:      underlying,
	}
}

func typeCompatibility(goType types.Type) map[string]TypeCompatibility {
	compatibility := make(map[string]TypeCompatibility)
	for logical, base := range logicalGoTypes(goType) {
		compatibility[logical.String()] = TypeCompatibility{
			Assignable:         types.AssignableTo(goType, base),
			Convertible:        logicalConvertibleTo(goType, logical, base),
			ReverseConvertible: types.ConvertibleTo(base, goType),
			Projection:         logicalProjection(goType, logical, base),
		}
	}
	return compatibility
}

func logicalConvertibleTo(goType types.Type, logical field.Type, base types.Type) bool {
	if !types.ConvertibleTo(goType, base) {
		return false
	}
	if logical != field.TypeString && logical != field.TypeEnum {
		return true
	}
	basic, ok := types.Unalias(goType).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.String
}

func logicalProjection(goType types.Type, logical field.Type, base types.Type) string {
	if types.AssignableTo(goType, base) || logicalConvertibleTo(goType, logical, base) {
		return ""
	}
	if logical == field.TypeBytes {
		if array, ok := goType.Underlying().(*types.Array); ok && types.Identical(array.Elem(), types.Typ[types.Byte]) {
			return "[:]"
		}
	}
	if logical != field.TypeString && logical != field.TypeEnum {
		return ""
	}
	const name = "String"
	object, _, _ := types.LookupFieldOrMethod(goType, true, nil, name)
	switch member := object.(type) {
	case *types.Func:
		signature, _ := member.Type().(*types.Signature)
		if signature != nil && signature.Params().Len() == 0 && signature.Results().Len() == 1 && types.Identical(signature.Results().At(0).Type(), base) {
			return name + "()"
		}
	case *types.Var:
		if types.Identical(member.Type(), base) {
			return name
		}
	}
	return ""
}

func logicalGoTypes(goType types.Type) map[field.Type]types.Type {
	basic := func(kind types.BasicKind) types.Type { return types.Typ[kind] }
	logicalTypes := map[field.Type]types.Type{
		field.TypeBool:    basic(types.Bool),
		field.TypeString:  basic(types.String),
		field.TypeEnum:    basic(types.String),
		field.TypeBytes:   types.NewSlice(basic(types.Byte)),
		field.TypeJSON:    types.NewSlice(basic(types.Byte)),
		field.TypeUUID:    types.NewArray(basic(types.Byte), 16),
		field.TypeInt:     basic(types.Int),
		field.TypeInt8:    basic(types.Int8),
		field.TypeInt16:   basic(types.Int16),
		field.TypeInt32:   basic(types.Int32),
		field.TypeInt64:   basic(types.Int64),
		field.TypeUint:    basic(types.Uint),
		field.TypeUint8:   basic(types.Uint8),
		field.TypeUint16:  basic(types.Uint16),
		field.TypeUint32:  basic(types.Uint32),
		field.TypeUint64:  basic(types.Uint64),
		field.TypeFloat32: basic(types.Float32),
		field.TypeFloat64: basic(types.Float64),
	}
	if named := typeName(goType); named != nil && named.Pkg() != nil {
		for _, importedPackage := range named.Pkg().Imports() {
			if importedPackage.Path() == "time" {
				logicalTypes[field.TypeTime] = importedPackage.Scope().Lookup("Time").Type()
				break
			}
		}
	}
	return logicalTypes
}

func isNillable(goType types.Type) bool {
	switch goType.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Signature, *types.Interface, *types.Chan:
		return true
	default:
		return false
	}
}

var semanticInterfaces = struct {
	scanner   *types.Interface
	validator *types.Interface
	stringer  *types.Interface
}{
	scanner: singleMethodInterface(
		"Scan",
		types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("any").Type())),
		types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("error").Type())),
	),
	validator: singleMethodInterface(
		"Validate",
		nil,
		types.NewTuple(types.NewVar(0, nil, "", types.Universe.Lookup("error").Type())),
	),
	stringer: singleMethodInterface(
		"String",
		nil,
		types.NewTuple(types.NewVar(0, nil, "", types.Typ[types.String])),
	),
}

func implementsScanner(goType types.Type) bool {
	return types.Implements(goType, semanticInterfaces.scanner)
}

func implementsValuer(goType types.Type) bool {
	object, _, _ := types.LookupFieldOrMethod(goType, true, nil, "Value")
	function, _ := object.(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	if signature == nil || signature.Params().Len() != 0 || signature.Results().Len() != 2 || !types.Identical(signature.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return false
	}
	value := typeName(signature.Results().At(0).Type())
	if value == nil || value.Pkg() == nil || value.Pkg().Path() != "database/sql/driver" || value.Name() != "Value" {
		return false
	}
	valuer, ok := value.Pkg().Scope().Lookup("Valuer").Type().Underlying().(*types.Interface)
	return ok && types.Implements(goType, valuer.Complete())
}

func typeName(goType types.Type) *types.TypeName {
	switch goType := goType.(type) {
	case *types.Named:
		return goType.Obj()
	case *types.Alias:
		return goType.Obj()
	default:
		return nil
	}
}

func singleMethodInterface(name string, parameters, results *types.Tuple) *types.Interface {
	method := types.NewFunc(0, nil, name, types.NewSignatureType(nil, nil, nil, parameters, results, false))
	return types.NewInterfaceType([]*types.Func{method}, nil).Complete()
}

func isNullableScanner(goType types.Type) bool {
	pointer, ok := goType.(*types.Pointer)
	return ok && implementsScanner(goType) && !implementsScanner(pointer.Elem())
}

func implementsAdder(goType types.Type) bool {
	object, _, _ := types.LookupFieldOrMethod(goType, true, nil, "Add")
	function, _ := object.(*types.Func)
	if function == nil {
		return false
	}
	signature, _ := function.Type().(*types.Signature)
	return signature != nil &&
		signature.Params().Len() == 1 &&
		signature.Results().Len() == 1 &&
		types.Identical(signature.Params().At(0).Type(), goType) &&
		types.Identical(signature.Results().At(0).Type(), goType)
}

// FieldTypeOf creates compiler-authored field metadata for a representation.
func FieldTypeOf(logical field.Type, representation *TypeExpression) (*FieldType, error) {
	if !logical.Valid() {
		return nil, fmt.Errorf("load: invalid logical field type %q", logical)
	}
	base := logicalTypeExpression(logical)
	if base == nil {
		return nil, fmt.Errorf("load: logical field type %q has no semantic base", logical)
	}
	if representation == nil {
		return nil, fmt.Errorf("load: logical field type %q has no representation", logical)
	}
	id, err := representation.ID()
	if err != nil {
		return nil, err
	}
	return &FieldType{
		ID:             id,
		Logical:        logicalType(logical),
		Base:           base,
		Representation: representation,
		Storage:        StorageType{Family: logical.String()},
	}, nil
}

func logicalTypeExpression(fieldType field.Type) *TypeExpression {
	basic := func(kind BasicKind) *TypeExpression { return &TypeExpression{Kind: TypeKindBasic, Basic: kind} }
	switch fieldType {
	case field.TypeBool:
		return basic(BasicKindBool)
	case field.TypeTime:
		return &TypeExpression{Kind: TypeKindNamed, Named: &TypeName{Package: Package{Path: "time", Name: "time"}, Name: "Time"}}
	case field.TypeJSON:
		return &TypeExpression{Kind: TypeKindSlice, Element: basic(BasicKindUint8)}
	case field.TypeOther:
		return &TypeExpression{Kind: TypeKindNamed, Named: &TypeName{Name: "any"}}
	case field.TypeUUID:
		return &TypeExpression{Kind: TypeKindArray, Length: 16, Element: basic(BasicKindUint8)}
	case field.TypeBytes:
		return &TypeExpression{Kind: TypeKindSlice, Element: basic(BasicKindUint8)}
	case field.TypeEnum, field.TypeString:
		return basic(BasicKindString)
	case field.TypeInt8:
		return basic(BasicKindInt8)
	case field.TypeInt16:
		return basic(BasicKindInt16)
	case field.TypeInt32:
		return basic(BasicKindInt32)
	case field.TypeInt:
		return basic(BasicKindInt)
	case field.TypeInt64:
		return basic(BasicKindInt64)
	case field.TypeUint8:
		return basic(BasicKindUint8)
	case field.TypeUint16:
		return basic(BasicKindUint16)
	case field.TypeUint32:
		return basic(BasicKindUint32)
	case field.TypeUint:
		return basic(BasicKindUint)
	case field.TypeUint64:
		return basic(BasicKindUint64)
	case field.TypeFloat32:
		return basic(BasicKindFloat32)
	case field.TypeFloat64:
		return basic(BasicKindFloat64)
	default:
		return nil
	}
}

func logicalType(fieldType field.Type) LogicalType {
	switch fieldType {
	case field.TypeBool:
		return LogicalTypeBool
	case field.TypeTime:
		return LogicalTypeTime
	case field.TypeJSON:
		return LogicalTypeJSON
	case field.TypeUUID:
		return LogicalTypeUUID
	case field.TypeBytes:
		return LogicalTypeBytes
	case field.TypeEnum:
		return LogicalTypeEnum
	case field.TypeString:
		return LogicalTypeString
	case field.TypeOther:
		return LogicalTypeOther
	case field.TypeInt8, field.TypeInt16, field.TypeInt32, field.TypeInt, field.TypeInt64:
		return LogicalTypeInt
	case field.TypeUint8, field.TypeUint16, field.TypeUint32, field.TypeUint, field.TypeUint64:
		return LogicalTypeUint
	case field.TypeFloat32, field.TypeFloat64:
		return LogicalTypeFloat
	default:
		return LogicalType(fieldType.String())
	}
}

// TypeExpressionOf converts an arbitrary go/types type to its semantic representation.
func TypeExpressionOf(goType types.Type) (*TypeExpression, error) {
	return (&typeConverter{
		active: make(map[types.Type]*TypeExpression),
		scopes: make(map[*types.TypeParam]typeParameterScope),
	}).convert(goType)
}

type typeParameterScope struct {
	id    int
	index int
}

type typeConverter struct {
	active    map[types.Type]*TypeExpression
	scopes    map[*types.TypeParam]typeParameterScope
	nextScope int
}

func (c *typeConverter) convert(goType types.Type) (*TypeExpression, error) {
	if goType == nil {
		return nil, fmt.Errorf("load: cannot convert nil Go type")
	}
	if active, ok := c.active[goType]; ok {
		return active, nil
	}
	switch goType := goType.(type) {
	case *types.Basic:
		return &TypeExpression{Kind: TypeKindBasic, Basic: BasicKind(goType.Name())}, nil
	case *types.Named:
		return c.named(goType)
	case *types.Alias:
		return c.alias(goType)
	case *types.Pointer:
		return c.unary(TypeKindPointer, goType, goType.Elem())
	case *types.Slice:
		return c.unary(TypeKindSlice, goType, goType.Elem())
	case *types.Array:
		expression := &TypeExpression{Kind: TypeKindArray, Length: goType.Len()}
		c.active[goType] = expression
		var err error
		expression.Element, err = c.convert(goType.Elem())
		delete(c.active, goType)
		return expression, err
	case *types.Map:
		expression := &TypeExpression{Kind: TypeKindMap}
		c.active[goType] = expression
		var err error
		if expression.Key, err = c.convert(goType.Key()); err == nil {
			expression.Element, err = c.convert(goType.Elem())
		}
		delete(c.active, goType)
		return expression, err
	case *types.Struct:
		return c.structType(goType)
	case *types.Interface:
		return c.interfaceType(goType)
	case *types.Signature:
		return c.signature(goType)
	case *types.Chan:
		expression, err := c.unary(TypeKindChannel, goType, goType.Elem())
		if err != nil {
			return nil, err
		}
		expression.ChannelDirection = channelDirection(goType.Dir())
		return expression, nil
	case *types.TypeParam:
		return c.typeParameterReference(goType), nil
	case *types.Tuple:
		return c.tuple(goType)
	case *types.Union:
		return c.union(goType)
	default:
		return nil, fmt.Errorf("load: unsupported Go type %T", goType)
	}
}

func (c *typeConverter) named(namedType *types.Named) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindNamed, Named: declaredTypeName(namedType.Obj())}
	c.active[namedType] = expression
	var err error
	expression.Arguments, err = c.typeList(namedType.TypeArgs())
	delete(c.active, namedType)
	return expression, err
}

func (c *typeConverter) alias(aliasType *types.Alias) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindAlias, Alias: declaredTypeName(aliasType.Obj())}
	c.active[aliasType] = expression
	var err error
	if expression.Arguments, err = c.typeList(aliasType.TypeArgs()); err == nil {
		expression.Target, err = c.convert(aliasType.Rhs())
	}
	delete(c.active, aliasType)
	return expression, err
}

func (c *typeConverter) unary(kind TypeKind, original, element types.Type) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: kind}
	c.active[original] = expression
	var err error
	expression.Element, err = c.convert(element)
	delete(c.active, original)
	return expression, err
}

func (c *typeConverter) structType(structType *types.Struct) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindStruct, Fields: make([]StructField, structType.NumFields())}
	c.active[structType] = expression
	defer delete(c.active, structType)
	for i := range structType.NumFields() {
		field := structType.Field(i)
		expression.Fields[i] = StructField{
			Name:     field.Name(),
			Package:  objectPackage(field),
			Tag:      structType.Tag(i),
			Embedded: field.Embedded(),
		}
		var err error
		if expression.Fields[i].Type, err = c.convert(field.Type()); err != nil {
			return nil, err
		}
	}
	return expression, nil
}

func (c *typeConverter) interfaceType(interfaceType *types.Interface) (*TypeExpression, error) {
	interfaceType = interfaceType.Complete()
	expression := &TypeExpression{
		Kind:     TypeKindInterface,
		Methods:  make([]InterfaceMethod, interfaceType.NumExplicitMethods()),
		Embedded: make([]*TypeExpression, interfaceType.NumEmbeddeds()),
	}
	c.active[interfaceType] = expression
	defer delete(c.active, interfaceType)
	for i := range interfaceType.NumExplicitMethods() {
		method := interfaceType.ExplicitMethod(i)
		expression.Methods[i] = InterfaceMethod{Name: method.Name(), Package: objectPackage(method)}
		var err error
		if expression.Methods[i].Signature, err = c.convert(method.Type()); err != nil {
			return nil, err
		}
	}
	for i := range interfaceType.NumEmbeddeds() {
		var err error
		if expression.Embedded[i], err = c.convert(interfaceType.EmbeddedType(i)); err != nil {
			return nil, err
		}
	}
	return expression, nil
}

func (c *typeConverter) signature(signatureType *types.Signature) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindSignature, Variadic: signatureType.Variadic()}
	c.active[signatureType] = expression
	defer delete(c.active, signatureType)
	restore := c.bindTypeParameters(signatureType.TypeParams())
	defer restore()
	var err error
	if expression.TypeParameters, err = c.typeParameterDeclarations(signatureType.TypeParams()); err != nil {
		return nil, err
	}
	if expression.Parameters, err = c.tupleTypes(signatureType.Params()); err != nil {
		return nil, err
	}
	if expression.Results, err = c.tupleTypes(signatureType.Results()); err != nil {
		return nil, err
	}
	return expression, nil
}

func (c *typeConverter) tuple(tupleType *types.Tuple) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindTuple}
	var err error
	expression.Variables, err = c.tupleTypes(tupleType)
	return expression, err
}

func (c *typeConverter) union(unionType *types.Union) (*TypeExpression, error) {
	expression := &TypeExpression{Kind: TypeKindUnion, Terms: make([]UnionTerm, unionType.Len())}
	c.active[unionType] = expression
	defer delete(c.active, unionType)
	for i := range unionType.Len() {
		term := unionType.Term(i)
		expression.Terms[i].Tilde = term.Tilde()
		var err error
		if expression.Terms[i].Type, err = c.convert(term.Type()); err != nil {
			return nil, err
		}
	}
	return expression, nil
}

func (c *typeConverter) typeParameterReference(typeParameterValue *types.TypeParam) *TypeExpression {
	parameter := typeParameter(typeParameterValue)
	if scope, ok := c.scopes[typeParameterValue]; ok {
		parameter.Scope = scope.id
		parameter.Index = scope.index
	}
	return &TypeExpression{Kind: TypeKindTypeParameter, TypeParameter: &parameter}
}

func (c *typeConverter) bindTypeParameters(parameters *types.TypeParamList) func() {
	if parameters == nil || parameters.Len() == 0 {
		return func() {}
	}
	c.nextScope++
	scope := c.nextScope
	previous := make(map[*types.TypeParam]typeParameterScope, parameters.Len())
	present := make(map[*types.TypeParam]bool, parameters.Len())
	for i := range parameters.Len() {
		parameter := parameters.At(i)
		previous[parameter], present[parameter] = c.scopes[parameter]
		c.scopes[parameter] = typeParameterScope{id: scope, index: i}
	}
	return func() {
		for parameter, binding := range previous {
			if present[parameter] {
				c.scopes[parameter] = binding
			} else {
				delete(c.scopes, parameter)
			}
		}
	}
}

func (c *typeConverter) typeParameterDeclarations(parameters *types.TypeParamList) ([]TypeParameter, error) {
	if parameters == nil {
		return nil, nil
	}
	result := make([]TypeParameter, parameters.Len())
	for i := range parameters.Len() {
		parameter := parameters.At(i)
		result[i] = typeParameter(parameter)
		binding := c.scopes[parameter]
		result[i].Scope = binding.id
		result[i].Index = binding.index
		var err error
		if result[i].Constraint, err = c.convert(parameter.Constraint()); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *typeConverter) typeList(types *types.TypeList) ([]*TypeExpression, error) {
	if types == nil {
		return nil, nil
	}
	result := make([]*TypeExpression, types.Len())
	for i := range types.Len() {
		var err error
		if result[i], err = c.convert(types.At(i)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (c *typeConverter) tupleTypes(tuple *types.Tuple) ([]*TypeExpression, error) {
	if tuple == nil {
		return nil, nil
	}
	result := make([]*TypeExpression, tuple.Len())
	for i := range tuple.Len() {
		var err error
		if result[i], err = c.convert(tuple.At(i).Type()); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func declaredTypeName(object *types.TypeName) *TypeName {
	return &TypeName{Package: packageOf(object.Pkg()), Name: object.Name()}
}

func typeParameter(parameter *types.TypeParam) TypeParameter {
	object := parameter.Obj()
	result := TypeParameter{
		Name:  object.Name(),
		Index: parameter.Index(),
		Owner: typeParameterOwner(object),
	}
	if object.Pkg() != nil {
		result.Package = packageOf(object.Pkg())
	}
	return result
}

func typeParameterOwner(object *types.TypeName) string {
	if object == nil || object.Parent() == nil {
		return ""
	}
	for _, name := range object.Pkg().Scope().Names() {
		owner := object.Pkg().Scope().Lookup(name)
		switch goType := owner.Type().(type) {
		case *types.Named:
			if typeParameterListContains(goType.TypeParams(), object) {
				return "type:" + owner.Name()
			}
		case *types.Signature:
			if typeParameterListContains(goType.TypeParams(), object) {
				return "function:" + owner.Name()
			}
		}
	}
	return ""
}

func typeParameterListContains(parameters *types.TypeParamList, object *types.TypeName) bool {
	if parameters == nil {
		return false
	}
	for index := range parameters.Len() {
		if parameters.At(index).Obj() == object {
			return true
		}
	}
	return false
}

func objectPackage(object types.Object) *Package {
	if object.Exported() || object.Pkg() == nil {
		return nil
	}
	packageValue := packageOf(object.Pkg())
	return &packageValue
}

func packageOf(packageValue *types.Package) Package {
	if packageValue == nil {
		return Package{}
	}
	return Package{Path: packageValue.Path(), Name: packageValue.Name()}
}

func channelDirection(direction types.ChanDir) ChannelDirection {
	switch direction {
	case types.SendOnly:
		return ChannelSend
	case types.RecvOnly:
		return ChannelReceive
	case types.SendRecv:
		return ChannelBoth
	default:
		panic(fmt.Sprintf("unexpected channel direction %d", direction))
	}
}
