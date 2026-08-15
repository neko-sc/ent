// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package field_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/neko-sc/ent/dialect"
	"github.com/neko-sc/ent/schema/field"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInt(t *testing.T) {
	fd := field.Int("age").
		Positive().
		Comment("comment").
		Descriptor()
	assert.Equal(t, "age", fd.Name)
	assert.Equal(t, field.TypeInt, fd.Type)
	assert.Len(t, fd.Validators, 1)
	assert.Equal(t, "comment", fd.Comment)

	fd = field.Int("age").
		Default(10).
		Min(10).
		Max(20).
		Descriptor()
	assert.NotNil(t, fd.Default)
	assert.Equal(t, 10, fd.Default)
	assert.Len(t, fd.Validators, 2)

	fd = field.Int("age").
		Range(20, 40).
		Nillable().
		SchemaType(map[string]string{
			dialect.SQLite:   "numeric",
			dialect.Postgres: "int_type",
		}).
		Descriptor()
	assert.Nil(t, fd.Default)
	assert.True(t, fd.Nillable)
	assert.False(t, fd.Immutable)
	assert.Len(t, fd.Validators, 1)
	assert.Equal(t, "numeric", fd.SchemaType[dialect.SQLite])
	assert.Equal(t, "int_type", fd.SchemaType[dialect.Postgres])

	assert.Equal(t, field.TypeInt8, field.Int8("age").Descriptor().Type)
	assert.Equal(t, field.TypeInt16, field.Int16("age").Descriptor().Type)
	assert.Equal(t, field.TypeInt32, field.Int32("age").Descriptor().Type)
	assert.Equal(t, field.TypeInt64, field.Int64("age").Descriptor().Type)
	assert.Equal(t, field.TypeUint, field.Uint("age").Descriptor().Type)
	assert.Equal(t, field.TypeUint8, field.Uint8("age").Descriptor().Type)
	assert.Equal(t, field.TypeUint16, field.Uint16("age").Descriptor().Type)
	assert.Equal(t, field.TypeUint32, field.Uint32("age").Descriptor().Type)
	assert.Equal(t, field.TypeUint64, field.Uint64("age").Descriptor().Type)
}

func TestInt_DefaultFunc(t *testing.T) {
	type CustomInt int

	f1 := func() CustomInt { return 1000 }
	fd := field.IntAs[CustomInt]("id").DefaultFunc(f1).Descriptor()
	assert.NoError(t, fd.Err)

	f2 := func() int { return 1000 }
	fd = field.IntAs[CustomInt]("dir").DefaultFunc(func() CustomInt { return CustomInt(f2()) }).Descriptor()
	assert.NoError(t, fd.Err)

	fd = field.Int("id").DefaultFunc(f2).UpdateDefault(f2).Descriptor()
	assert.NoError(t, fd.Err)
	assert.NotNil(t, fd.Default)
	assert.NotNil(t, fd.UpdateDefault)
}

func TestFloat(t *testing.T) {
	f := field.Float("age").Comment("comment").Positive()
	fd := f.Descriptor()
	assert.Equal(t, "age", fd.Name)
	assert.Equal(t, field.TypeFloat64, fd.Type)
	assert.Len(t, fd.Validators, 1)
	assert.Equal(t, "comment", fd.Comment)

	f = field.Float("age").Min(2.5).Max(5)
	fd = f.Descriptor()
	assert.Len(t, fd.Validators, 2)
	assert.Equal(t, field.TypeFloat32, field.Float32("age").Descriptor().Type)
}

func TestFloat_DefaultFunc(t *testing.T) {
	type CustomFloat float64

	f1 := func() CustomFloat { return 1.2 }
	fd := field.FloatAs[CustomFloat]("weight").DefaultFunc(f1).Descriptor()
	assert.NoError(t, fd.Err)

	f2 := func() float64 { return 1000 }
	fd = field.FloatAs[CustomFloat]("weight").DefaultFunc(func() CustomFloat { return CustomFloat(f2()) }).Descriptor()
	assert.NoError(t, fd.Err)

	fd = field.Float("weight").DefaultFunc(f2).UpdateDefault(f2).Descriptor()
	assert.NoError(t, fd.Err)
	assert.NotNil(t, fd.Default)
	assert.NotNil(t, fd.UpdateDefault)

	f3 := func() float64 { return 1.2 }
	fd = field.Float("weight").DefaultFunc(f3).Descriptor()
	assert.NoError(t, fd.Err)
}

