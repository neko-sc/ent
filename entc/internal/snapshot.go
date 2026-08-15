// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/neko-sc/ent/entc/gen"
	"github.com/neko-sc/ent/entc/load"
)

// ErrStaleSnapshot marks a snapshot whose serialized representation is no
// longer compatible with the current generator.
var ErrStaleSnapshot = errors.New("stale schema snapshot")

// Snapshot describes the schema snapshot restore.
type Snapshot struct {
	Path   string      // Path to snapshot.
	Config *gen.Config // Config of codegen.
}

// Restore restores the generated package from the latest schema snapshot.
// If there is a conflict between upstream and local snapshots, it is merged
// before running the code generation.
func (s *Snapshot) Restore() error {
	buf, err := os.ReadFile(s.Path)
	if err != nil {
		return fmt.Errorf("unable to read snapshot schema %w", err)
	}
	snap, err := s.parseSnapshot(buf)
	if err != nil {
		return err
	}
	s.Config.Schema = snap.Schema
	s.Config.Package = snap.Package
	s.addFeatures(snap)
	graph, err := gen.NewGraph(s.Config, snap.Schemas...)
	if err != nil {
		return err
	}
	return graph.Gen()
}

// schemaIdent holds the schema identifier in snapshot file.
const schemaIdent = "const Schema"

// parseSnapshot parses the given buffer and extract the generated snapshot.
// If it encounters a merge-conflict, it will resolve it by merging the relevant
// parts for the codegen.
func (s *Snapshot) parseSnapshot(buf []byte) (*gen.Snapshot, error) {
	var (
		conflict bool
		matches  = make([][]byte, 0, 2)
		lines    = bytes.Split(buf, []byte("\n"))
	)
	for i := range lines {
		switch line := lines[i]; {
		case bytes.HasPrefix(line, []byte(schemaIdent)):
			matches = append(matches, line)
		case bytes.HasPrefix(line, []byte(conflictMarker)):
			conflict = true
		}
	}
	switch n := len(matches); {
	case n == 0:
		return nil, fmt.Errorf("schema snapshot was not found in %s", s.Path)
	case n > 1 && !conflict:
		return nil, fmt.Errorf("expect to have exactly 1 snapshot in %s", s.Path)
	}
	line, err := trim(matches[0])
	if err != nil {
		return nil, err
	}
	var header struct{ Version int }
	if err := json.Unmarshal(line, &header); err != nil {
		return nil, fmt.Errorf("%w: unmarshal snapshot header: %w", ErrStaleSnapshot, err)
	}
	if header.Version != 2 {
		return nil, fmt.Errorf("%w: unsupported schema snapshot version %d", ErrStaleSnapshot, header.Version)
	}
	local := &gen.Snapshot{}
	if err := json.Unmarshal(line, &local); err != nil {
		return nil, fmt.Errorf("%w: unmarshal snapshot: %v", ErrStaleSnapshot, err)
	}
	if !conflict || len(matches) == 1 {
		return local, nil
	}
	// In case of merge-conflict, we merge the 2 schemas.
	line, err = trim(matches[1])
	if err != nil {
		return nil, err
	}
	other := &gen.Snapshot{}
	if err := json.Unmarshal(line, &other); err != nil {
		return nil, fmt.Errorf("%w: unmarshal conflicting snapshot: %v", ErrStaleSnapshot, err)
	}
	if other.Version != 2 {
		return nil, fmt.Errorf("%w: unsupported schema snapshot version %d", ErrStaleSnapshot, other.Version)
	}
	if err := merge(local, other); err != nil {
		return nil, err
	}
	return local, nil
}

// addFeatures adds the features in the snapshot to the codegen config.
func (s *Snapshot) addFeatures(snap *gen.Snapshot) {
	add := make(map[string]gen.Feature)
	for _, name := range snap.Features {
		for _, feat := range gen.AllFeatures {
			if name == feat.Name {
				add[name] = feat
			}
		}
	}
	for _, feat := range s.Config.Features {
		delete(add, feat.Name)
	}
	for _, feat := range gen.AllFeatures {
		if _, ok := add[feat.Name]; ok {
			s.Config.Features = append(s.Config.Features, feat)
		}
	}
}

