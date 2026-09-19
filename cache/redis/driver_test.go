// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package redis_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/neko-sc/ent/cache"
	"github.com/neko-sc/ent/cache/redis"
	"github.com/neko-sc/ent/dialect"
	entsql "github.com/neko-sc/ent/dialect/sql"
	"github.com/stretchr/testify/require"
)

type capturedEntry struct {
	entry *cache.Entry
}

func (*capturedEntry) Get(context.Context, cache.Key) (*cache.Entry, error) {
	return nil, nil
}

func (c *capturedEntry) Set(_ context.Context, _ cache.Key, entry *cache.Entry, _ time.Duration) error {
	c.entry = entry
	return nil
}

func (*capturedEntry) Delete(context.Context, ...cache.Key) error {
	return nil
}

func newDriver(t *testing.T, levels ...cache.Level) (*cache.Driver, sqlmock.Sqlmock) {
	t.Helper()
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	driver, err := cache.New(entsql.OpenDB(dialect.SQLite, database), cache.Levels(levels...), cache.TTL(time.Minute), cache.OnError(func(_ context.Context, err error) {
		t.Errorf("cache error: %v", err)
	}))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, mock.ExpectationsWereMet())
		mock.ExpectClose()
		require.NoError(t, driver.Close())
	})
	return driver, mock
}

func readID(t *testing.T, driver *cache.Driver) (int64, *cache.Info) {
	t.Helper()
	ctx, info := cache.WithInfo(cache.Cache(dialect.WithStatement(t.Context(), &dialect.Statement{Kind: dialect.StatementRead, Tables: []string{"users"}})))
	rows, err := driver.Query(ctx, "SELECT id FROM users", nil)
	require.NoError(t, err)
	require.True(t, rows.Next())
	var id int64
	require.NoError(t, rows.Scan(&id))
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	return id, info
}

func newEntry(t *testing.T) *cache.Entry {
	t.Helper()
	capture := &capturedEntry{}
	driver, mock := newDriver(t, capture)
	mock.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7))).RowsWillBeClosed()
	id, _ := readID(t, driver)
	require.Equal(t, int64(7), id)
	require.NotNil(t, capture.entry)
	return capture.entry
}

func TestDriverRedisBackfillAndInvalidation(t *testing.T) {
	prefix := fmt.Sprintf("%s:%d", t.Name(), time.Now().UnixNano())
	firstClient := newClient(t)
	firstLevel := redis.New(firstClient, prefix)
	t.Cleanup(func() { require.NoError(t, firstLevel.Close()) })
	secondClient := newClient(t)
	secondLevel := redis.New(secondClient, prefix)
	t.Cleanup(func() { require.NoError(t, secondLevel.Close()) })
	first, firstMock := newDriver(t, cache.NewMemory(100, 0), firstLevel)
	second, secondMock := newDriver(t, cache.NewMemory(100, 0), secondLevel)
	firstMock.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7))).RowsWillBeClosed()
	id, info := readID(t, first)
	require.Equal(t, int64(7), id)
	require.False(t, info.Hit)
	require.Eventually(t, func() bool {
		keys, err := firstClient.Do(t.Context(), firstClient.B().Keys().Pattern(prefix+":e:*").Build()).AsStrSlice()
		return err == nil && len(keys) == 1
	}, time.Second, time.Millisecond)
	id, info = readID(t, second)
	require.Equal(t, int64(7), id)
	require.True(t, info.Hit)
	require.Equal(t, "redis", info.Level)
	_, info = readID(t, second)
	require.Equal(t, "memory", info.Level)
	firstMock.ExpectExec("UPDATE users").WillReturnResult(sqlmock.NewResult(0, 1))
	_, err := first.Exec(dialect.WithStatement(t.Context(), &dialect.Statement{Kind: dialect.StatementWrite, Tables: []string{"users"}, Bulk: []string{"users"}}), "UPDATE users", nil)
	require.NoError(t, err)
	secondMock.ExpectQuery("SELECT id FROM users").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(8))).RowsWillBeClosed()
	// A command on the tracking connection observes all preceding invalidations.
	require.NoError(t, secondClient.Do(t.Context(), secondClient.B().Get().Key(prefix+":g:users").Build()).Error())
	id, info = readID(t, second)
	require.Equal(t, int64(8), id)
	require.False(t, info.Hit)
}
