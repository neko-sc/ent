// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"database/sql/driver"
	"encoding/gob"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/dialect/sql"
	"github.com/neko-sc/ent/entc/integration/ent/role"
	"github.com/neko-sc/ent/schema/field"

	"github.com/google/uuid"
)

// FieldType holds the schema definition for the FieldType entity.
// used for testing field types.
type FieldType struct {
	ent.Schema
}

// Fields of the File.
func (FieldType) Fields() []ent.Field { //nolint:funlen
	return []ent.Field{
		// ----------------------------------------------------------------------------
		// Basic types

		field.Int("int"),
		field.Int8("int8"),
		field.Int16("int16"),
		field.Int32("int32"),
		field.Int64("int64").
			UpdateDefault(func() int64 {
				return 100
			}),
		field.Int("optional_int").
			Optional(),
		field.Int8("optional_int8").
			Optional(),
		field.Int16("optional_int16").
			Optional(),
		field.Int32("optional_int32").
			Optional(),
		field.Int64("optional_int64").
			Optional(),
		field.Int("nillable_int").
			Optional().
			Nillable(),
		field.Int8("nillable_int8").
			Optional().
			Nillable(),
		field.Int16("nillable_int16").
			Optional().
			Nillable(),
		field.Int32("nillable_int32").
			Optional().
			Nillable(),
		field.Int64("nillable_int64").
			Optional().
			Nillable(),
		field.Int32("validate_optional_int32").
			Optional().
			Max(100),
		field.Uint("optional_uint").
			Optional(),
		field.Uint8("optional_uint8").
			Optional(),
		field.Uint16("optional_uint16").
			Optional(),
		field.Uint32("optional_uint32").
			Optional(),
		field.Uint64("optional_uint64").
			Optional(),
		field.Enum("state").
			Values("on", "off").
			Optional(),
		field.Float("optional_float").
			Optional(),
		field.Float32("optional_float32").
			Optional(),

		// ----------------------------------------------------------------------------
		// Dialect-specific types

		field.Text("text").
			Optional().
			SchemaType(map[string]string{}),
		field.Time("datetime").
			Optional().
			SchemaType(map[string]string{
				dialect.Postgres: "date",
			}),
		field.Float("decimal").
			Optional().
			SchemaType(map[string]string{
				dialect.Postgres: "numeric",
			}),
		field.Other[*Link]("link_other").
			SchemaType(map[string]string{
				dialect.Postgres: "varchar",
				dialect.SQLite:   "varchar(255)",
			}).
			Optional().
			Default(DefaultLink()),
		field.Other[*Link]("link_other_func").
			SchemaType(map[string]string{
				dialect.Postgres: "varchar",
				dialect.SQLite:   "varchar(255)",
			}).
			Optional().
			DefaultFunc(DefaultLink),
		field.StringAs[MAC]("mac").
			Optional().
			SchemaType(map[string]string{
				dialect.Postgres: "macaddr",
			}).
			Validate(func(mac MAC) error {
				_, err := net.ParseMAC(mac.String())
				return err
			}),
		field.Other[Strings]("string_array").
			Optional().
			SchemaType(map[string]string{
				dialect.Postgres: "text[]",
				dialect.SQLite:   "json",
			}),
		field.String("password").
			Optional().
			Sensitive().
			SchemaType(map[string]string{}),

		// ----------------------------------------------------------------------------
		// Custom Go types

		field.StringAs[StringScanner]("string_scanner").
			Nillable().
			Optional(),
		field.Int64As[time.Duration]("duration").
			UpdateDefault(func() time.Duration {
				return time.Duration(100)
			}).
			Optional(),
		field.StringAs[http.Dir]("dir").
			DefaultFunc(func() http.Dir {
				return "unknown"
			}),
		field.StringAs[http.Dir]("ndir").
			Optional().
			Nillable().
			NotEmpty(),
		field.StringAs[sql.NullString]("str").
			Optional().
			DefaultFunc(func() sql.NullString {
				return sql.NullString{String: "default", Valid: true}
			}),
		field.StringAs[*sql.NullString]("null_str").
			Optional().
			Nillable().
			DefaultFunc(func() *sql.NullString {
				return &sql.NullString{String: "default", Valid: true}
			}),
		field.StringAs[Link]("link").
			Optional().
			NotEmpty(),
		field.StringAs[*Link]("null_link").
			Optional().
			Nillable(),
		field.BoolAs[Status]("active").
			Optional(),
		field.BoolAs[Status]("null_active").
			Optional().
			Nillable(),
		field.BoolAs[*sql.NullBool]("deleted").
			Optional().
			Nillable(),
		field.TimeAs[*sql.NullTime]("deleted_at").
			Optional().
			DefaultFunc(func() *sql.NullTime {
				return &sql.NullTime{Time: time.Now(), Valid: true}
			}).
			UpdateDefault(func() *sql.NullTime {
				return &sql.NullTime{Time: time.Now(), Valid: true}
			}),
		field.Bytes("raw_data").
			Optional().
			MaxLen(20).
			MinLen(3),
		field.Bytes("sensitive").
			Optional().
			Sensitive(),
		field.BytesAs[net.IP]("ip").
			Optional().
			DefaultFunc(func() net.IP {
				return net.ParseIP("127.0.0.1")
			}).
			Validate(func(ip net.IP) error {
				if ip.To16() == nil {
					return fmt.Errorf("ent/schema: invalid ip %q", ip)
				}
				return nil
			}),
		field.IntAs[*sql.NullInt64]("null_int64").
			Optional(),
		field.IntAs[Int]("schema_int").
			Optional(),
		field.Int8As[Int8]("schema_int8").
			Optional(),
		field.Int64As[Int64]("schema_int64").
			Optional(),
		field.FloatAs[Float64]("schema_float").
			Optional(),
		field.Float32As[Float32]("schema_float32").
			Optional(),
		field.FloatAs[*sql.NullFloat64]("null_float").
			Optional(),
		field.EnumAs[role.Role]("role").
			Values(role.Role("").Values()...).
			Default(role.Read),
		field.EnumAs[role.Priority]("priority").
			Values(role.Priority(0).Values()...).
			Optional(),
		field.UUID[uuid.UUID]("optional_uuid").
			Optional(),
		field.UUID[uuid.UUID]("nillable_uuid").
			Optional().
			Nillable(),
		field.Strings("strings").
			Optional(),
		field.BytesAs[Pair]("pair").
			DefaultFunc(func() Pair {
				return Pair{K: []byte("K"), V: []byte("V")}
			}),
		field.BytesAs[*Pair]("nil_pair").
			Optional().
			Nillable(),
		field.StringAs[VString]("vstring").
			DefaultFunc(func() VString {
				return "value scanner string"
			}),
		field.StringAs[Triple]("triple").
			DefaultFunc(func() Triple {
				return Triple{E: [3]string{"A", "B", "C"}}
			}),
		field.IntAs[BigInt]("big_int").
			Optional(),
		field.Other[Password]("password_other").
			Optional().
			Sensitive().
			SchemaType(map[string]string{
				dialect.SQLite:   "char(32)",
				dialect.Postgres: "varchar",
			}),
	}
}

