package sql

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/neko-sc/ent/dialect"
)

func arrayArguments(arguments []any) ([]any, error) {
	result := append([]any(nil), arguments...)
	for index, argument := range result {
		if _, ok := argument.(driver.Valuer); ok {
			continue
		}
		value := reflect.ValueOf(argument)
		for value.IsValid() && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				break
			}
			value = value.Elem()
		}
		if !value.IsValid() || value.Kind() != reflect.Slice || value.Type().Elem().Kind() == reflect.Uint8 {
			continue
		}
		if value.IsNil() {
			result[index] = nil
			continue
		}
		encoded, err := json.Marshal(argument)
		if err != nil {
			return nil, fmt.Errorf("encode array argument %d: %w", index, err)
		}
		result[index] = string(encoded)
	}
	return result, nil
}

func (r *Rows) Scan(destinations ...any) error {
	if r.dialect != dialect.SQLite {
		return mapError(r.ColumnScanner.Scan(destinations...))
	}
	converted := append([]any(nil), destinations...)
	for index, destination := range converted {
		if _, ok := destination.(sql.Scanner); ok {
			continue
		}
		typ := reflect.TypeOf(destination)
		if typ == nil || typ.Kind() != reflect.Pointer {
			continue
		}
		for typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.Kind() == reflect.Slice && typ.Elem().Kind() != reflect.Uint8 {
			converted[index] = arrayScanner{destination}
		}
	}
	return mapError(r.ColumnScanner.Scan(converted...))
}

type arrayScanner struct{ destination any }

func (scanner arrayScanner) Scan(source any) error {
	if source == nil {
		reflect.ValueOf(scanner.destination).Elem().SetZero()
		return nil
	}
	switch source := source.(type) {
	case string:
		return json.Unmarshal([]byte(source), scanner.destination)
	case []byte:
		return json.Unmarshal(source, scanner.destination)
	default:
		return fmt.Errorf("cannot decode array from %T", source)
	}
}
