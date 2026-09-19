// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"database/sql/driver"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"time"

	sef "github.com/neko-sc/sef/go"
)

type entryWire struct {
	Columns        []string   `sef:"0,key=uint8"`
	IDs            [][]byte   `sef:"1,key=uint8"`
	Rows           [][][]byte `sef:"2,key=uint8"`
	ValidationTags []string   `sef:"3,key=uint8"`
	FillStart      uint64     `sef:"4,key=uint8"`
	ExpiresAt      uint64     `sef:"5,key=uint8"`
	StaleUntil     uint64     `sef:"6,key=uint8"`
}

func encodeValue(value any) ([]byte, error) {
	if value == nil {
		// A complete SEF value is never empty, leaving an unambiguous NULL sentinel.
		return nil, nil
	}
	return sef.Marshal(value)
}

func decodeValueInto(data []byte, destination any) error {
	pointer := reflect.ValueOf(destination)
	if !pointer.IsValid() || pointer.Kind() != reflect.Pointer || pointer.IsNil() {
		return fmt.Errorf("cache: decode destination must be a non-nil pointer")
	}
	if len(data) == 0 {
		pointer.Elem().SetZero()
		return nil
	}
	return sef.Unmarshal(data, destination)
}

const (
	keyNil byte = iota
	keyInt
	keyUint
	keyFloat
	keyBool
	keyString
	keyBytes
	keyTime
	keySEF
)

func encodeKeyValue(destination []byte, value any) ([]byte, error) {
	if value != nil && reflect.ValueOf(value).Kind() == reflect.Pointer && reflect.ValueOf(value).IsNil() {
		return append(destination, keyNil), nil
	}
	if valuer, ok := value.(driver.Valuer); ok {
		var err error
		value, err = valuer.Value()
		if err != nil {
			return nil, err
		}
	}
	if value == nil {
		return append(destination, keyNil), nil
	}
	if timestamp, ok := value.(time.Time); ok {
		//nolint:gosec // Preserve the signed seconds as their two's-complement bits.
		destination = binary.LittleEndian.AppendUint64(append(destination, keyTime), uint64(timestamp.Unix()))
		//nolint:gosec // Nanosecond is bounded to [0, 999999999].
		return binary.LittleEndian.AppendUint32(destination, uint32(timestamp.Nanosecond())), nil
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		//nolint:gosec // Preserve signed integers as their two's-complement bits.
		return binary.LittleEndian.AppendUint64(append(destination, keyInt), uint64(reflected.Int())), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return binary.LittleEndian.AppendUint64(append(destination, keyUint), reflected.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return binary.LittleEndian.AppendUint64(append(destination, keyFloat), math.Float64bits(reflected.Float())), nil
	case reflect.Bool:
		if reflected.Bool() {
			return append(destination, keyBool, 1), nil
		}
		return append(destination, keyBool, 0), nil
	case reflect.String:
		destination = binary.LittleEndian.AppendUint64(append(destination, keyString), uint64(len(reflected.String())))
		return append(destination, reflected.String()...), nil
	case reflect.Slice:
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			destination = binary.LittleEndian.AppendUint64(append(destination, keyBytes), uint64(len(reflected.Bytes())))
			return append(destination, reflected.Bytes()...), nil
		}
	}
	encoded, err := sef.Marshal(value)
	if err != nil {
		return nil, err
	}
	return append(binary.LittleEndian.AppendUint64(append(destination, keySEF), uint64(len(encoded))), encoded...), nil
}
