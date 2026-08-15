// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

// Package load is the interface for loading an ent/schema package into a Go program.
package load

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/neko-sc/ent"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

type (
	// A SchemaSpec holds a serializable version of an ent.Schema
	// and its Go package and module information.
	SchemaSpec struct {
		// Schemas defines the loaded schema descriptors.
		Schemas []*Schema

		// PkgPath is the package path of the loaded
		// ent.Schema package.
		PkgPath string

		// Module defines the module information for
		// the user schema package if exists.
		Module *packages.Module
	}

	// Config holds the configuration for loading an ent/schema package.
	Config struct {
		// Path is the path for the schema package.
		Path string
		// Names are the schema names to load. Empty means all schemas in the directory.
		Names []string
		// BuildFlags are forwarded to package loading and schema execution.
		BuildFlags []string
	}
)

// Load loads the schemas package and build the Go plugin with this info.
func (c *Config) Load() (*SchemaSpec, error) {
	schemaSpecification, positions, schemaPackage, err := c.load()
	if err != nil {
		return nil, fmt.Errorf("entc/load: parse schema dir: %w", err)
	}
	if len(c.Names) == 0 {
		return nil, fmt.Errorf("entc/load: no schema found in: %s", c.Path)
	}
	runtimeTypes, err := CollectRuntimeTypes([]*packages.Package{schemaPackage})
	if err != nil {
		return nil, fmt.Errorf("entc/load: collect semantic types: %w", err)
	}
	registry, err := renderRuntimeRegistry(runtimeTypes, schemaPackage)
	if err != nil {
		return nil, fmt.Errorf("entc/load: render semantic registry: %w", err)
	}
	var b bytes.Buffer
	err = buildTmpl.ExecuteTemplate(&b, "main", struct {
		*Config
		Package  string
		Registry runtimeRegistry
	}{
		Config:   c,
		Package:  schemaSpecification.PkgPath,
		Registry: registry,
	})
	if err != nil {
		return nil, fmt.Errorf("entc/load: execute template: %w", err)
	}
	buf, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("entc/load: format template: %w", err)
	}
	if err := os.MkdirAll(".entc", os.ModePerm); err != nil {
		return nil, err
	}
	target := fmt.Sprintf(".entc/%s.go", filename(schemaSpecification.PkgPath))
	if err := os.WriteFile(target, buf, 0644); err != nil {
		return nil, fmt.Errorf("entc/load: write file %s: %w", target, err)
	}
	defer os.RemoveAll(".entc")
	out, err := gorun(target, c.BuildFlags)
	if err != nil {
		return nil, err
	}
	schemas, err := unmarshalSchemaLines(out)
	if err != nil {
		return nil, err
	}
	schemaSpecification.Schemas = append(schemaSpecification.Schemas, schemas...)
	for _, s := range schemaSpecification.Schemas {
		s.Pos = positions[s.Name]
	}
	return schemaSpecification, nil
}

// entInterface holds the reflect.Type of ent.Interface.
var entInterface = reflect.TypeOf(struct{ ent.Interface }{}).Field(0).Type

// load the ent/schema info.
func (c *Config) load() (*SchemaSpec, map[string]string, *packages.Package, error) {
	loadedPackages, err := packages.Load(&packages.Config{
		BuildFlags: c.BuildFlags,
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedDeps | packages.NeedTypes |
			packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedTypesSizes |
			packages.NeedModule,
	}, c.Path, entInterface.PkgPath())
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading package: %w", err)
	}
	if len(loadedPackages) < 2 {
		// Check if the package loading failed due to Go-related
		// errors, such as 'missing go.sum entry'.
		if err := golist(c.Path, c.BuildFlags); err != nil {
			return nil, nil, nil, err
		}
		return nil, nil, nil, fmt.Errorf("missing package information for: %s", c.Path)
	}
	entPackage, schemaPackage := loadedPackages[0], loadedPackages[1]
	if len(schemaPackage.Errors) != 0 {
		return nil, nil, nil, c.loadError(schemaPackage.Errors[0])
	}
	if len(entPackage.Errors) != 0 {
		return nil, nil, nil, entPackage.Errors[0]
	}
	if loadedPackages[0].PkgPath != entInterface.PkgPath() {
		entPackage, schemaPackage = loadedPackages[1], loadedPackages[0]
	}
	names := make(map[string]string)
	schemaInterface := entPackage.Types.Scope().Lookup(entInterface.Name()).Type().Underlying().(*types.Interface)
	for identifier, definition := range schemaPackage.TypesInfo.Defs {
		schemaType, ok := definition.(*types.TypeName)
		if !ok || !identifier.IsExported() || !types.Implements(schemaType.Type(), schemaInterface) {
			continue
		}
		if _, ok := schemaType.Type().Underlying().(*types.Struct); !ok {
			return nil, nil, nil, fmt.Errorf("invalid schema type %T for %s", schemaType.Type().Underlying(), identifier.Name)
		}
		position := schemaPackage.Fset.Position(schemaType.Pos())
		names[identifier.Name] = fmt.Sprintf("%s:%d", position.Filename, position.Line)
	}
	if len(c.Names) == 0 {
		c.Names = slices.Sorted(maps.Keys(names))
	} else {
		sort.Strings(c.Names)
	}
	return &SchemaSpec{PkgPath: schemaPackage.PkgPath, Module: schemaPackage.Module}, names, schemaPackage, nil
}