// merge the "other"/"upstream" snapshot to the "local" version.
func merge(local, other *gen.Snapshot) error {
	if local.Schema == "" {
		local.Schema = other.Schema
	}
	if local.Package == "" {
		local.Package = other.Package
	}
	locals := make(map[string]*load.Schema, len(local.Schemas))
	for _, schema := range local.Schemas {
		locals[schema.Name] = schema
	}
	// Merge "other" schemas.
	for _, schema := range other.Schemas {
		switch match, ok := locals[schema.Name]; {
		case !ok:
			local.Schemas = append(local.Schemas, schema)
		default:
			if err := mergeSchema(match, schema); err != nil {
				return fmt.Errorf("merge schema %q: %w", schema.Name, err)
			}
		}
	}
	// Merge codegen features.
	features := make(map[string]struct{}, len(local.Features))
	for _, feat := range local.Features {
		features[feat] = struct{}{}
	}
	for _, feat := range other.Features {
		if _, ok := features[feat]; !ok {
			local.Features = append(local.Features, feat)
		}
	}
	return nil
}

// mergeSchema merges to "local" the additional information in
// the "other" schema, that may be necessary for code-generation.
func mergeSchema(local, other *load.Schema) error {
	if local.Config.Table == "" {
		local.Config.Table = other.Config.Table
	}
	if local.Annotations == nil && other.Annotations != nil {
		local.Annotations = make(map[string]any)
	}
	for ant := range other.Annotations {
		if _, ok := local.Annotations[ant]; !ok {
			local.Annotations[ant] = other.Annotations[ant]
		}
	}
	fields := make(map[string]*load.Field, len(local.Fields))
	for _, f := range local.Fields {
		fields[f.Name] = f
	}
	for _, f := range other.Fields {
		switch match, ok := fields[f.Name]; {
		case !ok:
			local.Fields = append(local.Fields, f)
		default:
			if err := mergeField(match, f); err != nil {
				return err
			}
		}
	}
	edges := make(map[string]*load.Edge, len(local.Edges))
	for _, e := range local.Edges {
		edges[e.Name] = e
	}
	for _, e := range other.Edges {
		switch match, ok := edges[e.Name]; {
		case !ok:
			local.Edges = append(local.Edges, e)
		default:
			mergeEdge(match, e)
		}
	}
	return nil
}

// mergeField merges to "local" the additional information in
// the "other" field, that may be necessary for code-generation.
func mergeField(local, other *load.Field) error {
	if local.Type != other.Type {
		return fmt.Errorf("field %q has conflicting logical types %q and %q", local.Name, local.Type, other.Type)
	}
	if !reflect.DeepEqual(local.Semantic, other.Semantic) {
		return fmt.Errorf("field %q has conflicting semantic type metadata", local.Name)
	}
	if local.Annotations == nil && other.Annotations != nil {
		local.Annotations = make(map[string]any)
	}
	for ant := range other.Annotations {
		if _, ok := local.Annotations[ant]; !ok {
			local.Annotations[ant] = other.Annotations[ant]
		}
	}
	if !local.Immutable && other.Immutable {
		local.Immutable = other.Immutable
	}
	return nil
}

// mergeEdge merges to "local" the additional information in
// the "other" edge, that may be necessary for code-generation.
func mergeEdge(local, other *load.Edge) {
	if local.Annotations == nil && other.Annotations != nil {
		local.Annotations = make(map[string]any)
	}
	for ant := range other.Annotations {
		if _, ok := local.Annotations[ant]; !ok {
			local.Annotations[ant] = other.Annotations[ant]
		}
	}
}

// IsBuildError reports if the given error is an error from the Go command (e.g. syntax error).
func IsBuildError(err error) bool {
	if strings.HasPrefix(err.Error(), "entc/load: #") {
		return true
	}
	for _, s := range []string{
		"syntax error",
		"previous declaration",
		"invalid character",
		"could not import",
		"found '<<'",
	} {
		if strings.Contains(err.Error(), s) {
			return true
		}
	}
	return false
}

func trim(line []byte) ([]byte, error) {
	start := bytes.IndexByte(line, '"')
	end := bytes.LastIndexByte(line, '"')
	if start == -1 || start >= end {
		return nil, fmt.Errorf("unexpected snapshot line %s", line)
	}
	l, err := strconv.Unquote(string(line[start : end+1]))
	if err != nil {
		return nil, err
	}
	return []byte(l), nil
}
