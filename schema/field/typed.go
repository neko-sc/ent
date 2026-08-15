// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field

import (
	"errors"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/neko-sc/ent/schema"
)

// String returns a string field.
func String(name string) *StringBuilder[string] {
	return StringAs[string](name)
}

// Text returns an unlimited-size string field.
func Text(name string) *StringBuilder[string] {
	return TextAs[string](name)
}

// Bool returns a boolean field.
func Bool(name string) *BoolBuilder[bool] {
	return BoolAs[bool](name)
}

// Bytes returns a byte-slice field.
func Bytes(name string) *BytesBuilder[[]byte] {
	return BytesAs[[]byte](name)
}

// Time returns a time.Time field.
func Time(name string) *TimeBuilder[time.Time] {
	return TimeAs[time.Time](name)
}

// Enum returns a string-backed enum field.
func Enum(name string) *EnumBuilder[string] {
	return EnumAs[string](name)
}

// Strings returns a JSON field represented by []string.
func Strings(name string) *JSONBuilder[[]string] {
	return JSON[[]string](name)
}

// Ints returns a JSON field represented by []int.
func Ints(name string) *JSONBuilder[[]int] {
	return JSON[[]int](name)
}

// Floats returns a JSON field represented by []float64.
func Floats(name string) *JSONBuilder[[]float64] {
	return JSON[[]float64](name)
}

// Any returns a JSON field represented by any.
func Any(name string) *JSONBuilder[any] {
	return JSON[any](name)
}

// StringAs returns a string field represented by T.
func StringAs[T any](name string) *StringBuilder[T] {
	return newStringBuilder[T](name, 0)
}

// TextAs returns an unlimited-size string field represented by T.
func TextAs[T any](name string) *StringBuilder[T] {
	return newStringBuilder[T](name, math.MaxInt32)
}

// BoolAs returns a boolean field represented by T.
func BoolAs[T any](name string) *BoolBuilder[T] {
	builder := &BoolBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeBool, builder)
	return builder
}

// BytesAs returns a bytes field represented by T.
func BytesAs[T any](name string) *BytesBuilder[T] {
	builder := &BytesBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeBytes, builder)
	return builder
}

// TimeAs returns a time field represented by T.
func TimeAs[T any](name string) *TimeBuilder[T] {
	builder := &TimeBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeTime, builder)
	return builder
}

// UUID returns a UUID field represented by T.
func UUID[T any](name string) *UUIDBuilder[T] {
	builder := &UUIDBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeUUID, builder)
	return builder
}

// EnumAs returns an enum field represented by T.
func EnumAs[T any](name string) *EnumBuilder[T] {
	builder := &EnumBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeEnum, builder)
	return builder
}

// JSON returns a JSON field represented by T.
func JSON[T any](name string) *JSONBuilder[T] {
	builder := &JSONBuilder[T]{}
	builder.typedFieldBuilder = newTypedBuilder[T](name, TypeJSON, builder)
	return builder
}

// Other returns an otherwise unsupported field represented by T.
func Other[T any](name string) *OtherBuilder[T] {
	builder := &OtherBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeOther, builder)
	return builder
}

// EnumValues supplies the values of an enum representation.
type EnumValues interface {
	Values() []string
}

type typedFieldBuilder[T any, B any] struct {
	desc *Descriptor
	self B
}

func newTypedBuilder[T any, B any](name string, typ Type, self B) *typedFieldBuilder[T, B] {
	return &typedFieldBuilder[T, B]{
		desc: &Descriptor{
			Name:        name,
			Type:        typ,
			RuntimeType: reflect.TypeFor[T](),
		},
		self: self,
	}
}

// Default sets the default value used on creation.
func (b *typedFieldBuilder[T, B]) Default(value T) B {
	b.desc.Default = value
	return b.self
}

