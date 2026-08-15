// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package gen

import (
	"fmt"
	"go/token"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/neko-sc/ent/entc/load"
)

// Import describes one import required by a generated Go file. Alias is empty
// when the package's declared name can be used directly.
type Import struct {
	Alias string
	Path  string
}

// TypeRenderer allocates imports and renders semantic types for one generated file.
type TypeRenderer struct {
	currentPath string
	reserved    map[string]bool
	packages    map[string]string
	qualifiers  map[string]string
	imports     []Import
	allocated   bool
}

// NewTypeRenderer creates a per-file renderer. Generated names are reserved so
// imported package qualifiers cannot collide with declarations in the file.
func NewTypeRenderer(currentPackage load.Package, generatedNames ...string) *TypeRenderer {
	reserved := make(map[string]bool, len(generatedNames)+len(templateImportQualifiers)+64)
	for _, name := range templateImportQualifiers {
		reserved[name] = true
	}
	for _, name := range generatedNames {
		reserved[name] = true
	}
	for _, name := range goReservedNames {
		reserved[name] = true
	}
	return &TypeRenderer{
		currentPath: currentPackage.Path,
		reserved:    reserved,
		packages:    make(map[string]string),
	}
}

// Add collects all packages and type-parameter names referenced by types.
// Imports are allocated when Imports or Render is first called.
func (r *TypeRenderer) Add(types ...*load.TypeExpression) error {
	if r.allocated {
		return fmt.Errorf("gen: cannot add semantic types after imports were allocated")
	}
	for _, typ := range types {
		if err := r.collect(typ); err != nil {
			return err
		}
	}
	return nil
}

// Imports returns the deterministic imports needed by the collected types.
func (r *TypeRenderer) Imports() []Import {
	r.allocate()
	return append([]Import(nil), r.imports...)
}

