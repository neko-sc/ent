// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
)

func BenchmarkHit(b *testing.B) {
	level := NewMemory(100, 0)
	b.Cleanup(func() { level.(*memoryLevel).cache.StopAllGoroutines() })
	benchmarkHit(b, level)
}

func BenchmarkHitWire(b *testing.B) {
	benchmarkHit(b, newWireLevel())
}

func benchmarkHit(b *testing.B, level Level) {
	database, mock, err := sqlmock.New()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		mock.ExpectClose()
		if err := database.Close(); err != nil {
			b.Error(err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			b.Error(err)
		}
	})
	cached, err := New(entsql.OpenDB(dialect.SQLite, database), Levels(level), Generations(newClockGenerations()), TTL(time.Hour))
	if err != nil {
		b.Fatal(err)
	}
	ctx := Cache(readContext(b.Context()))
	const query = "SELECT id FROM users"
	expectRead(mock, query, 1)
	rows, err := cached.Query(ctx, query, nil)
	if err != nil {
		b.Fatal(err)
	}
	var id int64
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			b.Fatal(err)
		}
	}
	if err := rows.Err(); err != nil {
		b.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		b.Fatal(err)
	}
	if id != 1 || cached.Stats().Fills.Load() != 1 {
		b.Fatal("initial read did not fill the cache")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rows, err := cached.Query(ctx, query, nil)
		if err != nil {
			b.Fatal(err)
		}
		if !rows.Next() {
			b.Fatal("cached row missing")
		}
		if err := rows.Scan(&id); err != nil {
			b.Fatal(err)
		}
		if id != 1 || rows.Next() {
			b.Fatal("unexpected cached rows")
		}
		if err := rows.Err(); err != nil {
			b.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			b.Fatal(err)
		}
	}
	if cached.Stats().Snapshot().Hits[levelName(level)] != uint64(b.N) {
		b.Fatal("not every measured query hit the cache")
	}
}

func BenchmarkRecord(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			rows := benchmarkRows(n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchmarkRecordEntry(b, rows)
			}
		})
	}
}

func BenchmarkReplay(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			entry := benchmarkRecordEntry(b, benchmarkRows(n))
			var id int64
			var name string
			var active bool
			var score float64
			var data []byte
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				rows := replay(entry)
				count := 0
				for rows.Next() {
					if err := rows.Scan(&id, &name, &active, &score, &data); err != nil {
						b.Fatal(err)
					}
					count++
				}
				if err := rows.Err(); err != nil {
					b.Fatal(err)
				}
				if err := rows.Close(); err != nil {
					b.Fatal(err)
				}
				if count != n {
					b.Fatalf("replayed %d rows, want %d", count, n)
				}
			}
		})
	}
}

func BenchmarkKeyBuilder(b *testing.B) {
	for _, n := range []int{0, 5, 20} {
		b.Run(fmt.Sprintf("args=%d", n), func(b *testing.B) {
			values := []any{int64(1), "alice", true, 42.5, []byte("payload")}
			arguments := make([]any, n)
			for index := range arguments {
				arguments[index] = values[index%len(values)]
			}
			query := "SELECT id FROM users"
			if n > 0 {
				query += " WHERE id IN (" + strings.TrimSuffix(strings.Repeat("?,", n), ",") + ")"
			}
			tags := []string{"users", "*", "~benchmark"}
			tokens := []uint64{1_000_000, 2_000_000, 3_000_000}
			var builder keyBuilder
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := builder.query(dialect.SQLite, query, arguments); err != nil {
					b.Fatal(err)
				}
				builder.tags(tags, tokens)
				builder.sum()
			}
		})
	}
}

func BenchmarkEntryCodec(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("rows=%d", n), func(b *testing.B) {
			entry := benchmarkRecordEntry(b, benchmarkRows(n))
			data, err := entry.Bytes()
			if err != nil {
				b.Fatal(err)
			}
			b.Run("encode", func(b *testing.B) {
				entries := make([]*Entry, 256)
				for index := range entries {
					entries[index] = &Entry{columns: entry.columns, rows: entry.rows, ids: entry.ids}
				}
				index := 0
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if index == len(entries) {
						b.StopTimer()
						for index := range entries {
							entries[index] = &Entry{columns: entry.columns, rows: entry.rows, ids: entry.ids}
						}
						index = 0
						b.StartTimer()
					}
					encoded, err := entries[index].Bytes()
					if err != nil {
						b.Fatal(err)
					}
					if len(encoded) != len(data) {
						b.Fatal("unexpected encoded entry size")
					}
					index++
				}
			})
			b.Run("decode", func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					decoded, err := DecodeEntry(data)
					if err != nil {
						b.Fatal(err)
					}
					if decoded.Len() != n {
						b.Fatalf("decoded %d rows, want %d", decoded.Len(), n)
					}
				}
			})
		})
	}
}

func benchmarkRows(n int) [][]any {
	rows := make([][]any, n)
	for index := range rows {
		rows[index] = []any{int64(index + 1), "alice", true, 42.5, []byte("payload")}
	}
	return rows
}

func benchmarkRecordEntry(b *testing.B, rows [][]any) *Entry {
	recorded := newRecorder(&fakeRows{
		columns: []string{"id", "name", "active", "score", "data"},
		rows:    rows,
	}, "id", Limits{})
	var id int64
	var name string
	var active bool
	var score float64
	var data []byte
	for recorded.Next() {
		if err := recorded.Scan(&id, &name, &active, &score, &data); err != nil {
			b.Fatal(err)
		}
	}
	if err := recorded.Err(); err != nil {
		b.Fatal(err)
	}
	if err := recorded.Close(); err != nil {
		b.Fatal(err)
	}
	entry, complete := recorded.Entry()
	if !complete || entry.Len() != len(rows) {
		b.Fatal("incomplete recorded entry")
	}
	return entry
}
