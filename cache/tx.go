// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/neko-sc/ent/dialect"
)

type Tx struct {
	executor
	underlying dialect.Tx
	ctx        context.Context
	dirty      atomic.Bool
	mutex      sync.Mutex
	pending    []string
	openWrites map[*writeRows]struct{}
}

func (d *Driver) Tx(ctx context.Context) (dialect.Tx, error) {
	return d.BeginTx(ctx, nil)
}

func (d *Driver) BeginTx(ctx context.Context, options *dialect.TxOptions) (dialect.Tx, error) {
	underlying, err := d.Driver.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	transaction := &Tx{underlying: underlying, ctx: ctx, openWrites: make(map[*writeRows]struct{})}
	transaction.executor = executor{ExecQuerier: underlying, driver: d, transaction: transaction}
	return transaction, nil
}

func (t *Tx) Commit() error {
	t.dirty.Store(true)
	if err := t.underlying.Commit(); err != nil {
		return err
	}

	t.mutex.Lock()
	pending := t.pending
	for rows := range t.openWrites {
		pending = append(pending, writeTags(rows.statement, rows.touches)...)
		// Unconsumed RETURNING rows may identify additional writes after commit.
		if rows.statement != nil {
			for _, table := range rows.statement.Tables {
				pending = append(pending, table+"!bulk")
			}
		}
	}

	clear(t.openWrites)
	t.pending = nil
	t.mutex.Unlock()
	if err := t.driver.bump(context.WithoutCancel(t.ctx), pending); err != nil {
		t.driver.onError(t.ctx, err)
	}
	return nil
}

func (t *Tx) Rollback() error {
	t.dirty.Store(true)
	err := t.underlying.Rollback()
	t.mutex.Lock()
	clear(t.openWrites)
	t.pending = nil
	t.mutex.Unlock()
	return err
}
