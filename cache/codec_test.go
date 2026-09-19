// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"database/sql"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	sef "github.com/neko-sc/sef/go"
	"github.com/stretchr/testify/require"
)

// encodingLevel stores entries on the wire, as a remote level does, so cache
// behavior observes only what survives encoding and decoding.
type encodingLevel struct {
	mutex   sync.Mutex
	entries map[Key][]byte
}

func newEncodingLevel() *encodingLevel {
	return &encodingLevel{entries: make(map[Key][]byte)}
}

func (*encodingLevel) Name() string {
	return "encoding"
}

func (l *encodingLevel) Get(_ context.Context, key Key) (*Entry, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	data, found := l.entries[key]
	if !found {
		return nil, nil
	}
	return DecodeEntry(data)
}

func (l *encodingLevel) Set(_ context.Context, key Key, entry *Entry, ttl time.Duration) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if entry == nil || ttl <= 0 {
		delete(l.entries, key)
		return nil
	}
	data, err := entry.Bytes()
	if err != nil {
		return err
	}
	l.entries[key] = data
	return nil
}

func (l *encodingLevel) Delete(_ context.Context, keys ...Key) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
	return nil
}

func TestEntryCodec(t *testing.T) {
	t.Run("roundtrip replays every column type", func(t *testing.T) {
		timestamp := time.Date(2026, 9, 18, 12, 13, 14, 15, time.FixedZone("offset", 3600))
		var id int
		var text string
		var bytes []byte
		var instant time.Time
		var null *int
		var scanner valueScanner
		var dynamic any
		entry := recordEntry(t,
			"id",
			[]string{"id", "text", "bytes", "time", "null", "scanner", "dynamic"},
			[][]any{{7, "text", []byte("bytes"), timestamp, nil, "scanner", int32(12)}},
			&id, &text, &bytes, &instant, &null, &scanner, &dynamic,
		)
		encoded, err := entry.Bytes()
		require.NoError(t, err)
		decoded, err := DecodeEntry(encoded)
		require.NoError(t, err)
		require.Equal(t, entry.Columns(), decoded.Columns())
		require.Equal(t, 1, decoded.Len())
		for range 2 {
			replayed := replay(decoded)
			require.True(t, replayed.Next())
			require.NoError(t, replayed.Scan(&id, &text, &bytes, &instant, &null, &scanner, &dynamic))
			require.Equal(t, 7, id)
			require.Equal(t, "text", text)
			require.Equal(t, []byte("bytes"), bytes)
			require.Equal(t, timestamp.UTC(), instant)
			require.Same(t, time.UTC, instant.Location())
			require.Nil(t, null)
			require.Equal(t, "scanner", scanner.value)
			require.Equal(t, int32(12), dynamic)
			// Replayed cells belong to the caller and must not corrupt the entry.
			bytes[0] = 'X'
			require.NoError(t, replayed.Close())
		}
		// Bytes returns a copy, so mutating it cannot change either entry.
		encoded[0] ^= 0xff
		again, err := decoded.Bytes()
		require.NoError(t, err)
		original, err := entry.Bytes()
		require.NoError(t, err)
		require.Equal(t, original, again)
	})

	t.Run("typed destinations decode from dynamic cells", func(t *testing.T) {
		type record struct {
			Name  string
			Bytes []byte
		}
		entry := recordEntry(t,
			"",
			[]string{"record", "nullable", "nullScanner"},
			[][]any{{record{"name", []byte("data")}, 12, nil}},
			new(record), new(int), new(any),
		)
		encoded, err := entry.Bytes()
		require.NoError(t, err)
		decoded, err := DecodeEntry(encoded)
		require.NoError(t, err)
		rows := replay(decoded)
		var value record
		var nullable *int
		var null sql.NullString
		require.True(t, rows.Next())
		require.NoError(t, rows.Scan(&value, &nullable, &null))
		require.Equal(t, record{"name", []byte("data")}, value)
		require.Equal(t, 12, *nullable)
		require.False(t, null.Valid)
		require.NoError(t, rows.Close())
	})

	t.Run("dynamic destinations own their cells", func(t *testing.T) {
		entry := recordEntry(t,
			"",
			[]string{"scalar", "bytes", "map", "null"},
			[][]any{{int32(7), []byte("bytes"), map[string][]byte{"key": []byte("value")}, nil}},
			new(any), new(any), new(any), new(any),
		)
		encoded, err := entry.Bytes()
		require.NoError(t, err)
		decoded, err := DecodeEntry(encoded)
		require.NoError(t, err)
		for range 2 {
			rows := replay(decoded)
			var scalar, bytes, mapping, null any
			require.True(t, rows.Next())
			require.NoError(t, rows.Scan(&scalar, &bytes, &mapping, &null))
			require.Equal(t, int32(7), scalar)
			require.Equal(t, []byte("bytes"), bytes)
			require.Nil(t, null)
			require.Equal(t, map[any]any{"key": []byte("value")}, mapping)
			bytes.([]byte)[0] = 'X'
			mapping.(map[any]any)["key"].([]byte)[0] = 'X'
			require.NoError(t, rows.Close())
		}
	})

	t.Run("empty result replays no rows", func(t *testing.T) {
		encoded, err := recordEntry(t, "id", []string{"id"}, nil, new(int)).Bytes()
		require.NoError(t, err)
		entry, err := DecodeEntry(encoded)
		require.NoError(t, err)
		require.Zero(t, entry.Len())
		rows := replay(entry)
		require.False(t, rows.Next())
		require.NoError(t, rows.Close())
	})

	t.Run("malformed input is rejected", func(t *testing.T) {
		_, err := DecodeEntry([]byte("invalid"))
		require.Error(t, err)
		encoded, err := sef.Marshal(entryWire{Columns: []string{"id"}, Rows: [][][]byte{{nil, nil}}})
		require.NoError(t, err)
		_, err = DecodeEntry(encoded)
		require.Error(t, err)
	})

	t.Run("decode requires a non-nil pointer", func(t *testing.T) {
		require.Error(t, decodeValueInto(nil, 1))
		require.Error(t, decodeValueInto(nil, (*int)(nil)))
	})

	t.Run("concurrent encode and replay", func(t *testing.T) {
		entry := recordEntry(t, "id", []string{"id"}, [][]any{{42}}, new(int))
		encoded, err := entry.Bytes()
		require.NoError(t, err)
		decoded, err := DecodeEntry(encoded)
		require.NoError(t, err)
		var workers sync.WaitGroup
		for range 20 {
			workers.Go(func() {
				for range 20 {
					data, encodeError := entry.Bytes()
					if encodeError != nil || len(data) == 0 {
						t.Error("encode failed", encodeError)
					}
					rows := replay(decoded)
					var value int
					if !rows.Next() || rows.Scan(&value) != nil || value != 42 {
						t.Error("replay failed")
					}
					if closeError := rows.Close(); closeError != nil {
						t.Error(closeError)
					}
				}
			})
		}
		workers.Wait()
	})
}

