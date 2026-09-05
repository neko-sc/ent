// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package integration

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/ent"
	"github.com/neko-sc/ent/entc/integration/ent/fieldtype"
	"github.com/neko-sc/ent/entc/integration/ent/role"
	"github.com/neko-sc/ent/entc/integration/ent/schema"
	"github.com/neko-sc/ent/entc/integration/ent/schema/task"
	enttask "github.com/neko-sc/ent/entc/integration/ent/task"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func Types(t *testing.T, client *ent.Client) {
	ctx := context.Background()
	require := require.New(t)

	link, err := url.Parse("localhost")
	require.NoError(err)

	bigint := schema.NewBigInt(0)
	require.NoError(bigint.Scan("1000"))

	ft := client.FieldType.Create().
		Set(fieldtype.Int, 1).
		Set(fieldtype.Int8, 8).
		Set(fieldtype.Int16, 16).
		Set(fieldtype.Int32, 32).
		Set(fieldtype.Int64, 64).
		SaveX(ctx)

	require.NotEmpty(t, ft.ID)
	require.Equal(1, ft.Int)
	require.Equal(int8(8), ft.Int8)
	require.Equal(int16(16), ft.Int16)
	require.Equal(int32(32), ft.Int32)
	require.Equal(int64(64), ft.Int64)
	require.Nil(ft.NullLink)
	require.Nil(ft.NilPair)
	require.Nil(ft.Deleted)

	ft = client.FieldType.Create().
		Set(fieldtype.Int, 1).
		Set(fieldtype.Int8, math.MinInt8).
		Set(fieldtype.Int16, math.MinInt16).
		Set(fieldtype.Int32, math.MinInt16).
		Set(fieldtype.Int64, math.MinInt16).
		Set(fieldtype.OptionalInt8, math.MinInt8).
		Set(fieldtype.OptionalInt16, math.MinInt16).
		Set(fieldtype.OptionalInt32, math.MinInt32).
		Set(fieldtype.OptionalInt64, math.MinInt64).
		Set(fieldtype.NillableInt8, math.MinInt8).
		Set(fieldtype.NillableInt16, math.MinInt16).
		Set(fieldtype.NillableInt32, math.MinInt32).
		Set(fieldtype.NillableInt64, math.MinInt64).
		Set(fieldtype.Dir, "dir").
		Set(fieldtype.Ndir, "ndir").
		Set(fieldtype.NullStr, &sql.NullString{String: "not-default", Valid: true}).
		Set(fieldtype.Link, schema.Link{URL: link}).
		Set(fieldtype.LinkOther, &schema.Link{URL: link}).
		Set(fieldtype.NullLink, &schema.Link{URL: link}).
		Set(fieldtype.Role, role.Admin).
		Set(fieldtype.Priority, role.High).
		Set(fieldtype.Duration, time.Hour).
		Set(fieldtype.Pair, schema.Pair{K: []byte("K"), V: []byte("V")}).
		Set(fieldtype.NilPair, &schema.Pair{K: []byte("K"), V: []byte("V")}).
		Set(fieldtype.StringArray, []string{"foo", "bar", "baz"}).
		Set(fieldtype.BigInt, bigint).
		Set(fieldtype.RawData, []byte{1, 2, 3}).
		SaveX(ctx)

	require.Equal(int8(math.MinInt8), ft.OptionalInt8)
	require.Equal(int16(math.MinInt16), ft.OptionalInt16)
	require.Equal(int32(math.MinInt32), ft.OptionalInt32)
	require.Equal(int64(math.MinInt64), ft.OptionalInt64)
	require.Equal(int8(math.MinInt8), *ft.NillableInt8)
	require.Equal(int16(math.MinInt16), *ft.NillableInt16)
	require.Equal(int32(math.MinInt32), *ft.NillableInt32)
	require.Equal(int64(math.MinInt64), *ft.NillableInt64)
	require.Equal([]byte{1, 2, 3}, ft.RawData)
	require.Equal(http.Dir("dir"), ft.Dir)
	require.NotNil(*ft.Ndir)
	require.Equal(http.Dir("ndir"), *ft.Ndir)
	require.Equal("default", ft.Str.String)
	require.Equal("not-default", ft.NullStr.String)
	require.Equal("localhost", ft.Link.String())
	require.Equal("localhost", ft.LinkOther.String())
	require.Equal("localhost", ft.NullLink.String())
	require.Equal(net.ParseIP("127.0.0.1"), ft.IP)
	mac, err := net.ParseMAC("3b:b3:6b:3c:10:79")
	require.Equal(role.Admin, ft.Role)
	require.Equal(role.High, ft.Priority)
	require.NoError(err)
	dt, err := time.Parse(time.RFC3339, "1906-01-02T00:00:00+00:00")
	require.NoError(err)
	require.Equal(schema.Pair{K: []byte("K"), V: []byte("V")}, ft.Pair)
	require.Equal(&schema.Pair{K: []byte("K"), V: []byte("V")}, ft.NilPair)
	require.EqualValues([]string{"foo", "bar", "baz"}, ft.StringArray)
	require.Equal("1000", ft.BigInt.String())
	exists, err := client.FieldType.Query().Where(fieldtype.Duration.LT(time.Hour * 2)).Exist(ctx)
	require.NoError(err)
	require.True(exists)
	exists, err = client.FieldType.Query().Where(fieldtype.Duration.LT(time.Hour)).Exist(ctx)
	require.NoError(err)
	require.False(exists)
	require.Equal("127.0.0.1", ft.LinkOtherFunc.String())
	require.False(ft.DeletedAt.Time.IsZero())

	ft = client.FieldType.UpdateOne(ft).Add(fieldtype.OptionalUint64, 10).SaveX(ctx)
	require.EqualValues(10, ft.OptionalUint64)
	ft = client.FieldType.UpdateOne(ft).Add(fieldtype.OptionalUint64, 20).Set(fieldtype.OptionalUint64, 5).SaveX(ctx)
	require.EqualValues(5, ft.OptionalUint64)
	ft = client.FieldType.UpdateOne(ft).Apply(ent.FieldTypePatch{OptionalUint64Add: ent.Some[int64](-5)}).SaveX(ctx)
	require.Zero(ft.OptionalUint64)

	err = client.FieldType.Create().
		Set(fieldtype.Int, 1).
		Set(fieldtype.Int8, 8).
		Set(fieldtype.Int16, 16).
		Set(fieldtype.Int32, 32).
		Set(fieldtype.Int64, 64).
		Set(fieldtype.RawData, make([]byte, 40)).
		Exec(ctx)
	require.Error(err, "MaxLen validator should reject this operation")
	err = client.FieldType.Create().
		Set(fieldtype.Int, 1).
		Set(fieldtype.Int8, 8).
		Set(fieldtype.Int16, 16).
		Set(fieldtype.Int32, 32).
		Set(fieldtype.Int64, 64).
		Set(fieldtype.RawData, make([]byte, 2)).
		Exec(ctx)
	require.Error(err, "MinLen validator should reject this operation")
	ft = ft.Update().
		Set(fieldtype.Int, 1).
		Set(fieldtype.Int8, math.MaxInt8).
		Set(fieldtype.Int16, math.MaxInt16).
		Set(fieldtype.Int32, math.MaxInt16).
		Set(fieldtype.OptionalInt8, math.MaxInt8).
		Set(fieldtype.OptionalInt16, math.MaxInt16).
		Set(fieldtype.OptionalInt32, math.MaxInt32).
		Set(fieldtype.OptionalInt64, math.MaxInt64).
		Set(fieldtype.NillableInt8, math.MaxInt8).
		Set(fieldtype.NillableInt16, math.MaxInt16).
		Set(fieldtype.NillableInt32, math.MaxInt32).
		Set(fieldtype.NillableInt64, math.MaxInt64).
		Set(fieldtype.Datetime, dt).
		Set(fieldtype.Decimal, 10.20).
		Set(fieldtype.Dir, "dir").
		Set(fieldtype.Ndir, "ndir").
		Set(fieldtype.Str, sql.NullString{String: "str", Valid: true}).
		Set(fieldtype.NullStr, &sql.NullString{String: "str", Valid: true}).
		Set(fieldtype.Link, schema.Link{URL: link}).
		Set(fieldtype.NullLink, &schema.Link{URL: link}).
		Set(fieldtype.LinkOther, &schema.Link{URL: link}).
		Set(fieldtype.SchemaInt, 64).
		Set(fieldtype.SchemaInt8, 8).
		Set(fieldtype.SchemaInt64, 64).
		Set(fieldtype.MAC, schema.MAC{HardwareAddr: mac}).
		Set(fieldtype.Pair, schema.Pair{K: []byte("K1"), V: []byte("V1")}).
		Set(fieldtype.NilPair, &schema.Pair{K: []byte("K1"), V: []byte("V1")}).
		Set(fieldtype.StringArray, []string{"qux"}).
		Apply(ent.FieldTypePatch{BigIntAdd: ent.Some(bigint)}).
		SaveX(ctx)

	require.Equal(int8(math.MaxInt8), ft.OptionalInt8)
	require.Equal(int16(math.MaxInt16), ft.OptionalInt16)
	require.Equal(int32(math.MaxInt32), ft.OptionalInt32)
	require.Equal(int64(math.MaxInt64), ft.OptionalInt64)
	require.Equal(int8(math.MaxInt8), *ft.NillableInt8)
	require.Equal(int16(math.MaxInt16), *ft.NillableInt16)
	require.Equal(int32(math.MaxInt32), *ft.NillableInt32)
	require.Equal(int64(math.MaxInt64), *ft.NillableInt64)
	require.Equal(10.20, ft.Decimal)
	require.True(dt.Equal(ft.Datetime))
	require.Equal(http.Dir("dir"), ft.Dir)
	require.NotNil(*ft.Ndir)
	require.Equal(http.Dir("ndir"), *ft.Ndir)
	require.Equal("str", ft.Str.String)
	require.Equal("str", ft.NullStr.String)
	require.Equal("localhost", ft.Link.String())
	require.Equal("localhost", ft.LinkOther.String())
	require.Equal("localhost", ft.NullLink.String())
	require.Equal(schema.Int(64), ft.SchemaInt)
	require.Equal(schema.Int8(8), ft.SchemaInt8)
	require.Equal(schema.Int64(64), ft.SchemaInt64)
	require.Equal(mac.String(), ft.MAC.String())
	require.Equal(schema.Pair{K: []byte("K1"), V: []byte("V1")}, ft.Pair)
	require.Equal(&schema.Pair{K: []byte("K1"), V: []byte("V1")}, ft.NilPair)
	require.EqualValues([]string{"qux"}, ft.StringArray)
	require.Nil(ft.NillableUUID)
	require.Equal(uuid.UUID{}, ft.OptionalUUID)
	require.Equal("2000", ft.BigInt.String())
	require.EqualValues(100, ft.Int64, "UpdateDefault sets the value to 100")
	require.EqualValues(100, ft.Duration, "UpdateDefault sets the value to 100ns")
	require.False(ft.DeletedAt.Time.IsZero())

	defaultTask := client.Task.Create().SaveX(ctx)
	require.Equal(task.PriorityMid, defaultTask.Priority)

	err = client.Task.CreateBulk(
		client.Task.Create().Set(enttask.Priority, task.PriorityLow),
		client.Task.Create().Set(enttask.Priority, task.PriorityMid),
		client.Task.Create().Set(enttask.Priority, task.PriorityHigh),
	).Exec(ctx)
	require.NoError(err)
	err = client.Task.Create().Set(enttask.Priority, task.Priority(10)).Exec(ctx)
	require.Error(err)
	err = client.Task.Update().Set(enttask.Priority, task.Priority(10)).Exec(ctx)
	require.Error(err)
	err = defaultTask.Update().Set(enttask.Priority, task.Priority(10)).Exec(ctx)
	require.Error(err)
	client.Task.DeleteOne(defaultTask).ExecX(ctx)

	tasks := client.Task.Query().Order(enttask.Priority.Asc()).AllX(ctx)
	require.Equal(task.PriorityLow, tasks[0].Priority)
	require.Equal(task.PriorityMid, tasks[1].Priority)
	require.Equal(task.PriorityHigh, tasks[2].Priority)

	tasks = client.Task.Query().Order(enttask.Priority.Desc()).AllX(ctx)
	require.Equal(task.PriorityLow, tasks[2].Priority)
	require.Equal(task.PriorityMid, tasks[1].Priority)
	require.Equal(task.PriorityHigh, tasks[0].Priority)
}
