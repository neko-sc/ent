// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"

	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

type fakeRows struct {
	columns          []string
	rows             [][]any
	index            int
	closed           bool
	err              error
	closeError       error
	scanError        error
	columnsError     error
	columnTypes      []*sql.ColumnType
	columnTypesError error
}

func (r *fakeRows) Next() bool {
	if r.closed || r.index >= len(r.rows) {
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(destinations ...any) error {
	if r.scanError != nil {
		return r.scanError
	}
	for i, destination := range destinations {
		if scanner, ok := destination.(sql.Scanner); ok {
			if err := scanner.Scan(r.rows[r.index-1][i]); err != nil {
				return err
			}
			continue
		}
		pointer := reflect.ValueOf(destination).Elem()
		if r.rows[r.index-1][i] == nil {
			pointer.SetZero()
			continue
		}
		if pointer.Kind() == reflect.Pointer {
			pointer.Set(reflect.New(pointer.Type().Elem()))
			pointer = pointer.Elem()
		}
		pointer.Set(reflect.ValueOf(r.rows[r.index-1][i]))
	}
	return nil
}

func (r *fakeRows) Columns() ([]string, error) {
	return r.columns, r.columnsError
}

func (r *fakeRows) ColumnTypes() ([]*sql.ColumnType, error) {
	return r.columnTypes, r.columnTypesError
}

func (r *fakeRows) Close() error {
	r.closed = true
	return r.closeError
}

func (r *fakeRows) Err() error {
	return r.err
}

type valueScanner struct {
	value driver.Value
	fail  bool
}

func (s *valueScanner) Scan(value any) error {
	s.value = value
	return nil
}

func (s *valueScanner) Value() (driver.Value, error) {
	if s.fail {
		return nil, errors.New("valuer failure")
	}
	return s.value, nil
}

// recordEntry produces an entry the way the driver does, by recording a complete
// result set. Tests never build entries from their unexported fields.
func recordEntry(t *testing.T, idColumn string, columns []string, rows [][]any, destinations ...any) *Entry {
	t.Helper()
	recorded := newRecorder(&fakeRows{columns: columns, rows: rows}, idColumn, Limits{})
	for range rows {
		require.True(t, recorded.Next())
		require.NoError(t, recorded.Scan(destinations...))
	}
	require.False(t, recorded.Next())
	require.NoError(t, recorded.Close())
	entry, complete := recorded.Entry()
	require.True(t, complete)
	return entry
}

func TestSnapshotReplay(t *testing.T) {
	timestamp := time.Date(2026, 9, 18, 1, 2, 3, 4, time.UTC)
	live := &fakeRows{
		columns: []string{"id", "nullable", "null", "bytes", "scanner", "dynamic", "unknown", "text", "time", "nullString", "slice", "structure"},
		rows:    [][]any{{int64(7), "nullable", nil, []byte("bytes"), "scan", []byte("dynamic"), int32(9), "plain", timestamp, "valid", []int{1, 2}, struct{ Name string }{"name"}}},
	}
	recorded := newRecorder(live, "id", Limits{})
	var id int64
	var nullable, null *string
	var bytes []byte
	var scanner valueScanner
	var dynamic any
	var unknown entsql.UnknownType
	var text string
	var instant time.Time
	var nullString sql.NullString
	var values []int
	var structure struct{ Name string }
	destinations := []any{&id, &nullable, &null, &bytes, &scanner, &dynamic, &unknown, &text, &instant, &nullString, &values, &structure}
	require.True(t, recorded.Next())
	require.NoError(t, recorded.Scan(destinations...))
	require.False(t, recorded.Next())
	require.NoError(t, recorded.Close())
	entry, ok := recorded.Entry()
	require.True(t, ok)
	require.Equal(t, 1, entry.Len())
	bytes[0] = 'X'
	dynamic.([]byte)[0] = 'X'
	values[0] = 99
	for range 2 {
		rows := replay(entry)
		require.IsType(t, new(any), entsql.ScanTypeOf(rows, 0))
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(destinations...))
		require.Equal(t, int64(7), id)
		require.Equal(t, "nullable", *nullable)
		require.Nil(t, null)
		require.Equal(t, []byte("bytes"), bytes)
		require.Equal(t, "scan", scanner.value)
		require.Equal(t, []byte("dynamic"), dynamic)
		require.Equal(t, int32(9), unknown)
		require.Equal(t, "plain", text)
		require.Equal(t, timestamp, instant)
		require.Equal(t, sql.NullString{String: "valid", Valid: true}, nullString)
		require.Equal(t, []int{1, 2}, values)
		require.Equal(t, "name", structure.Name)
		bytes[0] = 'X'
		dynamic.([]byte)[0] = 'X'
		values[0] = 99
		require.False(t, rows.Next())
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
	}
	columns := entry.Columns()
	columns[0] = "changed"
	require.Equal(t, "id", entry.Columns()[0])
}

func TestSnapshotUncacheable(t *testing.T) {
	for _, test := range []struct {
		name        string
		value       any
		destination any
		limits      Limits
	}{
		{name: "rows", value: "value", destination: new(string), limits: Limits{MaxRows: 1}},
		{name: "bytes", value: "value", destination: new(string), limits: Limits{MaxBytes: 3}},
		{name: "valuer", value: "value", destination: &valueScanner{fail: true}},
		{name: "channel", value: make(chan int), destination: new(chan int)},
		{name: "function", value: func() {}, destination: new(func())},
		{name: "map keys", value: map[int]string{1: "one"}, destination: new(map[int]string)},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := newRecorder(&fakeRows{columns: []string{"value"}, rows: [][]any{{test.value}, {test.value}}}, "", test.limits)
			for range 2 {
				require.True(t, rows.Next())
				require.NoError(t, rows.Scan(test.destination))
			}
			require.False(t, rows.Next())
			require.NoError(t, rows.Close())
			_, ok := rows.Entry()
			require.False(t, ok)
		})
	}
}