func (c *Config) loadError(perr packages.Error) (err error) {
	if strings.Contains(perr.Msg, "import cycle not allowed") {
		if cause := c.cycleCause(); cause != "" {
			perr.Msg += "\n" + cause
		}
	}
	err = perr
	if perr.Pos == "" {
		// Strip "-:" prefix in case of empty position.
		err = errors.New(perr.Msg)
	}
	return err
}

func (c *Config) cycleCause() (cause string) {
	dir, err := parser.ParseDir(token.NewFileSet(), c.Path, nil, 0)
	// Ignore reporting in case of parsing
	// error, or there no packages to parse.
	if err != nil || len(dir) == 0 {
		return cause
	}
	// Find the package that contains the schema, or
	// extract the first package if there is only one.
	parsedPackage := dir[filepath.Base(c.Path)]
	if parsedPackage == nil {
		for _, v := range dir {
			parsedPackage = v
			break
		}
	}
	// Package local declarations used by schema fields.
	locals := make(map[string]bool)
	for _, f := range parsedPackage.Files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, s := range g.Specs {
				ts, ok := s.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				// Non-struct types such as "type Role int".
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					locals[ts.Name.Name] = true
					continue
				}
				var embedSchema bool
				astutil.Apply(st.Fields, func(c *astutil.Cursor) bool {
					f, ok := c.Node().(*ast.Field)
					if ok {
						switch x := f.Type.(type) {
						case *ast.SelectorExpr:
							if x.Sel.Name == "Schema" || x.Sel.Name == "Mixin" {
								embedSchema = true
							}
						case *ast.Ident:
							// A common pattern is to create local base schema to be embedded by other schemas.
							if name := strings.ToLower(x.Name); name == "schema" || name == "mixin" {
								embedSchema = true
							}
						}
					}
					// Stop traversing the AST in case an ~ent.Schema is embedded.
					return !embedSchema
				}, nil)
				if !embedSchema {
					locals[ts.Name.Name] = true
				}
			}
		}
	}
	// No local declarations to report.
	if len(locals) == 0 {
		return cause
	}
	// Usage of local declarations by schema fields.
	goTypes := make(map[string]bool)
	for _, f := range parsedPackage.Files {
		for _, d := range f.Decls {
			f, ok := d.(*ast.FuncDecl)
			if !ok || f.Name.Name != "Fields" || f.Type.Params.NumFields() != 0 || f.Type.Results.NumFields() != 1 {
				continue
			}
			astutil.Apply(f.Body, func(cursor *astutil.Cursor) bool {
				i, ok := cursor.Node().(*ast.Ident)
				if ok && locals[i.Name] {
					goTypes[i.Name] = true
				}
				return true
			}, nil)
		}
	}
	names := make([]string, 0, len(goTypes))
	for k := range goTypes {
		names = append(names, strconv.Quote(k))
	}
	sort.Strings(names)
	if len(names) > 0 {
		cause = fmt.Sprintf("To resolve this issue, move the custom types used by the generated code to a separate package: %s", strings.Join(names, ", "))
	}
	return cause
}

var (
	//go:embed template/main.tmpl schema.go
	files     embed.FS
	buildTmpl = templates()
)