func TestBool(t *testing.T) {
	fd := field.Bool("active").Default(true).Comment("comment").Immutable().Descriptor()
	assert.Equal(t, "active", fd.Name)
	assert.Equal(t, field.TypeBool, fd.Type)
	assert.NotNil(t, fd.Default)
	assert.True(t, fd.Immutable)
	assert.Equal(t, true, fd.Default)
	assert.Equal(t, "comment", fd.Comment)
}

type Pair struct {
	K, V []byte
}

func (*Pair) Scan(any) error              { return nil }
func (Pair) Value() (driver.Value, error) { return nil, nil }

func TestBytes(t *testing.T) {
	fd := field.Bytes("active").
		Unique().
		Default([]byte("{}")).
		Comment("comment").
		Validate(func(bytes []byte) error {
			return nil
		}).
		MaxLen(50).
		Descriptor()
	assert.Equal(t, "active", fd.Name)
	assert.True(t, fd.Unique)
	assert.Equal(t, field.TypeBytes, fd.Type)
	assert.NotNil(t, fd.Default)
	assert.Equal(t, []byte("{}"), fd.Default)
	assert.Equal(t, "comment", fd.Comment)
	assert.Len(t, fd.Validators, 2)
}

func TestBytes_DefaultFunc(t *testing.T) {
	f1 := func() net.IP { return net.IP("0.0.0.0") }
	fd := field.BytesAs[net.IP]("ip").DefaultFunc(f1).Descriptor()
	assert.NoError(t, fd.Err)

	f2 := func() []byte { return []byte("0.0.0.0") }
	var _ net.IP = f2()
	fd = field.BytesAs[net.IP]("ip").DefaultFunc(func() net.IP { return net.IP(f2()) }).Descriptor()
	assert.NoError(t, fd.Err)

	f3 := func() []uint8 { return []uint8("0.0.0.0") }
	var _ net.IP = f3()
	fd = field.BytesAs[net.IP]("ip").DefaultFunc(func() net.IP { return net.IP(f3()) }).Descriptor()
	assert.NoError(t, fd.Err)
	fd = field.Bytes("ip").DefaultFunc(f3).Descriptor()
	assert.NoError(t, fd.Err)
}

type nullBytes []byte

func (b *nullBytes) Scan(v any) error {
	if v == nil {
		return nil
	}
	switch v := v.(type) {
	case []byte:
		*b = v
		return nil
	case string:
		*b = []byte(v)
		return nil
	default:
		return errors.New("unexpected type")
	}
}

func (b nullBytes) Value() (driver.Value, error) { return b, nil }

func TestBytes_ValueScanner(t *testing.T) {
	fd := field.Bytes("dir").
		Codec(field.ValueScannerFunc[[]byte, *nullBytes]{
			V: func(s []byte) (driver.Value, error) {
				return []byte(hex.EncodeToString(s)), nil
			},
			S: func(ns *nullBytes) ([]byte, error) {
				if ns == nil {
					return nil, nil
				}
				b, err := hex.DecodeString(string(*ns))
				if err != nil {
					return nil, err
				}
				return b, nil
			},
		}).Descriptor()
	require.NoError(t, fd.Err)
	require.NotNil(t, fd.ValueScanner)
	_, ok := fd.ValueScanner.(field.ValueScannerFunc[[]byte, *nullBytes])
	require.True(t, ok)

	fd = field.BytesAs[*url.URL]("url").
		Codec(field.BinaryValueScanner[*url.URL]{}).
		Descriptor()
	require.NoError(t, fd.Err)
	require.NotNil(t, fd.ValueScanner)
	_, ok = fd.ValueScanner.(field.TypeValueScanner[*url.URL])
	require.True(t, ok)
}

