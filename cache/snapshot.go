// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
)

type recorder struct {
	dialect.Rows
	entry     *Entry
	limits    Limits
	idIndex   int
	cacheable bool
	pending   bool
	exhausted bool
	closed    bool
}

func newRecorder(rows dialect.Rows, idColumn string, limits Limits) *recorder {
	if limits.MaxRows <= 0 {
		limits.MaxRows = DefaultMaxRows
	}
	if limits.MaxBytes <= 0 {
		limits.MaxBytes = DefaultMaxBytes
	}

	recorder := &recorder{
		Rows:      rows,
		limits:    limits,
		entry:     &Entry{},
		idIndex:   -1,
		cacheable: true,
	}

	columns, err := rows.Columns()
	if err != nil {
		recorder.cacheable = false
		return recorder
	}

	recorder.entry.columns = slices.Clone(columns)
	for index, column := range columns {
		recorder.entry.estimatedBytes += int64(len(column))
		if idColumn != "" && column == idColumn {
			recorder.idIndex = index
		}
	}
	recorder.cacheable = recorder.entry.estimatedBytes <= limits.MaxBytes
	return recorder
}

func (r *recorder) Next() bool {
	if r.closed || r.exhausted {
		return false
	}
	if r.pending {
		r.cacheable = false
	}
	r.pending = r.Rows.Next()
	r.exhausted = !r.pending
	return r.pending
}

func (r *recorder) Scan(destinations ...any) error {
	if err := r.Rows.Scan(destinations...); err != nil {
		r.cacheable = false
		return err
	}
	if !r.pending || !r.cacheable {
		return nil
	}
	r.pending = false
	if len(destinations) != len(r.entry.columns) || len(r.entry.rows) >= r.limits.MaxRows {
		r.cacheable = false
		return nil
	}

	row := make([]any, len(destinations))
	for index, destination := range destinations {
		value, err := snapshotCell(destination)
		if err != nil {
			r.cacheable = false
			return nil
		}
		row[index] = value
		r.entry.estimatedBytes += estimateValue(reflect.ValueOf(value))
		if r.entry.estimatedBytes > r.limits.MaxBytes {
			r.cacheable = false
			return nil
		}
	}
	r.entry.rows = append(r.entry.rows, row)
	if r.idIndex >= 0 {
		r.entry.ids = append(r.entry.ids, row[r.idIndex])
	}
	return nil
}

func (r *recorder) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if err := r.Rows.Close(); err != nil {
		r.cacheable = false
		return err
	}
	r.cacheable = r.cacheable && r.exhausted && !r.pending && r.Err() == nil
	return nil
}

func (r *recorder) Entry() (*Entry, bool) {
	if !r.closed || !r.cacheable {
		return nil, false
	}
	return r.entry, true
}

func (r *recorder) ColumnTypes() ([]*sql.ColumnType, error) {
	if typed, ok := r.Rows.(interface {
		ColumnTypes() ([]*sql.ColumnType, error)
	}); ok {
		return typed.ColumnTypes()
	}
	return nil, fmt.Errorf("cache: column types unavailable")
}

// snapshotCell normalizes a scan destination into a plain value before either cache tier retains it.
// Unlike snapshot, it strips scanner state and NULL wrappers so replays do not depend on scanner internals.
func snapshotCell(destination any) (any, error) {
	value := reflect.ValueOf(destination)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return nil, fmt.Errorf("cache: scan destination must be a non-nil pointer")
	}
	for depth := 0; depth <= 32; depth++ {
		if !value.IsValid() || ((value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil()) {
			return nil, nil
		}
		if scanner, ok := value.Interface().(*entsql.NullScanner); ok {
			if !scanner.Valid {
				return nil, nil
			}
			value = reflect.ValueOf(scanner.S)
			continue
		}
		if valuer, ok := value.Interface().(driver.Valuer); ok {
			normalized, err := valuer.Value()
			if err != nil {
				return nil, err
			}
			if !driver.IsValue(normalized) {
				return nil, fmt.Errorf("cache: unsupported driver value %T", normalized)
			}
			value = reflect.ValueOf(normalized)
			continue
		}
		if value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
			value = value.Elem()
			continue
		}
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			if value.IsNil() {
				return nil, nil
			}
			return slices.Clone(value.Bytes()), nil
		}
		if reflect.PointerTo(value.Type()).Implements(reflect.TypeFor[sql.Scanner]()) {
			return nil, fmt.Errorf("cache: cannot normalize scanner type %s", value.Type())
		}
		copied, err := copyValue(value, 0)
		if err != nil {
			return nil, err
		}
		return copied.Interface(), nil
	}
	return nil, fmt.Errorf("cache: snapshot nesting exceeds 32")
}

