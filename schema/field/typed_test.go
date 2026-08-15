// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type typedName string

type typedCount int

type typedPayload struct {
	Value string
}

type typedBox[T any] struct {
	Value T
}

type typedPhantom[A, B any] struct {
	Value A
}

type typedCodec[T any] struct {
	from string
}

func (typedCodec[T]) Value(T) (driver.Value, error) {
	return "encoded", nil
}

func (typedCodec[T]) ScanValue() field.ValueScanner {
	return &sql.NullString{}
}

func (codec typedCodec[T]) FromValue(driver.Value) (T, error) {
	var value T
	return value, errors.New(codec.from)
}

func TestTypedConstructors_RuntimeType(t *testing.T) {
	tests := []struct {
		name    string
		field   ent.Field
		typ     reflect.Type
		logical field.Type
	}{
		{name: "string", field: field.StringAs[typedName]("value"), typ: reflect.TypeFor[typedName](), logical: field.TypeString},
		{name: "text", field: field.TextAs[typedName]("value"), typ: reflect.TypeFor[typedName](), logical: field.TypeString},
		{name: "bool", field: field.BoolAs[bool]("value"), typ: reflect.TypeFor[bool](), logical: field.TypeBool},
		{name: "bytes", field: field.BytesAs[[]byte]("value"), typ: reflect.TypeFor[[]byte](), logical: field.TypeBytes},
		{name: "time", field: field.TimeAs[typedPayload]("value"), typ: reflect.TypeFor[typedPayload](), logical: field.TypeTime},
		{name: "uuid", field: field.UUID[[16]byte]("value"), typ: reflect.TypeFor[[16]byte](), logical: field.TypeUUID},
		{name: "enum", field: field.EnumAs[typedName]("value"), typ: reflect.TypeFor[typedName](), logical: field.TypeEnum},
		{name: "json", field: field.JSON[typedPayload]("value"), typ: reflect.TypeFor[typedPayload](), logical: field.TypeJSON},
		{name: "other", field: field.Other[typedPayload]("value").SchemaType(map[string]string{dialect.SQLite: "text"}), typ: reflect.TypeFor[typedPayload](), logical: field.TypeOther},
		{name: "int", field: field.IntAs[typedCount]("value"), typ: reflect.TypeFor[typedCount](), logical: field.TypeInt},
		{name: "uint64", field: field.Uint64As[uint64]("value"), typ: reflect.TypeFor[uint64](), logical: field.TypeUint64},
		{name: "float32", field: field.Float32As[float32]("value"), typ: reflect.TypeFor[float32](), logical: field.TypeFloat32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := tt.field.Descriptor()
			assert.Same(t, tt.typ, descriptor.RuntimeType)
			assert.Equal(t, tt.logical, descriptor.Type)
		})
	}
}

func TestTypedConstructors_NestedGenericRuntimeTypeIdentity(t *testing.T) {
	typ := reflect.TypeFor[typedBox[typedPhantom[string, typedBox[int]]]]()
	descriptor := field.JSON[typedBox[typedPhantom[string, typedBox[int]]]]("nested").Descriptor()
	require.Same(t, typ, descriptor.RuntimeType)
	assert.NotSame(t, reflect.TypeFor[typedBox[typedPhantom[string, typedBox[bool]]]](), descriptor.RuntimeType)
}

func TestTypedBuilder_CodecMethodAvailability(t *testing.T) {
	tests := []struct {
		name     string
		builder  any
		hasCodec bool
	}{
		{name: "string", builder: field.String("value"), hasCodec: true},
		{name: "bool", builder: field.Bool("value"), hasCodec: true},
		{name: "bytes", builder: field.Bytes("value"), hasCodec: true},
		{name: "time", builder: field.Time("value"), hasCodec: true},
		{name: "uuid", builder: field.UUID[[16]byte]("value"), hasCodec: true},
		{name: "enum", builder: field.Enum("value"), hasCodec: true},
		{name: "other", builder: field.Other[typedPayload]("value"), hasCodec: true},
		{name: "numeric", builder: field.Int("value"), hasCodec: true},
		{name: "json", builder: field.JSON[typedPayload]("value"), hasCodec: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := reflect.TypeOf(tt.builder).MethodByName("Codec")
			assert.Equal(t, tt.hasCodec, ok)
		})
	}
}