// Render renders a collected semantic type as valid Go source.
func (r *TypeRenderer) Render(typ *load.TypeExpression) (string, error) {
	r.allocate()
	var builder strings.Builder
	if err := r.render(&builder, typ); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func (r *TypeRenderer) allocate() {
	if r.allocated {
		return
	}
	r.allocated = true
	r.qualifiers = make(map[string]string, len(r.packages))
	paths := make([]string, 0, len(r.packages))
	for path := range r.packages {
		if path != "" && path != r.currentPath {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	used := make(map[string]bool, len(r.reserved)+len(paths))
	for name := range r.reserved {
		used[name] = true
	}
	for _, path := range paths {
		name := identifier(r.packages[path])
		qualifier := name
		for suffix := 2; used[qualifier]; suffix++ {
			qualifier = name + strconv.Itoa(suffix)
		}
		used[qualifier] = true
		r.qualifiers[path] = qualifier
		alias := ""
		if qualifier != r.packages[path] {
			alias = qualifier
		}
		r.imports = append(r.imports, Import{Alias: alias, Path: path})
	}
}

func (r *TypeRenderer) collect(typ *load.TypeExpression) error {
	if typ == nil {
		return fmt.Errorf("gen: cannot collect nil semantic type")
	}
	switch typ.Kind {
	case load.TypeKindBasic:
	case load.TypeKindNamed:
		if typ.Named == nil {
			return fmt.Errorf("gen: named semantic type has no name")
		}
		r.addPackage(typ.Named.Package)
		for _, argument := range typ.Arguments {
			if err := r.collect(argument); err != nil {
				return err
			}
		}
	case load.TypeKindAlias:
		if typ.Alias == nil {
			return fmt.Errorf("gen: alias semantic type has no name")
		}
		r.addPackage(typ.Alias.Package)
		for _, argument := range typ.Arguments {
			if err := r.collect(argument); err != nil {
				return err
			}
		}
	case load.TypeKindPointer, load.TypeKindSlice, load.TypeKindArray, load.TypeKindChannel:
		return r.collect(typ.Element)
	case load.TypeKindMap:
		if err := r.collect(typ.Key); err != nil {
			return err
		}
		return r.collect(typ.Element)
	case load.TypeKindStruct:
		for _, field := range typ.Fields {
			if field.Package != nil {
				r.addPackage(*field.Package)
			}
			if err := r.collect(field.Type); err != nil {
				return err
			}
		}
	case load.TypeKindInterface:
		for _, method := range typ.Methods {
			if method.Package != nil {
				r.addPackage(*method.Package)
			}
			if err := r.collect(method.Signature); err != nil {
				return err
			}
		}
		for _, embedded := range typ.Embedded {
			if err := r.collect(embedded); err != nil {
				return err
			}
		}
	case load.TypeKindSignature:
		for _, parameter := range typ.TypeParameters {
			r.reserved[parameter.Name] = true
			if err := r.collect(parameter.Constraint); err != nil {
				return err
			}
		}
		for _, parameter := range typ.Parameters {
			if err := r.collect(parameter); err != nil {
				return err
			}
		}
		for _, result := range typ.Results {
			if err := r.collect(result); err != nil {
				return err
			}
		}
	case load.TypeKindTypeParameter:
		if typ.TypeParameter == nil {
			return fmt.Errorf("gen: type parameter has no identity")
		}
		r.reserved[typ.TypeParameter.Name] = true
	case load.TypeKindTuple:
		for _, variable := range typ.Variables {
			if err := r.collect(variable); err != nil {
				return err
			}
		}
	case load.TypeKindUnion:
		for _, term := range typ.Terms {
			if err := r.collect(term.Type); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("gen: unsupported semantic type kind %q", typ.Kind)
	}
	return nil
}

func (r *TypeRenderer) addPackage(pkg load.Package) {
	if pkg.Path != "" && pkg.Path != r.currentPath {
		r.packages[pkg.Path] = pkg.Name
	}
}

func (r *TypeRenderer) render(builder *strings.Builder, typ *load.TypeExpression) error {
	if typ == nil {
		return fmt.Errorf("gen: cannot render nil semantic type")
	}
	switch typ.Kind {
	case load.TypeKindBasic:
		builder.WriteString(string(typ.Basic))
	case load.TypeKindNamed:
		if typ.Named == nil {
			return fmt.Errorf("gen: named semantic type has no name")
		}
		r.renderTypeName(builder, typ.Named)
		return r.renderArguments(builder, typ.Arguments)
	case load.TypeKindAlias:
		if typ.Alias == nil {
			return fmt.Errorf("gen: alias semantic type has no name")
		}
		r.renderTypeName(builder, typ.Alias)
		return r.renderArguments(builder, typ.Arguments)
	case load.TypeKindPointer:
		builder.WriteByte('*')
		return r.render(builder, typ.Element)
	case load.TypeKindSlice:
		builder.WriteString("[]")
		return r.render(builder, typ.Element)
	case load.TypeKindArray:
		builder.WriteByte('[')
		builder.WriteString(strconv.FormatInt(typ.Length, 10))
		builder.WriteByte(']')
		return r.render(builder, typ.Element)
	case load.TypeKindMap:
		builder.WriteString("map[")
		if err := r.render(builder, typ.Key); err != nil {
			return err
		}
		builder.WriteByte(']')
		return r.render(builder, typ.Element)
	case load.TypeKindStruct:
		builder.WriteString("struct {")
		for i, field := range typ.Fields {
			if i != 0 {
				builder.WriteString("; ")
			}
			if field.Package != nil && field.Package.Path != r.currentPath {
				return fmt.Errorf("gen: anonymous struct field %q is not accessible from package %q", field.Name, r.currentPath)
			}
			builder.WriteString(field.Name)
			if !field.Embedded {
				builder.WriteByte(' ')
			}
			if err := r.render(builder, field.Type); err != nil {
				return err
			}
			if field.Tag != "" {
				builder.WriteByte(' ')
				builder.WriteString(strconv.Quote(field.Tag))
			}
		}
		builder.WriteByte('}')
	case load.TypeKindInterface:
		builder.WriteString("interface {")
		separator := ""
		for _, method := range typ.Methods {
			if method.Package != nil && method.Package.Path != r.currentPath {
				return fmt.Errorf("gen: interface method %q is not accessible from package %q", method.Name, r.currentPath)
			}
			builder.WriteString(separator)
			builder.WriteString(method.Name)
			if err := r.renderSignature(builder, method.Signature, false); err != nil {
				return err
			}
			separator = "; "
		}
		for _, embedded := range typ.Embedded {
			builder.WriteString(separator)
			if err := r.render(builder, embedded); err != nil {
				return err
			}
			separator = "; "
		}
		builder.WriteByte('}')
	case load.TypeKindSignature:
		return r.renderSignature(builder, typ, true)
	case load.TypeKindChannel:
		switch typ.ChannelDirection {
		case load.ChannelBoth:
			builder.WriteString("chan ")
		case load.ChannelSend:
			builder.WriteString("chan<- ")
		case load.ChannelReceive:
			builder.WriteString("<-chan ")
		default:
			return fmt.Errorf("gen: unsupported channel direction %q", typ.ChannelDirection)
		}
		return r.render(builder, typ.Element)
	case load.TypeKindTypeParameter:
		if typ.TypeParameter == nil {
			return fmt.Errorf("gen: type parameter has no identity")
		}
		builder.WriteString(typ.TypeParameter.Name)
	case load.TypeKindTuple:
		builder.WriteByte('(')
		if err := r.renderList(builder, typ.Variables, false); err != nil {
			return err
		}
		builder.WriteByte(')')
	case load.TypeKindUnion:
		for i, term := range typ.Terms {
			if i != 0 {
				builder.WriteString(" | ")
			}
			if term.Tilde {
				builder.WriteByte('~')
			}
			if err := r.render(builder, term.Type); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("gen: unsupported semantic type kind %q", typ.Kind)
	}
	return nil
}

func (r *TypeRenderer) renderTypeName(builder *strings.Builder, name *load.TypeName) {
	if qualifier := r.qualifiers[name.Package.Path]; qualifier != "" {
		builder.WriteString(qualifier)
		builder.WriteByte('.')
	}
	builder.WriteString(name.Name)
}

func (r *TypeRenderer) renderArguments(builder *strings.Builder, arguments []*load.TypeExpression) error {
	if len(arguments) == 0 {
		return nil
	}
	builder.WriteByte('[')
	if err := r.renderList(builder, arguments, false); err != nil {
		return err
	}
	builder.WriteByte(']')
	return nil
}

func (r *TypeRenderer) renderSignature(builder *strings.Builder, typ *load.TypeExpression, keyword bool) error {
	if typ == nil || typ.Kind != load.TypeKindSignature {
		return fmt.Errorf("gen: interface method has non-signature type")
	}
	if keyword {
		builder.WriteString("func")
	}
	if len(typ.TypeParameters) != 0 {
		builder.WriteByte('[')
		for i, parameter := range typ.TypeParameters {
			if i != 0 {
				builder.WriteString(", ")
			}
			builder.WriteString(parameter.Name)
			builder.WriteByte(' ')
			if err := r.render(builder, parameter.Constraint); err != nil {
				return err
			}
		}
		builder.WriteByte(']')
	}
	builder.WriteByte('(')
	if err := r.renderList(builder, typ.Parameters, typ.Variadic); err != nil {
		return err
	}
	builder.WriteByte(')')
	if len(typ.Results) == 1 {
		builder.WriteByte(' ')
		return r.render(builder, typ.Results[0])
	}
	if len(typ.Results) > 1 {
		builder.WriteString(" (")
		if err := r.renderList(builder, typ.Results, false); err != nil {
			return err
		}
		builder.WriteByte(')')
	}
	return nil
}

func (r *TypeRenderer) renderList(builder *strings.Builder, types []*load.TypeExpression, variadic bool) error {
	for i, typ := range types {
		if i != 0 {
			builder.WriteString(", ")
		}
		if variadic && i == len(types)-1 {
			if typ.Kind != load.TypeKindSlice {
				return fmt.Errorf("gen: variadic signature has non-slice final parameter")
			}
			builder.WriteString("...")
			if err := r.render(builder, typ.Element); err != nil {
				return err
			}
			continue
		}
		if err := r.render(builder, typ); err != nil {
			return err
		}
	}
	return nil
}

func identifier(name string) string {
	if name == "" {
		return "pkg"
	}
	var builder strings.Builder
	for i, character := range name {
		if unicode.IsLetter(character) || character == '_' || i > 0 && unicode.IsDigit(character) {
			builder.WriteRune(character)
		} else if builder.Len() == 0 {
			builder.WriteString("pkg")
		}
	}
	if builder.Len() == 0 {
		return "pkg"
	}
	return builder.String()
}

// templateImportQualifiers contains package qualifiers emitted directly by the
// built-in templates. Semantic imports reserve them regardless of which feature
// templates are enabled so rendering stays deterministic across generated files.
var templateImportQualifiers = []string{
	"context", "dialect", "driver", "ent", "entql", "entsql", "errors", "field", "fmt",
	"log", "math", "predicate", "privacy", "reflect", "schema", "sql", "sqlgraph", "sqljson",
	"stdsql", "strings", "sync", "time",
}

var goReservedNames = func() []string {
	names := []string{
		"any", "append", "bool", "byte", "cap", "clear", "close", "comparable",
		"complex", "complex128", "complex64", "copy", "delete", "error", "false",
		"float32", "float64", "imag", "int", "int16", "int32", "int64", "int8",
		"iota", "len", "make", "max", "min", "new", "nil", "panic", "print",
		"println", "real", "recover", "rune", "string", "true", "uint", "uint16",
		"uint32", "uint64", "uint8", "uintptr",
	}
	for _, keyword := range []string{
		"break", "case", "chan", "const", "continue", "default", "defer", "else",
		"fallthrough", "for", "func", "go", "goto", "if", "import", "interface",
		"map", "package", "range", "return", "select", "struct", "switch", "type", "var",
	} {
		if token.Lookup(keyword).IsKeyword() {
			names = append(names, keyword)
		}
	}
	return names
}()