func TestString_DefaultFunc(t *testing.T) {
	f1 := func() http.Dir { return "/tmp" }
	fd := field.StringAs[http.Dir]("dir").DefaultFunc(f1).Descriptor()
	assert.NoError(t, fd.Err)

	f2 := func() string { return "/tmp" }
	fd = field.StringAs[http.Dir]("dir").DefaultFunc(func() http.Dir { return http.Dir(f2()) }).Descriptor()
	assert.NoError(t, fd.Err)

	f3 := func() sql.NullString { return sql.NullString{} }
	fd = field.StringAs[sql.NullString]("str").DefaultFunc(f3).Descriptor()
	assert.NoError(t, fd.Err)
}

func TestString_ValueScanner(t *testing.T) {
	fd := field.String("dir").
		Codec(field.ValueScannerFunc[string, *sql.NullString]{
			V: func(s string) (driver.Value, error) {
				return base64.StdEncoding.EncodeToString([]byte(s)), nil
			},
			S: func(ns *sql.NullString) (string, error) {
				if !ns.Valid {
					return "", nil
				}
				b, err := base64.StdEncoding.DecodeString(ns.String)
				if err != nil {
					return "", err
				}
				return string(b), nil
			},
		}).Descriptor()
	require.NoError(t, fd.Err)
	require.NotNil(t, fd.ValueScanner)
	_, ok := fd.ValueScanner.(field.TypeValueScanner[string])
	require.True(t, ok)

	fd = field.StringAs[*url.URL]("url").
		Codec(field.BinaryValueScanner[*url.URL]{}).
		Descriptor()
	require.NoError(t, fd.Err)
	require.NotNil(t, fd.ValueScanner)
	_, ok = fd.ValueScanner.(field.TypeValueScanner[*url.URL])
	require.True(t, ok)
}

func TestSlices(t *testing.T) {
	fd := field.Strings("strings").
		Default([]string{}).
		Comment("comment").
		Validate(func(xs []string) error {
			return nil
		}).
		Descriptor()
	assert.Equal(t, "strings", fd.Name)
	assert.Equal(t, field.TypeJSON, fd.Type)
	assert.NotNil(t, fd.Default)
	assert.Equal(t, []string{}, fd.Default)
	assert.Equal(t, "comment", fd.Comment)
	assert.Len(t, fd.Validators, 1)

	fd = field.Ints("ints").
		Default([]int{}).
		Comment("comment").
		Validate(func(xs []int) error {
			return nil
		}).
		Descriptor()
	assert.Equal(t, "ints", fd.Name)
	assert.Equal(t, field.TypeJSON, fd.Type)
	assert.NotNil(t, fd.Default)
	assert.Equal(t, []int{}, fd.Default)
	assert.Equal(t, "comment", fd.Comment)
	assert.Len(t, fd.Validators, 1)

	fd = field.Floats("floats").
		Default([]float64{}).
		Comment("comment").
		Validate(func(xs []float64) error {
			return nil
		}).
		Descriptor()
	assert.Equal(t, "floats", fd.Name)
	assert.Equal(t, field.TypeJSON, fd.Type)
	assert.NotNil(t, fd.Default)
	assert.Equal(t, []float64{}, fd.Default)
	assert.Equal(t, "comment", fd.Comment)
	assert.Len(t, fd.Validators, 1)
}

type VString string

func (s *VString) Scan(any) error {
	return nil
}

func (s VString) Value() (driver.Value, error) {
	return "", nil
}

func TestString(t *testing.T) {
	fd := field.String("name").
		DefaultFunc(func() string {
			return "Ent"
		}).
		Comment("comment").
		Descriptor()

	assert.Equal(t, "name", fd.Name)
	assert.Equal(t, field.TypeString, fd.Type)
	assert.Equal(t, "Ent", fd.Default.(func() string)())
	assert.Equal(t, "comment", fd.Comment)

	re := regexp.MustCompile("[a-zA-Z0-9]")
	f := field.String("name").Unique().Match(re).Validate(func(string) error { return nil }).Sensitive()
	fd = f.Descriptor()
	assert.Equal(t, field.TypeString, fd.Type)
	assert.Equal(t, "name", fd.Name)
	assert.True(t, fd.Unique)
	assert.Len(t, fd.Validators, 2)
	assert.True(t, fd.Sensitive)
}

