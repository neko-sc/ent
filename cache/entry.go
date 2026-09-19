// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"slices"
	"sync"

	sef "github.com/neko-sc/sef/go"
)

// Entry is an immutable SELECT result, shared by cache levels and readers.
type Entry struct {
	columns        []string
	rows           [][]any
	ids            []any
	validationTags []string
	fillStart      uint64
	expiresAt      uint64
	staleUntil     uint64
	estimatedBytes int64
	encoded        []byte
	encodedRows    [][][]byte
	encodeOnce     sync.Once
	encodeError    error
	decodeOnce     sync.Once
	decodeError    error
}

func (e *Entry) Columns() []string {
	return slices.Clone(e.columns)
}

func (e *Entry) Len() int {
	if e.encodedRows != nil {
		return len(e.encodedRows)
	}
	return len(e.rows)
}

// Bytes memoizes the wire representation. The returned bytes belong to the caller.
// SEF preserves time instants but normalizes their locations to UTC.
func (e *Entry) Bytes() ([]byte, error) {
	e.encodeOnce.Do(func() {
		if e.encoded != nil {
			return
		}

		wire := entryWire{
			Columns:        e.columns,
			IDs:            make([][]byte, len(e.ids)),
			Rows:           make([][][]byte, len(e.rows)),
			ValidationTags: e.validationTags,
			FillStart:      e.fillStart,
			ExpiresAt:      e.expiresAt,
			StaleUntil:     e.staleUntil,
		}
		for index, id := range e.ids {
			wire.IDs[index], e.encodeError = encodeValue(id)
			if e.encodeError != nil {
				return
			}
		}
		for rowIndex, row := range e.rows {
			wire.Rows[rowIndex] = make([][]byte, len(row))
			for columnIndex, value := range row {
				wire.Rows[rowIndex][columnIndex], e.encodeError = encodeValue(value)
				if e.encodeError != nil {
					return
				}
			}
		}
		e.encoded, e.encodeError = sef.Marshal(wire)
	})
	return slices.Clone(e.encoded), e.encodeError
}

// DecodeEntry retains encoded cells; replay decodes their values on first use.
func DecodeEntry(data []byte) (*Entry, error) {
	entry := &Entry{encoded: slices.Clone(data), estimatedBytes: int64(len(data))}
	var wire entryWire
	if err := sef.Unmarshal(entry.encoded, &wire); err != nil {
		return nil, err
	}
	entry.columns = wire.Columns
	entry.validationTags = wire.ValidationTags
	entry.fillStart = wire.FillStart
	entry.expiresAt = wire.ExpiresAt
	entry.staleUntil = wire.StaleUntil
	entry.encodedRows = wire.Rows
	if entry.encodedRows == nil {
		entry.encodedRows = make([][][]byte, 0)
	}
	entry.ids = make([]any, len(wire.IDs))
	for index, data := range wire.IDs {
		if err := decodeValueInto(data, &entry.ids[index]); err != nil {
			return nil, err
		}
	}

	for _, row := range wire.Rows {
		if len(row) != len(wire.Columns) {
			return nil, fmt.Errorf("cache: expected %d columns, got %d", len(wire.Columns), len(row))
		}
	}
	return entry, nil
}

func (e *Entry) decode() error {
	e.decodeOnce.Do(func() {
		if e.encodedRows == nil || e.rows != nil {
			// Refreshed entries carry snapshot rows alongside their encoded cells.
			return
		}
		e.rows = make([][]any, len(e.encodedRows))

		for rowIndex, row := range e.encodedRows {
			e.rows[rowIndex] = make([]any, len(row))
			for columnIndex, data := range row {
				if e.decodeError = decodeValueInto(data, &e.rows[rowIndex][columnIndex]); e.decodeError != nil {
					return
				}
			}
		}
	})
	return e.decodeError
}