// DefaultFunc sets the function used to generate a default value on creation.
func (b *typedFieldBuilder[T, B]) DefaultFunc(fn func() T) B {
	b.desc.Default = fn
	return b.self
}

// UpdateDefault sets the function used to generate a default value on update.
func (b *typedFieldBuilder[T, B]) UpdateDefault(fn func() T) B {
	b.desc.UpdateDefault = fn
	return b.self
}

// Validate adds a validator for the represented Go value.
func (b *typedFieldBuilder[T, B]) Validate(fn func(T) error) B {
	b.desc.Validators = append(b.desc.Validators, fn)
	b.desc.ValidatorKinds = append(b.desc.ValidatorKinds, ValidatorRepresentation)
	return b.self
}

func (b *typedFieldBuilder[T, B]) addLogicalValidator(validator any) {
	b.desc.Validators = append(b.desc.Validators, validator)
	b.desc.ValidatorKinds = append(b.desc.ValidatorKinds, ValidatorLogical)
}

// Nillable makes the generated struct field a pointer.
func (b *typedFieldBuilder[T, B]) Nillable() B {
	b.desc.Nillable = true
	return b.self
}

// Optional makes the field optional on creation.
func (b *typedFieldBuilder[T, B]) Optional() B {
	b.desc.Optional = true
	return b.self
}

// Immutable prevents the field from being updated.
func (b *typedFieldBuilder[T, B]) Immutable() B {
	b.desc.Immutable = true
	return b.self
}

// Comment sets the field comment.
func (b *typedFieldBuilder[T, B]) Comment(comment string) B {
	b.desc.Comment = comment
	return b.self
}

// StructTag sets the struct tag.
func (b *typedFieldBuilder[T, B]) StructTag(tag string) B {
	b.desc.Tag = tag
	return b.self
}

// StorageKey sets the storage column name.
func (b *typedFieldBuilder[T, B]) StorageKey(key string) B {
	b.desc.StorageKey = key
	return b.self
}

// SchemaType overrides the database type by dialect.
func (b *typedFieldBuilder[T, B]) SchemaType(types map[string]string) B {
	b.desc.SchemaType = types
	return b.self
}

// Annotations adds code-generation annotations.
func (b *typedFieldBuilder[T, B]) Annotations(annotations ...schema.Annotation) B {
	b.desc.Annotations = append(b.desc.Annotations, annotations...)
	return b.self
}

// Deprecated marks the field as deprecated.
func (b *typedFieldBuilder[T, B]) Deprecated(reason ...string) B {
	b.desc.Deprecated = true
	if len(reason) > 0 {
		b.desc.DeprecatedReason = strings.Join(reason, " ")
	}
	return b.self
}

// Descriptor implements ent.Field.
func (b *typedFieldBuilder[T, B]) Descriptor() *Descriptor {
	return b.desc
}

type codecBuilder[T any, B any] struct {
	*typedFieldBuilder[T, B]
}

func newCodecBuilder[T any, B any](name string, typ Type, self B) *codecBuilder[T, B] {
	return &codecBuilder[T, B]{typedFieldBuilder: newTypedBuilder[T](name, typ, self)}
}

// Codec attaches an external value scanner for the represented Go value.
func (b *codecBuilder[T, B]) Codec(codec TypeValueScanner[T]) B {
	b.desc.ValueScanner = codec
	return b.self
}

// StringBuilder builds a string-family field represented by T.
type StringBuilder[T any] struct {
	*codecBuilder[T, *StringBuilder[T]]
}

func newStringBuilder[T any](name string, size int) *StringBuilder[T] {
	builder := &StringBuilder[T]{}
	builder.codecBuilder = newCodecBuilder[T](name, TypeString, builder)
	builder.desc.Size = size
	return builder
}

// Unique makes the field unique.
func (b *StringBuilder[T]) Unique() *StringBuilder[T] {
	b.desc.Unique = true
	return b
}