func TestTime(t *testing.T) {
	now := time.Now()
	fd := field.Time("created_at").
		DefaultFunc(func() time.Time {
			return now
		}).
		Comment("comment").
		Descriptor()
	assert.Equal(t, "created_at", fd.Name)
	assert.Equal(t, field.TypeTime, fd.Type)
	assert.Equal(t, "time.Time", fd.Type.String())
	assert.NotNil(t, fd.Default)
	assert.Equal(t, now, fd.Default.(func() time.Time)())
	assert.Equal(t, "comment", fd.Comment)

	fd = field.Time("updated_at").
		UpdateDefault(func() time.Time {
			return now
		}).
		Descriptor()
	assert.Equal(t, "updated_at", fd.Name)
	assert.Equal(t, now, fd.UpdateDefault.(func() time.Time)())
}

func TestJSON(t *testing.T) {
	descriptor := field.JSON[map[string]string]("name").
		Optional().
		Comment("comment").
		Descriptor()
	assert.True(t, descriptor.Optional)
	assert.Equal(t, "name", descriptor.Name)
	assert.Equal(t, field.TypeJSON, descriptor.Type)
	assert.Equal(t, "comment", descriptor.Comment)
	assert.Equal(t, reflect.TypeFor[map[string]string](), descriptor.RuntimeType)

	descriptor = field.JSON[[]http.Dir]("dirs").
		Default([]http.Dir{"a", "b"}).
		Descriptor()
	assert.NoError(t, descriptor.Err)
	assert.Equal(t, []http.Dir{"a", "b"}, descriptor.Default)

	descriptor = field.JSON[[]http.Dir]("dirs").
		DefaultFunc(func() []http.Dir { return []http.Dir{"/tmp"} }).
		Descriptor()
	assert.NoError(t, descriptor.Err)
	assert.Equal(t, []http.Dir{"/tmp"}, descriptor.Default.(func() []http.Dir)())

	descriptor = field.JSON[map[string]*url.Values]("values").Descriptor()
	assert.Equal(t, reflect.TypeFor[map[string]*url.Values](), descriptor.RuntimeType)
}

func TestField_Tag(t *testing.T) {
	fd := field.Bool("expired").
		StructTag(`json:"expired,omitempty"`).
		Descriptor()
	assert.Equal(t, `json:"expired,omitempty"`, fd.Tag)
}

type Role string

func (Role) Values() []string {
	return []string{"admin", "owner"}
}

type RoleInt int32

func (RoleInt) Values() []string {
	return []string{"unknown", "admin", "owner"}
}

func (i RoleInt) String() string {
	switch i {
	case 1:
		return "admin"
	case 2:
		return "owner"
	default:
		return "unknown"
	}
}

func (i RoleInt) Value() (driver.Value, error) {
	return i.String(), nil
}

func (i *RoleInt) Scan(val any) error {
	switch v := val.(type) {
	case string:
		switch v {
		case "admin":
			*i = 1
		case "owner":
			*i = 2
		default:
			*i = 0
		}
	default:
		return errors.New("bad enum value")
	}

	return nil
}

func TestField_Enums(t *testing.T) {
	fd := field.Enum("role").
		Values(
			"user",
			"admin",
			"master",
		).
		Default("user").
		Comment("comment").
		Descriptor()
	assert.Equal(t, "role", fd.Name)
	assert.Equal(t, "user", fd.Enums[0].V)
	assert.Equal(t, "admin", fd.Enums[1].V)
	assert.Equal(t, "master", fd.Enums[2].V)
	assert.Equal(t, "user", fd.Default)
	assert.Equal(t, "comment", fd.Comment)

	fd = field.Enum("role").
		NamedValues("USER", "user").
		Default("user").
		Descriptor()
	assert.Equal(t, "role", fd.Name)
	assert.Equal(t, "USER", fd.Enums[0].N)
	assert.Equal(t, "user", fd.Enums[0].V)
	assert.Equal(t, "user", fd.Default)

	fd = field.EnumAs[Role]("role").Values("admin", "owner").Descriptor()
	assert.NoError(t, fd.Err)
	assert.Equal(t, reflect.TypeFor[Role](), fd.RuntimeType)
	assert.Equal(t, "admin", fd.Enums[0].V)
	assert.Equal(t, "owner", fd.Enums[1].V)

	fd = field.EnumAs[RoleInt]("role").Values("unknown", "admin", "owner").Descriptor()
	assert.Equal(t, reflect.TypeFor[RoleInt](), fd.RuntimeType)
	assert.Equal(t, "unknown", fd.Enums[0].V)
	assert.Equal(t, "admin", fd.Enums[1].V)
	assert.Equal(t, "owner", fd.Enums[2].V)
}

