// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql/schema"
	"github.com/zeebo/xxh3"
)

// Schema isolates entries across changes to the tables' result shapes and unique keys.
func Schema(tables ...*schema.Table) Option {
	tables = slices.Clone(tables)
	return func(driver *Driver) error {
		var descriptions [][]byte
		for _, table := range tables {
			if table == nil {
				return fmt.Errorf("cache: nil schema table")
			}
			columns := slices.Clone(table.Columns)
			for _, column := range columns {
				if column == nil {
					return fmt.Errorf("cache: nil schema column in %s", table.Name)
				}
			}
			slices.SortFunc(columns, func(left, right *schema.Column) int { return cmp.Compare(left.Name, right.Name) })
			var fields []any
			for _, column := range columns {
				fields = append(fields, []any{
					column.Name, uint64(column.Type), column.SchemaType,
					column.Nullable, column.Unique, column.Key,
				})
			}
			var unique [][]byte
			for _, index := range table.Indexes {
				if index == nil {
					return fmt.Errorf("cache: nil schema index in %s", table.Name)
				}
				if !index.Unique {
					continue
				}
				names := make([]string, len(index.Columns))
				for position, column := range index.Columns {
					if column == nil {
						return fmt.Errorf("cache: nil unique index column in %s", table.Name)
					}
					names[position] = column.Name
				}
				encoded, err := encodeKeyValue(nil, names)
				if err != nil {
					return err
				}
				unique = append(unique, encoded)
			}
			slices.SortFunc(unique, bytes.Compare)
			description, err := encodeKeyValue(nil, []any{dialect.QualifiedTable(table.Schema, table.Name), fields, unique})
			if err != nil {
				return err
			}
			descriptions = append(descriptions, description)
		}
		slices.SortFunc(descriptions, bytes.Compare)
		encoded, err := encodeKeyValue(nil, descriptions)
		if err != nil {
			return err
		}
		driver.schemaPrefix = xxh3.Hash128(encoded).Bytes()
		return nil
	}
}