func TestTypedBuilder_ValueMethods(t *testing.T) {
	defaultFunc := func() typedName { return "generated" }
	updateDefault := func() typedName { return "updated" }
	validator := func(value typedName) error {
		if value == "invalid" {
			return errors.New("invalid")
		}
		return nil
	}
	codec := typedCodec[typedName]{from: "decoded"}

	descriptor := field.StringAs[typedName]("name").
		Default("default").
		DefaultFunc(defaultFunc).
		UpdateDefault(updateDefault).
		Validate(validator).
		Codec(codec).
		Descriptor()

	assert.Equal(t, reflect.ValueOf(defaultFunc).Pointer(), reflect.ValueOf(descriptor.Default).Pointer())
	assert.Equal(t, reflect.ValueOf(updateDefault).Pointer(), reflect.ValueOf(descriptor.UpdateDefault).Pointer())
	require.Len(t, descriptor.Validators, 1)
	assert.NoError(t, descriptor.Validators[0].(func(typedName) error)("valid"))
	assert.EqualError(t, descriptor.Validators[0].(func(typedName) error)("invalid"), "invalid")
	assert.Equal(t, codec, descriptor.ValueScanner)
}

func TestTypedBuilder_StaticDefault(t *testing.T) {
	descriptor := field.JSON[typedPayload]("payload").Default(typedPayload{Value: "default"}).Descriptor()
	assert.Equal(t, typedPayload{Value: "default"}, descriptor.Default)
}

func TestTypedBuilder_ValidatorKindsPreserveFluentOrder(t *testing.T) {
	descriptor := field.StringAs[typedName]("name").
		Validate(func(typedName) error { return nil }).
		MinLen(3).
		Validate(func(typedName) error { return nil }).
		MaxLen(8).
		Descriptor()
	require.Len(t, descriptor.Validators, 4)
	assert.Equal(t, []field.ValidatorKind{
		field.ValidatorRepresentation,
		field.ValidatorLogical,
		field.ValidatorRepresentation,
		field.ValidatorLogical,
	}, descriptor.ValidatorKinds)
	assert.NoError(t, descriptor.Validators[0].(func(typedName) error)("ab"))
	assert.EqualError(t, descriptor.Validators[1].(func(string) error)("ab"), "value is less than the required length")
	assert.NoError(t, descriptor.Validators[2].(func(typedName) error)("ab"))
	assert.EqualError(t, descriptor.Validators[3].(func(string) error)("too long value"), "value is greater than the required length")

	descriptor = field.IntAs[typedCount]("count").Min(2).Validate(func(typedCount) error { return nil }).Descriptor()
	require.Len(t, descriptor.Validators, 2)
	assert.Equal(t, []field.ValidatorKind{field.ValidatorLogical, field.ValidatorRepresentation}, descriptor.ValidatorKinds)
	assert.EqualError(t, descriptor.Validators[0].(func(int) error)(1), "value out of range")
	assert.NoError(t, descriptor.Validators[1].(func(typedCount) error)(1))
}

func TestTypedBuilder_FluentReturnTypes(t *testing.T) {
	var stringBuilder = field.StringAs[typedName]("name").Optional().Unique().Default("name")
	var bytesBuilder = field.BytesAs[[]byte]("bytes").MaxLen(32).Sensitive()
	var enumBuilder = field.EnumAs[typedName]("state").Values("on", "off").Default("on")
	var numericBuilder = field.IntAs[typedCount]("count").Positive().Default(1)
	var jsonBuilder = field.JSON[typedPayload]("payload").Optional().Default(typedPayload{})
	var otherBuilder = field.Other[typedPayload]("other").SchemaType(map[string]string{dialect.SQLite: "text"}).Codec(typedCodec[typedPayload]{})

	assert.NotNil(t, stringBuilder)
	assert.NotNil(t, bytesBuilder)
	assert.NotNil(t, enumBuilder)
	assert.NotNil(t, numericBuilder)
	assert.NotNil(t, jsonBuilder)
	assert.NotNil(t, otherBuilder)
}
