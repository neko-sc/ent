// Copyright 2026 Neko Works LLC
// SPDX-License-Identifier: Apache-2.0

package cache

import (
	"encoding/hex"
	"slices"

	"github.com/neko-sc/ent/dialect"
)

type set map[string]struct{}

func newSet(values []string) set {
	result := make(set, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func keyTags(
	statement *dialect.Statement, options queryOptions,
	ttlOnlyTables set, hotTables *hotTableTracker,
) []string {
	tags := []string{"*"}

	if !options.ttlOnly {
		if statement != nil {
			for index, table := range statement.Tables {
				if _, excluded := ttlOnlyTables[table]; excluded {
					continue
				}
				if !statement.PointRead && hotTables.demoted(table) {
					continue
				}
				if index != 0 || !statement.PointRead {
					tags = append(tags, table)
					continue
				}
				tags = append(tags, table+"!bulk")
				for _, id := range statement.RequestedIDs {
					encoded, err := encodeKeyValue(nil, id)
					if err != nil {
						// Unknown ID representations require table-wide invalidation.
						tags = append(tags, table)
						break
					}
					tags = append(tags, table+"#"+hex.EncodeToString(encoded))
				}
			}
		}
		for _, table := range options.dependsOn {
			if _, excluded := ttlOnlyTables[table]; excluded {
				continue
			}
			if statement != nil && !statement.PointRead && hotTables.demoted(table) {
				continue
			}
			tags = append(tags, table)
		}
	}
	for _, tag := range options.tags {
		tags = append(tags, "~"+tag)
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

func validationTags(statement *dialect.Statement, ids []any, ttlOnlyTables set) []string {
	if statement == nil || !statement.PointRead || len(statement.RequestedIDs) != 0 || len(statement.Tables) == 0 {
		return nil
	}
	table := statement.Tables[0]
	if _, excluded := ttlOnlyTables[table]; excluded {
		return nil
	}
	if len(ids) == 0 {
		return []string{table}
	}
	tags := make([]string, 0, len(ids))
	for _, id := range ids {
		encoded, err := encodeKeyValue(nil, id)
		if err != nil {
			return []string{table}
		}
		tags = append(tags, table+"#"+hex.EncodeToString(encoded))
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}

func writeTags(statement *dialect.Statement, touches []string) []string {
	if statement == nil {
		if len(touches) == 0 {
			return []string{"*"}
		}
		tags := make([]string, 0, 2*len(touches))
		for _, table := range touches {
			tags = append(tags, table, table+"!bulk")
		}
		slices.Sort(tags)
		return slices.Compact(tags)
	}
	tags := slices.Clone(statement.Tables)

	for _, row := range statement.Rows {
		encoded, err := encodeKeyValue(nil, row.ID)
		if err != nil {
			// Bulk invalidation also reaches existing point-read entries.
			tags = append(tags, row.Table, row.Table+"!bulk")
			continue
		}
		tags = append(tags, row.Table, row.Table+"#"+hex.EncodeToString(encoded))
	}

	for _, table := range statement.Bulk {
		tags = append(tags, table, table+"!bulk")
	}

	for _, table := range touches {
		tags = append(tags, table, table+"!bulk")
	}
	slices.Sort(tags)
	return slices.Compact(tags)
}
