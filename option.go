package ent

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RelationPatch describes additions, removals, and clearing of a relation.
type RelationPatch[K comparable] struct {
	Add    []K  `json:"add,omitzero"`
	Remove []K  `json:"remove,omitzero"`
	Clear  bool `json:"clear,omitzero"`
}

// IsZero reports whether the relation is unchanged.
func (p RelationPatch[K]) IsZero() bool {
	return len(p.Add) == 0 && len(p.Remove) == 0 && !p.Clear
}

type optionState uint8

const (
	optionUnset optionState = iota
	optionValue
	optionNull
)

// Option distinguishes an omitted value, a present value, and an explicit NULL.
// Its zero value is unset. Use json:",omitzero" to omit unset struct fields.
type Option[T any] struct {
	state optionState
	value T
}

// Some wraps a present value, including the zero value of T.
func Some[T any](value T) Option[T] { return Option[T]{state: optionValue, value: value} }

func Null[T any]() Option[T] { return Option[T]{state: optionNull} }

func Unset[T any]() Option[T] { return Option[T]{} }

// FromPtr copies a pointed-to value; nil means unset.
func FromPtr[T any](pointer *T) Option[T] {
	if pointer == nil {
		return Unset[T]()
	}
	return Some(*pointer)
}

// NullIfNil copies a pointed-to value; nil means explicit null.
func NullIfNil[T any](pointer *T) Option[T] {
	if pointer == nil {
		return Null[T]()
	}
	return Some(*pointer)
}

// IsSet reports whether the option contains a value or explicit null.
func (o Option[T]) IsSet() bool { return o.state != optionUnset }

// IsNull reports whether the option is explicitly null.
func (o Option[T]) IsNull() bool { return o.state == optionNull }

// IsUnset reports whether the option was omitted.
func (o Option[T]) IsUnset() bool { return o.state == optionUnset }

// Get returns the value and whether it is present and non-null.
func (o Option[T]) Get() (T, bool) { return o.value, o.state == optionValue }

// MustGet returns the value or panics if the option is unset or null.
func (o Option[T]) MustGet() T {
	if o.state != optionValue {
		panic("ent: Option has no value")
	}
	return o.value
}

// Ptr returns a pointer to a copy of the value, or nil for unset and null.
func (o Option[T]) Ptr() *T {
	if o.state != optionValue {
		return nil
	}
	return &o.value
}

// Or returns the value, or the fallback for unset and null.
func (o Option[T]) Or(fallback T) T {
	if o.state != optionValue {
		return fallback
	}
	return o.value
}

// IsZero permits encoding/json to omit only unset options with omitzero.
func (o Option[T]) IsZero() bool { return o.IsUnset() }

// MarshalJSON encodes unset and null as null, and a present value as itself.
func (o Option[T]) MarshalJSON() ([]byte, error) {
	if o.state != optionValue {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON records null or a present value. An absent struct field is untouched.
func (o *Option[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		*o = Null[T]()
		return nil
	}
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*o = Some(value)
	return nil
}

// String formats an option's state and value for debugging.
func (o Option[T]) String() string {
	switch o.state {
	case optionValue:
		return fmt.Sprintf("Some(%v)", o.value)
	case optionNull:
		return "Null"
	default:
		return "Unset"
	}
}