// Sensitive prevents the field from being printed or serialized.
func (b *StringBuilder[T]) Sensitive() *StringBuilder[T] {
	b.desc.Sensitive = true
	return b
}

// Match adds a base string regular-expression validator.
func (b *StringBuilder[T]) Match(expression *regexp.Regexp) *StringBuilder[T] {
	b.addLogicalValidator(func(value string) error {
		if !expression.MatchString(value) {
			return errors.New("value does not match validation")
		}
		return nil
	})
	return b
}

// MinLen adds a base string byte-length validator.
func (b *StringBuilder[T]) MinLen(length int) *StringBuilder[T] {
	b.addLogicalValidator(func(value string) error {
		if len(value) < length {
			return errors.New("value is less than the required length")
		}
		return nil
	})
	return b
}

// MinRuneLen adds a base string rune-length validator.
func (b *StringBuilder[T]) MinRuneLen(length int) *StringBuilder[T] {
	b.addLogicalValidator(func(value string) error {
		if utf8.RuneCountInString(value) < length {
			return errors.New("value is less than the required rune length")
		}
		return nil
	})
	return b
}

// NotEmpty rejects empty base strings.
func (b *StringBuilder[T]) NotEmpty() *StringBuilder[T] {
	return b.MinLen(1)
}

// MaxLen adds a base string byte-length validator.
func (b *StringBuilder[T]) MaxLen(length int) *StringBuilder[T] {
	b.desc.Size = length
	b.addLogicalValidator(func(value string) error {
		if len(value) > length {
			return errors.New("value is greater than the required length")
		}
		return nil
	})
	return b
}

// MaxRuneLen adds a base string rune-length validator.
func (b *StringBuilder[T]) MaxRuneLen(length int) *StringBuilder[T] {
	b.desc.Size = length
	b.addLogicalValidator(func(value string) error {
		if utf8.RuneCountInString(value) > length {
			return errors.New("value is greater than the required rune length")
		}
		return nil
	})
	return b
}

// BoolBuilder builds a boolean field represented by T.
type BoolBuilder[T any] struct {
	*codecBuilder[T, *BoolBuilder[T]]
}

// TimeBuilder builds a time field represented by T.
type TimeBuilder[T any] struct {
	*codecBuilder[T, *TimeBuilder[T]]
}

// Unique makes the field unique.
func (b *TimeBuilder[T]) Unique() *TimeBuilder[T] {
	b.desc.Unique = true
	return b
}

// BytesBuilder builds a bytes field represented by T.
type BytesBuilder[T any] struct {
	*codecBuilder[T, *BytesBuilder[T]]
}

// Unique makes the field unique.
func (b *BytesBuilder[T]) Unique() *BytesBuilder[T] {
	b.desc.Unique = true
	return b
}

// Sensitive prevents the field from being printed or serialized.
func (b *BytesBuilder[T]) Sensitive() *BytesBuilder[T] {
	b.desc.Sensitive = true
	return b
}

// MinLen adds a base byte-slice length validator.
func (b *BytesBuilder[T]) MinLen(length int) *BytesBuilder[T] {
	b.addLogicalValidator(func(value []byte) error {
		if len(value) < length {
			return errors.New("value is less than the required length")
		}
		return nil
	})
	return b
}

// MaxLen adds a base byte-slice length validator.
func (b *BytesBuilder[T]) MaxLen(length int) *BytesBuilder[T] {
	b.desc.Size = length
	b.addLogicalValidator(func(value []byte) error {
		if len(value) > length {
			return errors.New("value is greater than the required length")
		}
		return nil
	})
	return b
}

// NotEmpty rejects empty base byte slices.
func (b *BytesBuilder[T]) NotEmpty() *BytesBuilder[T] {
	return b.MinLen(1)
}

// JSONBuilder builds a JSON field represented by T.
type JSONBuilder[T any] struct {
	*typedFieldBuilder[T, *JSONBuilder[T]]
}