func TestField_EnumValuesFrom(t *testing.T) {
	tests := []struct {
		name   string
		values field.EnumValues
		want   []string
	}{
		{name: "string representation", values: Role(""), want: []string{"admin", "owner"}},
		{name: "integer representation", values: RoleInt(0), want: []string{"unknown", "admin", "owner"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			descriptor := field.Enum("role").ValuesFrom(tt.values).Descriptor()
			require.Len(t, descriptor.Enums, len(tt.want))
			for index, value := range tt.want {
				assert.Equal(t, value, descriptor.Enums[index].N)
				assert.Equal(t, value, descriptor.Enums[index].V)
			}
		})
	}
}

func TestField_UUID(t *testing.T) {
	fd := field.UUID[uuid.UUID]("id").
		Unique().
		DefaultFunc(uuid.New).
		Comment("comment").
		Nillable().
		Descriptor()
	assert.Equal(t, "id", fd.Name)
	assert.True(t, fd.Unique)
	assert.Equal(t, reflect.TypeFor[uuid.UUID](), fd.RuntimeType)
	assert.NotNil(t, fd.Default)
	assert.NotEmpty(t, fd.Default.(func() uuid.UUID)())
	assert.Equal(t, "comment", fd.Comment)
	assert.True(t, fd.Nillable)

	fd = field.UUID[*uuid.UUID]("id").Descriptor()
	assert.Equal(t, reflect.TypeFor[*uuid.UUID](), fd.RuntimeType)
}

type custom struct {
}

func (c *custom) Scan(_ any) (err error) {
	return nil
}

func (c custom) Value() (driver.Value, error) {
	return nil, nil
}

func TestField_Other(t *testing.T) {
	fd := field.Other[*custom]("other").
		Unique().
		Default(&custom{}).
		SchemaType(map[string]string{dialect.Postgres: "varchar"}).
		Descriptor()
	assert.NoError(t, fd.Err)
	assert.Equal(t, "other", fd.Name)
	assert.True(t, fd.Unique)
	assert.Equal(t, reflect.TypeFor[*custom](), fd.RuntimeType)
	assert.NotNil(t, fd.Default)

	fd = field.Other[*custom]("other").
		Descriptor()
	assert.Error(t, fd.Err, "missing SchemaType option")

	fd = field.Other[*custom]("other").
		SchemaType(map[string]string{dialect.Postgres: "varchar"}).
		DefaultFunc(func() *custom { return &custom{} }).
		Descriptor()
	assert.NoError(t, fd.Err)

	fd = field.Other[custom]("other").
		SchemaType(map[string]string{dialect.Postgres: "varchar"}).
		DefaultFunc(func() custom { return custom{} }).
		Descriptor()
	assert.NoError(t, fd.Err)
}

type UserRole string

const (
	Admin   UserRole = "ADMIN"
	User    UserRole = "USER"
	Unknown UserRole = "UNKNOWN"
)

func (UserRole) Values() (roles []string) {
	for _, r := range []UserRole{Admin, User, Unknown} {
		roles = append(roles, string(r))
	}
	return
}

func (e UserRole) String() string {
	return string(e)
}

// MarshalGQL implements graphql.Marshaler interface.
func (e UserRole) MarshalGQL(w io.Writer) {
	_, _ = io.WriteString(w, strconv.Quote(e.String()))
}

// UnmarshalGQL implements graphql.Unmarshaler interface.
func (e *UserRole) UnmarshalGQL(val any) error {
	str, ok := val.(string)
	if !ok {
		return fmt.Errorf("enum %T must be a string", val)
	}
	*e = UserRole(str)
	switch *e {
	case Admin, User, Unknown:
		return nil
	default:
		return fmt.Errorf("%s is not a valid Role", str)
	}
}

