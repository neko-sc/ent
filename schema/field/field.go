// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field

import (
	"database/sql"
	"database/sql/driver"
	"encoding"
	"fmt"
	"reflect"

	"github.com/neko-sc/ent/schema"
)

// ValidatorKind identifies the value accepted by a field validator.
type ValidatorKind uint8

const (
	// ValidatorLogical accepts the field family's logical base value.
	ValidatorLogical ValidatorKind = iota
	// ValidatorRepresentation accepts the generated Go representation.
	ValidatorRepresentation
)

// A Descriptor describes a field configuration.
type Descriptor struct {
	Tag              string                  // struct tag.
	Size             int                     // varchar size.
	Name             string                  // field name.
	Type             Type                    // logical field type.
	RuntimeType      reflect.Type            `json:"-"` // opaque key for compiler-authored type metadata.
	ValueScanner     any                     // custom field codec.
	Unique           bool                    // unique index of field.
	Nillable         bool                    // nillable struct field.
	Optional         bool                    // nullable field in database.
	Immutable        bool                    // create only field.
	Default          any                     // default value on create.
	UpdateDefault    any                     // default value on update.
	Validators       []any                   // validator functions in fluent call order.
	ValidatorKinds   []ValidatorKind         // accepted value kind for each validator.
	StorageKey       string                  // sql column name.
	Enums            []struct{ N, V string } // enum values.
	Sensitive        bool                    // sensitive info string field.
	SchemaType       map[string]string       // override the schema type.
	Annotations      []schema.Annotation     // field annotations.
	Comment          string                  // field comment.
	Deprecated       bool                    // mark the field as deprecated.
	DeprecatedReason string                  // deprecation reason.
	Err              error
}

// ValueScanner groups the database value and scan methods implemented by custom Go types.
type ValueScanner interface {
	driver.Valuer
	sql.Scanner
}

// TypeValueScanner converts between a represented Go value and its database scanner.
type TypeValueScanner[T any] interface {
	Value(T) (driver.Value, error)
	ScanValue() ValueScanner
	FromValue(driver.Value) (T, error)
}

// TextValueScanner stores values with encoding.TextMarshaler and scans them with encoding.TextUnmarshaler.
type TextValueScanner[T interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
}] struct{}

// Value implements TypeValueScanner.Value.
func (TextValueScanner[T]) Value(value T) (driver.Value, error) {
	return value.MarshalText()
}

// ScanValue implements TypeValueScanner.ScanValue.
func (TextValueScanner[T]) ScanValue() ValueScanner {
	return new(sql.NullString)
}

// FromValue implements TypeValueScanner.FromValue.
func (TextValueScanner[T]) FromValue(value driver.Value) (ret T, err error) {
	scanner, ok := value.(*sql.NullString)
	if !ok {
		return ret, fmt.Errorf("unexpected input for FromValue: %T", value)
	}
	ret = initialize(ret)
	if scanner.Valid {
		err = ret.UnmarshalText([]byte(scanner.String))
	}
	return ret, err
}

// BinaryValueScanner stores values with encoding.BinaryMarshaler and scans them with encoding.BinaryUnmarshaler.
type BinaryValueScanner[T interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}] struct{}

// Value implements TypeValueScanner.Value.
func (BinaryValueScanner[T]) Value(value T) (driver.Value, error) {
	return value.MarshalBinary()
}

// ScanValue implements TypeValueScanner.ScanValue.
func (BinaryValueScanner[T]) ScanValue() ValueScanner {
	return new(sql.NullString)
}

// FromValue implements TypeValueScanner.FromValue.
func (BinaryValueScanner[T]) FromValue(value driver.Value) (ret T, err error) {
	scanner, ok := value.(*sql.NullString)
	if !ok {
		return ret, fmt.Errorf("unexpected input for FromValue: %T", value)
	}
	ret = initialize(ret)
	if scanner.Valid {
		err = ret.UnmarshalBinary([]byte(scanner.String))
	}
	return ret, err
}

// ValueScannerFunc adapts conversion functions to TypeValueScanner.
type ValueScannerFunc[T any, S ValueScanner] struct {
	V func(T) (driver.Value, error)
	S func(S) (T, error)
}

// Value implements TypeValueScanner.Value.
func (f ValueScannerFunc[T, S]) Value(value T) (driver.Value, error) {
	return f.V(value)
}

// ScanValue implements TypeValueScanner.ScanValue.
func (f ValueScannerFunc[T, S]) ScanValue() ValueScanner {
	var scanner S
	return initialize(scanner)
}

// FromValue implements TypeValueScanner.FromValue.
func (f ValueScannerFunc[T, S]) FromValue(value driver.Value) (T, error) {
	scanner, ok := value.(S)
	if !ok {
		var zero T
		return zero, fmt.Errorf("unexpected input for FromValue: %T", value)
	}
	return f.S(scanner)
}

func initialize[T any](value T) T {
	typ := reflect.TypeFor[T]()
	if typ.Kind() != reflect.Pointer {
		return value
	}
	return reflect.New(typ.Elem()).Interface().(T)
}

// Validator wraps the Validate method recognized on represented field types.
type Validator interface {
	Validate() error
}
