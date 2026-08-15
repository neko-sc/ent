// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package semantic

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/entc/load/testdata/semantic/schema_types"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/mixin"
)

type Envelope[Value, Phantom any] struct {
	Value Value
}

type Phantom[Value, Marker any] struct {
	Value Value
}

type Status string

type EncodedBytes []byte

var EncodedValidatorCalls []string

type FormattedTime time.Time

type DirectTime time.Time

type formattedTimeCodec struct{}

func (formattedTimeCodec) Value(value FormattedTime) (driver.Value, error) {
	return time.Time(value), nil
}

func (formattedTimeCodec) ScanValue() field.ValueScanner { return new(sql.NullTime) }

func (formattedTimeCodec) FromValue(value driver.Value) (FormattedTime, error) {
	return FormattedTime(value.(*sql.NullTime).Time), nil
}

type Validated string

func (status Status) Validate() error { return nil }

func (Validated) Validate() error { return nil }

type BigInt struct{}

func (BigInt) Add(BigInt) BigInt { return BigInt{} }

func (*BigInt) Scan(any) error { return nil }

func (BigInt) Value() (driver.Value, error) { return int64(0), nil }

type Link struct{}

func (*Link) Scan(any) error { return nil }

func (Link) Value() (driver.Value, error) { return "link", nil }

func (Link) String() string { return "link" }

type StatusCodec struct{}

func (StatusCodec) Value(status Status) (driver.Value, error) { return string(status), nil }

func (StatusCodec) ScanValue() field.ValueScanner { return new(statusScanner) }

func (StatusCodec) FromValue(value driver.Value) (Status, error) {
	return Status(*value.(*statusScanner)), nil
}

type statusScanner string

func (*statusScanner) Scan(any) error              { return nil }
func (statusScanner) Value() (driver.Value, error) { return nil, nil }

type restrictionListCodec struct{}

func (restrictionListCodec) Value([]schema_types.RestrictionType) (driver.Value, error) {
	return nil, nil
}

func (restrictionListCodec) ScanValue() field.ValueScanner { return new(sql.NullString) }

func (restrictionListCodec) FromValue(driver.Value) ([]schema_types.RestrictionType, error) {
	return nil, nil
}

func genericField[Representation any](name string) ent.Field {
	return field.JSON[Representation](name)
}

type genericMixin[Representation any] struct {
	mixin.Schema
}

func (genericMixin[Representation]) Fields() []ent.Field {
	return []ent.Field{field.JSON[Representation]("mixed")}
}

type Tag struct {
	ent.Schema
}

func (Tag) Fields() []ent.Field {
	return []ent.Field{
		field.UUID[uuid.UUID]("id").DefaultFunc(uuid.New),
	}
}

type User struct {
	ent.Schema
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{genericMixin[Phantom[url.URL, [3]bool]]{}}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{edge.To("tags", Tag.Type)}
}

func generatedPhase() string { return "active" }

func (User) Fields() []ent.Field {
	return []ent.Field{
		genericField[Envelope[map[string][]*Phantom[url.URL, [2]int], chan<- func(url.URL) error]]("payload"),
		genericField[Envelope[map[string][]*Phantom[url.URL, [2]int], chan<- func(url.URL) error]]("payload_duplicate"),
		field.StringAs[Status]("status").
			Default(Status("ready")).
			UpdateDefault(func() Status { return Status("ready") }).
			Validate(func(Status) error { return nil }).
			Codec(StatusCodec{}),
		field.Time("created_at"),
		field.TimeAs[DirectTime]("direct_at"),
		field.TimeAs[FormattedTime]("formatted_at").Codec(formattedTimeCodec{}),
		field.EnumAs[Status]("state").Values("ready", "done").Default(Status("ready")).Validate(func(Status) error { return nil }),
		field.Enum("phase").Values("active", "inactive").DefaultFunc(generatedPhase).UpdateDefault(generatedPhase),
		field.StringAs[Validated]("validated"),
		field.IntAs[BigInt]("big_int").Optional(),
		field.Int64As[time.Duration]("duration").Optional(),
		field.StringAs[Link]("link").Optional().Nillable().NotEmpty(),
		field.StringAs[*Link]("null_link").Optional().Nillable(),
		field.BytesAs[EncodedBytes]("encoded").
			Validate(func(value EncodedBytes) error {
				EncodedValidatorCalls = append(EncodedValidatorCalls, "representation:first:"+string(value))
				return nil
			}).
			MinLen(4).
			Validate(func(value EncodedBytes) error {
				EncodedValidatorCalls = append(EncodedValidatorCalls, "representation:second:"+string(value))
				return nil
			}),
		field.JSON[json.RawMessage]("raw").Optional(),
		field.JSON[map[string]string]("document").Optional(),
		field.Other[[]schema_types.RestrictionType]("restrictions").
			SchemaType(map[string]string{dialect.Postgres: "text[]"}).
			Codec(restrictionListCodec{}),
	}
}