func templates() *template.Template {
	tmpls, err := schemaTemplates()
	if err != nil {
		panic(err)
	}
	tmpl := template.Must(template.New("templates").
		ParseFS(files, "template/main.tmpl"))
	for _, t := range tmpls {
		tmpl = template.Must(tmpl.Parse(t))
	}
	return tmpl
}

// schemaTemplates turns the schema.go file and its import block into templates.
func schemaTemplates() ([]string, error) {
	var (
		imports []string
		code    bytes.Buffer
		fset    = token.NewFileSet()
		src, _  = files.ReadFile("schema.go")
	)
	f, err := parser.ParseFile(fset, "schema.go", src, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse schema file: %w", err)
	}
	for _, decl := range f.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok && decl.Tok == token.IMPORT {
			for _, spec := range decl.Specs {
				imports = append(imports, spec.(*ast.ImportSpec).Path.Value)
			}
			continue
		}
		if err := format.Node(&code, fset, decl); err != nil {
			return nil, fmt.Errorf("format node: %w", err)
		}
		code.WriteByte('\n')
	}
	return []string{
		fmt.Sprintf(`{{ define "schema" }} %s {{ end }}`, code.String()),
		fmt.Sprintf(`{{ define "imports" }} %s {{ end }}`, strings.Join(imports, "\n")),
	}, nil
}

func filename(packagePath string) string {
	name := strings.ReplaceAll(packagePath, "/", "_")
	return fmt.Sprintf("entc_%s_%d", name, time.Now().Unix())
}

func unmarshalSchemaLines(output string) ([]*Schema, error) {
	var schemas []*Schema
	for line := range strings.Lines(output) {
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			continue
		}
		schema, err := UnmarshalSchema([]byte(line))
		if err != nil {
			return nil, fmt.Errorf("entc/load: unmarshal schema %s: %w", line, err)
		}
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

type runtimeRegistry struct {
	Imports string
	Entries string
}

func renderRuntimeRegistry(runtimeTypes []RuntimeType, schemaPackage *packages.Package) (runtimeRegistry, error) {
	qualifiers := make(map[string]string)
	for _, runtimeType := range runtimeTypes {
		collectGoTypePackages(runtimeType.GoType, schemaPackage.PkgPath, qualifiers)
	}
	var registry runtimeRegistry
	for index, path := range slices.Sorted(maps.Keys(qualifiers)) {
		qualifiers[path] = "enttype" + strconv.Itoa(index+1)
		registry.Imports += fmt.Sprintf("%s %q\n", qualifiers[path], path)
	}
	for _, runtimeType := range runtimeTypes {
		encoded, err := json.Marshal(struct {
			Expression    *TypeExpression              `json:"expression"`
			Capabilities  TypeCapabilities             `json:"capabilities"`
			Compatibility map[string]TypeCompatibility `json:"compatibility"`
		}{runtimeType.Expression, runtimeType.Capabilities, runtimeType.Compatibility})
		if err != nil {
			return runtimeRegistry{}, err
		}
		registry.Entries += fmt.Sprintf("reflect.TypeFor[%s](): {id: %q, expression: %q},\n", types.TypeString(runtimeType.GoType, func(imported *types.Package) string {
			if imported.Path() == schemaPackage.PkgPath {
				return "entschema"
			}
			return qualifiers[imported.Path()]
		}), runtimeType.ID, encoded)
	}
	return registry, nil
}

func collectGoTypePackages(goType types.Type, currentPath string, packages map[string]string) {
	types.TypeString(goType, func(schemaPackage *types.Package) string {
		if schemaPackage.Path() != currentPath {
			packages[schemaPackage.Path()] = schemaPackage.Name()
		}
		return schemaPackage.Name()
	})
}

// run 'go run' command and return its output.
func gorun(target string, buildFlags []string) (string, error) {
	s, err := gocmd("run", target, buildFlags)
	if err != nil {
		return "", fmt.Errorf("entc/load: %w", err)
	}
	return s, nil
}

// golist checks if 'go list' can be executed on the given target.
func golist(target string, buildFlags []string) error {
	_, err := gocmd("list", target, buildFlags)
	return err
}

// goCmd runs a go command and returns its output.
func gocmd(command, target string, buildFlags []string) (string, error) {
	args := []string{command}
	args = append(args, buildFlags...)
	args = append(args, target)
	cmd := exec.Command("go", args...)
	stderr := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	cmd.Stderr = stderr
	cmd.Stdout = stdout
	if err := cmd.Run(); err != nil {
		return "", errors.New(strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