func snapshot(destination any) (any, error) {
	pointer := reflect.ValueOf(destination)
	if !pointer.IsValid() || pointer.Kind() != reflect.Pointer || pointer.IsNil() {
		return nil, fmt.Errorf("cache: scan destination must be a non-nil pointer")
	}

	value := pointer.Elem()
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, nil
		}
		value = value.Elem()
	} else if valuer, ok := destination.(driver.Valuer); ok {
		normalized, err := valuer.Value()
		if err != nil {
			return nil, err
		}
		value = reflect.ValueOf(normalized)
	}
	if value.IsValid() && value.Kind() == reflect.Interface {
		value = value.Elem()
	}
	if !value.IsValid() {
		return nil, nil
	}
	copied, err := copyValue(value, 0)
	if err != nil {
		return nil, err
	}
	return copied.Interface(), nil
}

// copyValue bounds recursion so cyclic or unsupported values bypass caching instead
// of retaining references owned by the database driver or the caller.
func copyValue(value reflect.Value, depth int) (reflect.Value, error) {
	if depth > 32 {
		return reflect.Value{}, fmt.Errorf("cache: snapshot nesting exceeds 32")
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		return value, nil
	}
	switch value.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64, reflect.String:
		return value, nil
	case reflect.Pointer, reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		copied, err := copyValue(value.Elem(), depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		result := reflect.New(value.Type()).Elem()
		if value.Kind() == reflect.Pointer {
			result.Set(reflect.New(value.Type().Elem()))
			result.Elem().Set(copied)
		} else {
			result.Set(copied)
		}
		return result, nil
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		result := reflect.New(value.Type()).Elem()
		if value.Kind() == reflect.Slice {
			result.Set(reflect.MakeSlice(value.Type(), value.Len(), value.Len()))
			if value.Type().Elem().Kind() == reflect.Uint8 {
				reflect.Copy(result, value)
				return result, nil
			}
		}
		for index := range value.Len() {
			copied, err := copyValue(value.Index(index), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(copied)
		}
		return result, nil
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String {
			break
		}
		if value.IsNil() {
			return reflect.Zero(value.Type()), nil
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		for iterator := value.MapRange(); iterator.Next(); {
			copied, err := copyValue(iterator.Value(), depth+1)
			if err != nil {
				return reflect.Value{}, err
			}
			result.SetMapIndex(iterator.Key(), copied)
		}
		return result, nil
	case reflect.Struct:
		return copyStruct(value, depth)
	}
	return reflect.Value{}, fmt.Errorf("cache: unsupported snapshot type %s", value.Type())
}

func copyStruct(value reflect.Value, depth int) (reflect.Value, error) {
	result := reflect.New(value.Type()).Elem()
	for field, fieldValue := range value.Fields() {
		if !field.IsExported() {
			return reflect.Value{}, fmt.Errorf("cache: cannot snapshot unexported field in %s", value.Type())
		}
		copied, err := copyValue(fieldValue, depth+1)
		if err != nil {
			return reflect.Value{}, err
		}
		result.FieldByIndex(field.Index).Set(copied)
	}
	return result, nil
}

func estimateValue(value reflect.Value) int64 {
	if !value.IsValid() {
		return 1
	}
	if value.Type() == reflect.TypeFor[time.Time]() {
		return 24
	}
	switch value.Kind() {
	case reflect.String:
		return int64(value.Len())
	case reflect.Pointer, reflect.Interface:
		return estimateValue(value.Elem())
	case reflect.Slice, reflect.Array:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return int64(value.Len())
		}
		var size int64
		for index := range value.Len() {
			size += estimateValue(value.Index(index))
		}
		return size
	case reflect.Map:
		var size int64
		for iterator := value.MapRange(); iterator.Next(); {
			size += int64(iterator.Key().Len()) + estimateValue(iterator.Value())
		}
		return size
	case reflect.Struct:
		var size int64
		for _, fieldValue := range value.Fields() {
			size += estimateValue(fieldValue)
		}
		return size
	default:
		return 8
	}
}

type replayRows struct {
	entry  *Entry
	index  int
	closed bool
	err    error
}

// replay deliberately has no ColumnTypes method. sql.ScanTypeOf therefore uses
// *any, which must accept every cached cell rather than just driver.Value types.
func replay(entry *Entry) dialect.Rows {
	return &replayRows{entry: entry, index: -1}
}

func (r *replayRows) Next() bool {
	if r.closed || r.err != nil {
		return false
	}
	r.index++
	return r.index < r.entry.Len()
}

func (r *replayRows) Scan(destinations ...any) error {
	if r.closed || r.index < 0 || r.index >= r.entry.Len() {
		return fmt.Errorf("cache: Scan called without a current row")
	}
	if len(destinations) != len(r.entry.columns) {
		return fmt.Errorf("cache: expected %d columns, got %d", len(r.entry.columns), len(destinations))
	}
	if r.err = r.entry.decode(); r.err != nil {
		return r.err
	}

	for index, destination := range destinations {
		var encoded []byte
		if r.entry.encodedRows != nil {
			encoded = r.entry.encodedRows[r.index][index]
		}
		if err := assignValue(destination, r.entry.rows[r.index][index], encoded); err != nil {
			return fmt.Errorf("cache: column %d: %w", index, err)
		}
	}
	return nil
}

func (r *replayRows) Columns() ([]string, error) {
	return r.entry.Columns(), nil
}

func (r *replayRows) Close() error {
	r.closed = true
	return nil
}

func (r *replayRows) Err() error {
	return r.err
}

func assignValue(destination, value any, encoded []byte) error {
	pointer := reflect.ValueOf(destination)
	if !pointer.IsValid() || pointer.Kind() != reflect.Pointer || pointer.IsNil() {
		return fmt.Errorf("destination must be a non-nil pointer")
	}

	nullable := pointer.Elem().Kind() == reflect.Pointer
	if nullable {
		if value == nil {
			pointer.Elem().SetZero()
			return nil
		}
		pointer.Elem().Set(reflect.New(pointer.Elem().Type().Elem()))
		pointer = pointer.Elem()
		destination = pointer.Interface()
	}
	if value != nil {
		copied, err := copyValue(reflect.ValueOf(value), 0)
		if err != nil {
			if encoded != nil {
				return decodeValueInto(encoded, destination)
			}
			return err
		}
		if copied.Type().AssignableTo(pointer.Elem().Type()) {
			pointer.Elem().Set(copied)
			return nil
		}
		value = copied.Interface()
	}

	if value == nil || driver.IsValue(value) {
		err := sql.ConvertAssign(driver.ScanContext{}, destination, value)
		if err == nil || encoded == nil {
			return err
		}
	}
	if scanner, ok := destination.(sql.Scanner); ok && !nullable {
		return scanner.Scan(value)
	}
	if encoded != nil {
		return decodeValueInto(encoded, destination)
	}
	return fmt.Errorf("cannot assign %T to %T", value, destination)
}