type Password string

func (p Password) Value() (driver.Value, error) {
	return string(p), nil
}

func (p *Password) Scan(src any) error {
	switch src := src.(type) {
	case nil:
		return nil
	case string:
		*p = Password(src)
		return nil
	case []byte:
		*p = Password(src)
		return nil
	default:
		return fmt.Errorf("scan: unable to scan type %T into string", src)
	}
}

type Strings []string

func (s *Strings) Scan(v any) (err error) {
	switch v := v.(type) {
	case nil:
	case []byte:
		err = s.scan(string(v))
	case string:
		err = s.scan(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

func (s *Strings) scan(v string) error {
	if v == "" {
		return nil
	}
	if l := len(v); l < 2 || v[0] != '{' && v[l-1] != '}' {
		return fmt.Errorf("unexpected array format %q", v)
	}
	*s = strings.Split(v[1:len(v)-1], ",")
	return nil
}

func (s Strings) Value() (driver.Value, error) {
	return "{" + strings.Join(s, ",") + "}", nil
}

type VString string

func (s *VString) Scan(v any) (err error) {
	switch v := v.(type) {
	case nil:
	case string:
		*s = VString(v)
	case []byte:
		*s = VString(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

func (s VString) Value() (driver.Value, error) {
	return string(s), nil
}

type Triple struct {
	E [3]string
}

// Value implements the driver Valuer interface.
func (t Triple) Value() (driver.Value, error) {
	return fmt.Sprintf("(%s,%s,%s)", t.E[0], t.E[1], t.E[2]), nil
}

// Scan implements the Scanner interface.
func (t *Triple) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case []byte:
		es := strings.Split(strings.TrimPrefix(string(v), "()"), ",")
		t.E[0], t.E[1], t.E[2] = es[0], es[1], es[2]
	case string:
		es := strings.Split(strings.TrimPrefix(v, "()"), ",")
		t.E[0], t.E[1], t.E[2] = es[0], es[1], es[2]
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

type Pair struct {
	K, V []byte
}

// Value implements the driver Valuer interface.
func (p Pair) Value() (driver.Value, error) {
	var b bytes.Buffer
	if err := gob.NewEncoder(&b).Encode(p); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Scan implements the Scanner interface.
func (p *Pair) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case []byte:
		err = gob.NewDecoder(bytes.NewBuffer(v)).Decode(p)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

type (
	Int     int
	Int8    int8
	Int64   int64
	Status  bool
	Float64 float64
	Float32 float32
)

type Link struct {
	*url.URL
}

func DefaultLink() *Link {
	u, _ := url.Parse("127.0.0.1")
	return &Link{URL: u}
}

// Scan implements the Scanner interface.
func (l *Link) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case []byte:
		l.URL, err = url.Parse(string(v))
	case string:
		l.URL, err = url.Parse(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

// Value implements the driver Valuer interface.
func (l Link) Value() (driver.Value, error) {
	if l.URL == nil {
		return nil, nil
	}
	return l.String(), nil
}

type MAC struct {
	net.HardwareAddr
}

// Scan implements the Scanner interface.
func (m *MAC) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case []byte:
		m.HardwareAddr, err = net.ParseMAC(string(v))
	case string:
		m.HardwareAddr, err = net.ParseMAC(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

// Value implements the driver Valuer interface.
func (m MAC) Value() (driver.Value, error) {
	return m.HardwareAddr.String(), nil
}

type StringScanner string

// Scan implements the Scanner interface.
func (s *StringScanner) Scan(value any) (err error) {
	switch v := value.(type) {
	case nil:
	case string:
		*s = StringScanner(v)
	default:
		err = fmt.Errorf("unexpected type %T", v)
	}
	return
}

// Value implements the driver Valuer interface.
func (s StringScanner) Value() (driver.Value, error) {
	return string(s), nil
}

type BigInt struct {
	*big.Int
}

func NewBigInt(i int64) BigInt {
	return BigInt{Int: big.NewInt(i)}
}

func (b *BigInt) Scan(src any) error {
	var i sql.NullString
	if err := i.Scan(src); err != nil {
		return err
	}
	if !i.Valid {
		return nil
	}
	if b.Int == nil {
		b.Int = big.NewInt(0)
	}
	// Value came in a floating point format.
	if strings.ContainsAny(i.String, ".+e") {
		f := big.NewFloat(0)
		if _, err := fmt.Sscan(i.String, f); err != nil {
			return err
		}
		b.Int, _ = f.Int(b.Int)
	} else if _, err := fmt.Sscan(i.String, b.Int); err != nil {
		return err
	}
	return nil
}

func (b BigInt) Value() (driver.Value, error) {
	return b.String(), nil
}

func (b BigInt) Add(c BigInt) BigInt {
	b.Int = b.Int.Add(b.Int, c.Int)
	return b
}