func TestRecorderRequiresCompleteResult(t *testing.T) {
	for _, test := range []struct {
		name    string
		rows    fakeRows
		scan    bool
		exhaust bool
	}{
		{name: "early close", scan: true},
		{name: "skipped scan", exhaust: true},
		{name: "iteration error", rows: fakeRows{err: errors.New("iteration")}, scan: true, exhaust: true},
		{name: "close error", rows: fakeRows{closeError: errors.New("close")}, scan: true, exhaust: true},
		{name: "scan error", rows: fakeRows{scanError: errors.New("scan")}, scan: true, exhaust: true},
		{name: "columns error", rows: fakeRows{columnsError: errors.New("columns")}, scan: true, exhaust: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.rows.columns, test.rows.rows = []string{"id"}, [][]any{{1}}
			rows := newRecorder(&test.rows, "id", Limits{})
			_, ok := rows.Entry()
			require.False(t, ok)
			require.True(t, rows.Next())
			if test.scan {
				require.ErrorIs(t, rows.Scan(new(int)), test.rows.scanError)
			}
			if test.exhaust {
				require.False(t, rows.Next())
			}
			require.ErrorIs(t, rows.Close(), test.rows.closeError)
			_, ok = rows.Entry()
			require.False(t, ok)
		})
	}
}

func TestRecorderColumnTypes(t *testing.T) {
	t.Run("recording", func(t *testing.T) {
		live := &fakeRows{columns: []string{"id"}, columnTypes: []*sql.ColumnType{new(sql.ColumnType)}}
		recorded := newRecorder(live, "id", Limits{})
		columnTypes, err := recorded.ColumnTypes()
		require.NoError(t, err)
		require.Len(t, columnTypes, 1)
		require.Same(t, live.columnTypes[0], columnTypes[0])

		live.columnTypes = nil
		live.columnTypesError = errors.New("column types failure")
		columnTypes, err = recorded.ColumnTypes()
		require.ErrorIs(t, err, live.columnTypesError)
		require.Nil(t, columnTypes)
		require.NoError(t, recorded.Close())
	})

	t.Run("replay", func(t *testing.T) {
		rows := replay(recordEntry(t, "id", []string{"id"}, [][]any{{1}}, new(int)))
		require.IsType(t, new(any), entsql.ScanTypeOf(rows, 0))
		recorded := newRecorder(rows, "id", Limits{})
		columnTypes, err := recorded.ColumnTypes()
		require.Error(t, err)
		require.Nil(t, columnTypes)
		require.True(t, recorded.Next())
		var id int
		require.NoError(t, recorded.Scan(&id))
		require.Equal(t, 1, id)
		require.False(t, recorded.Next())
		require.NoError(t, recorded.Close())
	})
}