// TestEntryMetadataSurvivesWire covers the fill and validation metadata that has no
// public accessor: an encoded entry must still be servable, still expire, and
// still be invalidated by a write to exactly the rows it recorded.
func TestEntryMetadataSurvivesWire(t *testing.T) {
	newDriver := func(t *testing.T, level Level, options ...Option) (*Driver, sqlmock.Sqlmock) {
		t.Helper()
		database, mock, err := sqlmock.New()
		require.NoError(t, err)
		cached, err := New(entsql.OpenDB(dialect.SQLite, database), append([]Option{Levels(level)}, options...)...)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, mock.ExpectationsWereMet())
			mock.ExpectClose()
			require.NoError(t, cached.Close())
		})
		return cached, mock
	}
	// A point read without requested identifiers keys on the table alone, so only
	// the validation tags recorded from the result rows can invalidate it.
	pointRead := func(ctx context.Context) context.Context {
		return dialect.WithStatement(ctx, &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}, IDColumn: "id", PointRead: true})
	}

	t.Run("encoded entry serves later reads", func(t *testing.T) {
		cached, mock := newDriver(t, newEncodingLevel())
		expectRead(mock, "SELECT id", 1)
		ctx, info := WithInfo(Cache(pointRead(t.Context())))
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.True(t, info.Hit)
		require.Equal(t, "encoding", info.Level)
	})

	t.Run("encoded entry keeps its row identity", func(t *testing.T) {
		cached, mock := newDriver(t, newEncodingLevel())
		expectRead(mock, "SELECT id", 1)
		ctx, info := WithInfo(Cache(pointRead(t.Context())))
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		require.NoError(t, cached.Invalidate(t.Context(), Rows("users", 2)))
		readIDs(t, cached, ctx, "SELECT id")
		require.True(t, info.Hit, "an unrelated row must not invalidate the entry")
		require.NoError(t, cached.Invalidate(t.Context(), Rows("users", int64(1))))
		expectRead(mock, "SELECT id", 3)
		require.Equal(t, []int64{3}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})

	t.Run("encoded entry keeps its expiry", func(t *testing.T) {
		cached, mock := newDriver(t, newEncodingLevel(), TTL(50*time.Millisecond))
		expectRead(mock, "SELECT id", 1)
		ctx, info := WithInfo(Cache(pointRead(t.Context())))
		require.Equal(t, []int64{1}, readIDs(t, cached, ctx, "SELECT id"))
		time.Sleep(80 * time.Millisecond)
		expectRead(mock, "SELECT id", 2)
		require.Equal(t, []int64{2}, readIDs(t, cached, ctx, "SELECT id"))
		require.False(t, info.Hit)
	})
}

