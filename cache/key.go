// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"cmp"
	"encoding/binary"
	"encoding/hex"
	"slices"

	"github.com/zeebo/xxh3"
)

type Key [16]byte

func (k Key) String() string {
	return hex.EncodeToString(k[:])
}

type keyBuilder struct {
	buffer []byte
	prefix [16]byte
}

func (b *keyBuilder) query(dialect, query string, arguments []any) error {
	b.buffer = append(b.buffer[:0], b.prefix[:]...)
	b.buffer = append(binary.LittleEndian.AppendUint64(b.buffer, uint64(len(dialect))), dialect...)
	b.buffer = append(binary.LittleEndian.AppendUint64(b.buffer, uint64(len(query))), query...)
	b.buffer = binary.LittleEndian.AppendUint64(b.buffer, uint64(len(arguments)))
	for _, argument := range arguments {
		var err error
		b.buffer, err = encodeKeyValue(b.buffer, argument)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *keyBuilder) variables(variables [][2]string) {
	b.buffer = binary.LittleEndian.AppendUint64(b.buffer, uint64(len(variables)))
	for _, variable := range variables {
		for _, value := range variable {
			b.buffer = append(binary.LittleEndian.AppendUint64(b.buffer, uint64(len(value))), value...)
		}
	}
}

func (b *keyBuilder) tags(tags []string, tokens []uint64) {
	indices := make([]int, len(tags))
	for index := range indices {
		indices[index] = index
	}
	slices.SortFunc(indices, func(left, right int) int {
		if order := cmp.Compare(tags[left], tags[right]); order != 0 {
			return order
		}
		return cmp.Compare(tokens[left], tokens[right])
	})
	b.buffer = binary.LittleEndian.AppendUint64(b.buffer, uint64(len(tags)))
	for _, index := range indices {
		b.buffer = append(binary.LittleEndian.AppendUint64(b.buffer, uint64(len(tags[index]))), tags[index]...)
		b.buffer = binary.LittleEndian.AppendUint64(b.buffer, tokens[index])
	}
}

func (b *keyBuilder) sum() Key {
	return Key(xxh3.Hash128(b.buffer).Bytes())
}