// Sensitive prevents the field from being printed or serialized.
func (b *JSONBuilder[T]) Sensitive() *JSONBuilder[T] {
	b.desc.Sensitive = true
	return b
}

// EnumBuilder builds an enum field represented by T.
type EnumBuilder[T any] struct {
	*codecBuilder[T, *EnumBuilder[T]]
}

// Values adds enum values whose Go names and database values are equal.
func (b *EnumBuilder[T]) Values(values ...string) *EnumBuilder[T] {
	for _, value := range values {
		b.desc.Enums = append(b.desc.Enums, struct{ N, V string }{N: value, V: value})
	}
	return b
}

// ValuesFrom adds enum values supplied by T.
func (b *EnumBuilder[T]) ValuesFrom(values EnumValues) *EnumBuilder[T] {
	return b.Values(values.Values()...)
}

// NamedValues adds alternating Go name and database value pairs.
func (b *EnumBuilder[T]) NamedValues(nameValues ...string) *EnumBuilder[T] {
	if len(nameValues)%2 == 1 {
		b.desc.Err = errors.New("Enum.NamedValues: odd argument count")
		return b
	}
	for index := 0; index < len(nameValues); index += 2 {
		b.desc.Enums = append(b.desc.Enums, struct{ N, V string }{N: nameValues[index], V: nameValues[index+1]})
	}
	return b
}

// UUIDBuilder builds a UUID field represented by T.
type UUIDBuilder[T any] struct {
	*codecBuilder[T, *UUIDBuilder[T]]
}

// Unique makes the field unique.
func (b *UUIDBuilder[T]) Unique() *UUIDBuilder[T] {
	b.desc.Unique = true
	return b
}

// OtherBuilder builds an otherwise unsupported field represented by T.
type OtherBuilder[T any] struct {
	*codecBuilder[T, *OtherBuilder[T]]
}

// Unique makes the field unique.
func (b *OtherBuilder[T]) Unique() *OtherBuilder[T] {
	b.desc.Unique = true
	return b
}

// Sensitive prevents the field from being printed or serialized.
func (b *OtherBuilder[T]) Sensitive() *OtherBuilder[T] {
	b.desc.Sensitive = true
	return b
}

// Descriptor implements ent.Field and checks Other's storage requirement.
func (b *OtherBuilder[T]) Descriptor() *Descriptor {
	if len(b.desc.SchemaType) == 0 {
		b.desc.Err = errors.New("expect SchemaType to be set for other field")
	}
	return b.desc
}

type numeric interface {
	~int | ~uint | ~int8 | ~int16 | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float32 | ~float64
}

type numericBuilder[T any, N numeric, B any] struct {
	*codecBuilder[T, B]
}

func newNumericBuilder[T any, N numeric, B any](name string, typ Type, self B) *numericBuilder[T, N, B] {
	return &numericBuilder[T, N, B]{codecBuilder: newCodecBuilder[T](name, typ, self)}
}

// Unique makes the field unique.
func (b *numericBuilder[T, N, B]) Unique() B {
	b.desc.Unique = true
	return b.self
}

// Range adds a base numeric inclusive-range validator.
func (b *numericBuilder[T, N, B]) Range(minimum, maximum N) B {
	b.addLogicalValidator(func(value N) error {
		if value < minimum || value > maximum {
			return errors.New("value out of range")
		}
		return nil
	})
	return b.self
}

// Min adds a base numeric minimum validator.
func (b *numericBuilder[T, N, B]) Min(minimum N) B {
	b.addLogicalValidator(func(value N) error {
		if value < minimum {
			return errors.New("value out of range")
		}
		return nil
	})
	return b.self
}

// Max adds a base numeric maximum validator.
func (b *numericBuilder[T, N, B]) Max(maximum N) B {
	b.addLogicalValidator(func(value N) error {
		if value > maximum {
			return errors.New("value out of range")
		}
		return nil
	})
	return b.self
}