type Scalar struct{}

func (Scalar) MarshalGQL(io.Writer)         {}
func (*Scalar) UnmarshalGQL(any) error      { return nil }
func (Scalar) Value() (driver.Value, error) { return nil, nil }

func TestRuntimeType_Implements(t *testing.T) {
	type (
		marshaler   interface{ MarshalGQL(w io.Writer) }
		unmarshaler interface{ UnmarshalGQL(v any) error }
		codec       interface {
			marshaler
			unmarshaler
		}
	)
	var (
		codecType     = reflect.TypeFor[codec]()
		marshalType   = reflect.TypeFor[marshaler]()
		unmarshalType = reflect.TypeFor[unmarshaler]()
	)
	for _, typ := range []reflect.Type{
		field.EnumAs[UserRole]("role").Descriptor().RuntimeType,
		field.Other[*Scalar]("scalar").SchemaType(map[string]string{dialect.SQLite: "blob"}).Descriptor().RuntimeType,
		field.Other[Scalar]("scalar").SchemaType(map[string]string{dialect.SQLite: "blob"}).Descriptor().RuntimeType,
	} {
		assert.True(t, typ.Implements(codecType) || reflect.PointerTo(typ).Implements(codecType))
		assert.True(t, typ.Implements(marshalType) || reflect.PointerTo(typ).Implements(marshalType))
		assert.True(t, typ.Implements(unmarshalType) || reflect.PointerTo(typ).Implements(unmarshalType))
	}
}

func TestTypeString(t *testing.T) {
	typ := field.TypeBool
	assert.Equal(t, "bool", typ.String())
	typ = field.TypeInvalid
	assert.Equal(t, "invalid", typ.String())
	typ = 21
	assert.Equal(t, "invalid", typ.String())
}

func TestTypeNumeric(t *testing.T) {
	typ := field.TypeBool
	assert.False(t, typ.Numeric())
	typ = field.TypeUint8
	assert.True(t, typ.Numeric())
}

func TestTypeValid(t *testing.T) {
	typ := field.TypeBool
	assert.True(t, typ.Valid())
	typ = 0
	assert.False(t, typ.Valid())
	typ = 21
	assert.False(t, typ.Valid())
}

func TestTypeConstName(t *testing.T) {
	typ := field.TypeJSON
	assert.Equal(t, "TypeJSON", typ.ConstName())
	typ = field.TypeInt
	assert.Equal(t, "TypeInt", typ.ConstName())
	typ = field.TypeInt64
	assert.Equal(t, "TypeInt64", typ.ConstName())
	typ = field.TypeOther
	assert.Equal(t, "TypeOther", typ.ConstName())
	typ = 21
	assert.Equal(t, "invalid", typ.ConstName())
}

func TestString_MinRuneLen(t *testing.T) {
	fd := field.String("name").MinRuneLen(5).Descriptor()
	assert.Len(t, fd.Validators, 1)

	err := fd.Validators[0].(func(string) error)("hello")
	assert.NoError(t, err)

	err = fd.Validators[0].(func(string) error)("hi")
	assert.EqualError(t, err, "value is less than the required rune length")

	err = fd.Validators[0].(func(string) error)("你好")
	assert.EqualError(t, err, "value is less than the required rune length")

	err = fd.Validators[0].(func(string) error)("你好世界！")
	assert.NoError(t, err)

	err = fd.Validators[0].(func(string) error)("")
	assert.Error(t, err)
}

func TestString_MaxRuneLen(t *testing.T) {
	fd := field.String("name").MaxRuneLen(5).Descriptor()
	assert.Len(t, fd.Validators, 1)

	err := fd.Validators[0].(func(string) error)("hello")
	assert.NoError(t, err)

	err = fd.Validators[0].(func(string) error)("hello world")
	assert.EqualError(t, err, "value is greater than the required rune length")

	err = fd.Validators[0].(func(string) error)("你好世界你好")
	assert.EqualError(t, err, "value is greater than the required rune length")

	err = fd.Validators[0].(func(string) error)("你好世界！")
	assert.NoError(t, err)

	err = fd.Validators[0].(func(string) error)("")
	assert.NoError(t, err)
}