func TestEncodeKeyValue(t *testing.T) {
	encode := func(t *testing.T, value any) []byte {
		t.Helper()
		encoded, err := encodeKeyValue(nil, value)
		require.NoError(t, err)
		return encoded
	}

	t.Run("signed widths share one representation", func(t *testing.T) {
		for _, value := range []any{int(1), int8(1), int16(1), int32(1), int64(1), &valueScanner{value: int64(1)}} {
			require.Equal(t, encode(t, int64(1)), encode(t, value))
		}
	})

	t.Run("unsigned widths share one representation", func(t *testing.T) {
		for _, value := range []any{uint(1), uint8(1), uint16(1), uint32(1), uint64(1)} {
			require.Equal(t, encode(t, uint64(1)), encode(t, value))
		}
	})

	t.Run("distinct values never collide", func(t *testing.T) {
		for _, pair := range [][2]any{{int64(1), "1"}, {int64(1), 1.0}, {"1", 1.0}, {nil, ""}, {nil, []byte{}}, {"", []byte{}}, {true, int64(1)}, {int64(-1), uint64(math.MaxUint64)}} {
			require.NotEqual(t, encode(t, pair[0]), encode(t, pair[1]))
		}
	})

	t.Run("equal values encode identically", func(t *testing.T) {
		timestamp := time.Now()
		require.Equal(t, encode(t, timestamp), encode(t, timestamp.UTC()))
		require.Equal(t, encode(t, map[string]int{"a": 1, "b": 2}), encode(t, map[string]int{"b": 2, "a": 1}))
	})

	t.Run("unsupported values fail", func(t *testing.T) {
		for name, value := range map[string]any{"channel": make(chan int), "function": func() {}, "failing valuer": &valueScanner{fail: true}} {
			t.Run(name, func(t *testing.T) {
				_, err := encodeKeyValue(nil, value)
				require.Error(t, err)
			})
		}
	})
}