func TestRecorderEmptyResult(t *testing.T) {
	rows := newRecorder(&fakeRows{columns: []string{"id"}}, "id", Limits{})
	require.False(t, rows.Next())
	require.NoError(t, rows.Close())
	entry, ok := rows.Entry()
	require.True(t, ok)
	require.Zero(t, entry.Len())
}

func TestRecorderRepeatedScan(t *testing.T) {
	rows := newRecorder(&fakeRows{columns: []string{"id"}, rows: [][]any{{1}}}, "id", Limits{})
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(new(int)))
	require.NoError(t, rows.Scan(new(int)))
	require.False(t, rows.Next())
	require.NoError(t, rows.Close())
	entry, ok := rows.Entry()
	require.True(t, ok)
	require.Equal(t, 1, entry.Len())
}

func TestSnapshotComplexDestinations(t *testing.T) {
	for _, original := range []any{
		sql.NullString{String: "value", Valid: true},
		[]byte("bytes"),
		[]string{"first", "second"},
		map[string][]byte{"key": []byte("bytes")},
		struct{ Values [][]byte }{Values: [][]byte{[]byte("bytes")}},
	} {
		t.Run(reflect.TypeOf(original).String(), func(t *testing.T) {
			value := reflect.New(reflect.TypeOf(original))
			value.Elem().Set(reflect.ValueOf(original))
			nullable := reflect.New(value.Type())
			nullable.Elem().Set(value)
			cell, err := snapshot(nullable.Interface())
			require.NoError(t, err)
			require.Equal(t, original, cell)
			destination := reflect.New(value.Type())
			require.NoError(t, assignValue(destination.Interface(), cell, nil))
			require.Equal(t, original, destination.Elem().Elem().Interface())
			require.NotEqual(t, value.Pointer(), destination.Elem().Pointer())
		})
	}
	cyclic := make(map[string]any)
	cyclic["self"] = cyclic
	_, err := snapshot(&cyclic)
	require.Error(t, err)
}

func TestReplayTypeConversion(t *testing.T) {
	entry := recordEntry(t, "", []string{"v"}, [][]any{{int64(7)}}, new(any))
	for _, test := range []struct {
		name        string
		destination any
		expected    any
	}{
		{"int64 to int", new(int), 7},
		{"int64 to int32", new(int32), int32(7)},
		{"int64 to float64", new(float64), float64(7)},
		{"int64 to string", new(string), "7"},
		{"int64 to any", new(any), int64(7)},
	} {
		t.Run(test.name, func(t *testing.T) {
			rows := replay(entry)
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(test.destination))
			require.Equal(t, test.expected, reflect.ValueOf(test.destination).Elem().Interface())
		})
	}
	byteEntry := recordEntry(t, "", []string{"v"}, [][]any{{[]byte("hello")}}, new(any))
	t.Run("bytes to string", func(t *testing.T) {
		rows := replay(byteEntry)
		require.True(t, rows.Next())
		var s string
		require.NoError(t, rows.Scan(&s))
		require.Equal(t, "hello", s)
	})
	stringEntry := recordEntry(t, "", []string{"v"}, [][]any{{"hello"}}, new(any))
	t.Run("string to bytes", func(t *testing.T) {
		rows := replay(stringEntry)
		require.True(t, rows.Next())
		var b []byte
		require.NoError(t, rows.Scan(&b))
		require.Equal(t, []byte("hello"), b)
	})
}

func TestReplayErrors(t *testing.T) {
	rows := replay(recordEntry(t, "", []string{"id"}, [][]any{{1}, {nil}}, new(any)))
	require.Error(t, rows.Scan(new(int)))
	require.True(t, rows.Next())
	require.Error(t, rows.Scan())
	require.Error(t, rows.Scan(new(string)))
	require.Error(t, rows.Scan((*int)(nil)))
	require.True(t, rows.Next())
	require.Error(t, rows.Scan(new(int)))
	require.NoError(t, rows.Close())
	require.False(t, rows.Next())
	require.Error(t, rows.Scan(new(any)))
}
