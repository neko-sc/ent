// Copyright 2019-2026 Facebook Inc.
// SPDX-License-Identifier: Apache-2.0

package load

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/neko-sc/ent"
	"github.com/neko-sc/ent/schema"
	"github.com/neko-sc/ent/schema/edge"
	"github.com/neko-sc/ent/schema/field"
	"github.com/neko-sc/ent/schema/index"
	"github.com/neko-sc/ent/schema/mixin"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func marshalSchema(t *testing.T, schema ent.Interface) ([]byte, error) {
	t.Helper()
	return MarshalSchema(schema, func(_ reflect.Type, logical field.Type) (*FieldType, error) {
		semantic, err := FieldTypeOf(logical, logicalTypeExpression(logical))
		if err != nil {
			return nil, err
		}
		semantic.Capabilities.AssignableToLogical = true
		semantic.Capabilities.ConvertibleToLogical = true
		semantic.Capabilities.LogicalReverseConvertible = true
		return semantic, nil
	})
}

type OrderConfig struct {
	FieldName string
}

func (OrderConfig) Name() string {
	return "order_config"
}

func (o OrderConfig) Merge(ant schema.Annotation) schema.Annotation {
	o.FieldName = ant.(OrderConfig).FieldName
	return o
}

type IDConfig struct {
	TagName string
}

func (IDConfig) Name() string {
	return "id_config"
}

type PartialIndex struct {
	WhereClause string
}

func (PartialIndex) Name() string {
	return "partial_index"
}

func (p PartialIndex) Merge(ant schema.Annotation) schema.Annotation {
	p.WhereClause = ant.(PartialIndex).WhereClause
	return p
}

type AnnotationMixin struct {
	mixin.Schema
}

func (AnnotationMixin) Annotations() []schema.Annotation {
	return []schema.Annotation{
		IDConfig{TagName: "id tag"},
		OrderConfig{FieldName: "mixin annotations"},
	}
}

type User struct {
	ent.Schema
}

func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		AnnotationMixin{},
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		OrderConfig{FieldName: "type annotations"},
	}
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("age").
			Comment("some comment"),
		field.String("name").
			Default("unknown").
			Annotations(&OrderConfig{FieldName: "name"}),
		field.String("nillable").
			Nillable(),
		field.String("optional").
			Optional(),
		field.Enum("state").
			Values("on", "off").
			Optional(),
		field.String("sensitive").
			Sensitive(),
		field.Time("creation_time").
			DefaultFunc(time.Now),
		field.UUID[uuid.UUID]("uuid").
			DefaultFunc(uuid.New),
		field.Int("parent_id").
			Optional(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("groups", Group.Type).
			Annotations(&OrderConfig{FieldName: "name"}),
		edge.To("parent", User.Type).
			Unique().
			Required().
			Immutable().
			Field("parent_id").
			StorageKey(edge.Column("parent_id")).
			From("children"),
		edge.To("following", User.Type).
			Annotations(&OrderConfig{FieldName: "following"}).
			From("followers").
			Annotations(&OrderConfig{FieldName: "followers"}),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "address").
			Unique(),
		index.Fields("name").
			Edges("parent").
			StorageKey("user_parent_name").
			Annotations(&PartialIndex{
				WhereClause: "age > 20",
			}).
			Unique(),
	}
}

type Group struct{ ent.Schema }

func (Group) Fields() []ent.Field { return nil }

func (Group) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("users", User.Type),
	}
}

func TestNewField_RejectsUnsupportedRepresentations(t *testing.T) {
	tests := []struct {
		name         string
		descriptor   *field.Descriptor
		capabilities TypeCapabilities
		error        string
	}{
		{
			name:       "primitive struct",
			descriptor: field.StringAs[struct{}]("value").Descriptor(),
			error:      `field "value": representation cannot round-trip logical string; use a reverse-convertible representation, Scanner and Valuer, or an external Codec`,
		},
		{
			name:       "projection only enum",
			descriptor: field.EnumAs[struct{}]("priority").Values("low", "high").Descriptor(),
			capabilities: TypeCapabilities{
				Stringer:          true,
				LogicalProjection: "String()",
			},
			error: `field "priority": representation cannot round-trip logical string; use a reverse-convertible representation, Scanner and Valuer, or an external Codec`,
		},
		{
			name:       "scanner without valuer",
			descriptor: field.StringAs[struct{}]("value").Descriptor(),
			capabilities: TypeCapabilities{
				Scanner: true,
			},
			error: `field "value": representation must implement both Scanner and Valuer or provide an external Codec`,
		},
		{
			name:       "other without codec",
			descriptor: field.Other[struct{}]("value").SchemaType(map[string]string{"sqlite3": "text"}).Descriptor(),
			error:      `field "value": representation requires Scanner and Valuer or an external Codec`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewField(tt.descriptor, func(_ reflect.Type, logical field.Type) (*FieldType, error) {
				semantic, semanticError := FieldTypeOf(logical, &TypeExpression{Kind: TypeKindStruct})
				if semanticError != nil {
					return nil, semanticError
				}
				semantic.Capabilities = tt.capabilities
				return semantic, nil
			})
			require.EqualError(t, err, tt.error)
		})
	}
}

func TestNewField_AcceptsSupportedRepresentations(t *testing.T) {
	tests := []struct {
		name         string
		descriptor   *field.Descriptor
		capabilities TypeCapabilities
	}{
		{
			name:       "defined scalar",
			descriptor: field.StringAs[typedSchemaString]("value").Descriptor(),
			capabilities: TypeCapabilities{
				ConvertibleToLogical:      true,
				LogicalReverseConvertible: true,
			},
		},
		{
			name:       "byte array projection",
			descriptor: field.BytesAs[[16]byte]("value").Descriptor(),
			capabilities: TypeCapabilities{
				LogicalProjection:         "[:]",
				LogicalReverseConvertible: true,
			},
		},
		{
			name:       "native scanner valuer",
			descriptor: field.StringAs[struct{}]("value").Descriptor(),
			capabilities: TypeCapabilities{
				Scanner: true,
				Valuer:  true,
			},
		},
		{
			name:       "json builtin",
			descriptor: field.JSON[struct{}]("value").Descriptor(),
		},
		{
			name:       "external codec",
			descriptor: field.StringAs[struct{}]("value").Codec(typedSchemaCodec{}).Descriptor(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded, err := NewField(tt.descriptor, func(_ reflect.Type, logical field.Type) (*FieldType, error) {
				semantic, semanticError := FieldTypeOf(logical, &TypeExpression{Kind: TypeKindStruct})
				if semanticError != nil {
					return nil, semanticError
				}
				semantic.Capabilities = tt.capabilities
				return semantic, nil
			})
			require.NoError(t, err)
			require.NotNil(t, loaded)
		})
	}
}

func TestNewField_ValidatorKindsPreserveOrder(t *testing.T) {
	descriptor := field.StringAs[typedSchemaString]("value").
		Validate(func(typedSchemaString) error { return nil }).
		MinLen(1).
		Validate(func(typedSchemaString) error { return nil }).
		Descriptor()
	loaded, err := NewField(descriptor, func(_ reflect.Type, logical field.Type) (*FieldType, error) {
		representation, semanticError := TypeExpressionFor[typedSchemaString]()
		if semanticError != nil {
			return nil, semanticError
		}
		semantic, semanticError := FieldTypeOf(logical, representation)
		if semanticError != nil {
			return nil, semanticError
		}
		semantic.Capabilities.ConvertibleToLogical = true
		semantic.Capabilities.LogicalReverseConvertible = true
		return semantic, nil
	})
	require.NoError(t, err)
	require.Equal(t, []field.ValidatorKind{
		field.ValidatorRepresentation,
		field.ValidatorLogical,
		field.ValidatorRepresentation,
	}, loaded.ValidatorKinds)
}

type typedSchemaString string

type typedSchemaCodec struct{}

func (typedSchemaCodec) Value(struct{}) (driver.Value, error) { return nil, nil }
func (typedSchemaCodec) ScanValue() field.ValueScanner        { return new(sql.NullString) }
func (typedSchemaCodec) FromValue(driver.Value) (struct{}, error) {
	return struct{}{}, nil
}

func TestMarshalSchema(t *testing.T) {
	for _, u := range []ent.Interface{User{}, &User{}} {
		buf, err := marshalSchema(t, u)
		require.NoError(t, err)

		schema, err := UnmarshalSchema(buf)
		require.NoError(t, err)
		require.Equal(t, "User", schema.Name)
		require.Len(t, schema.Annotations, 2)
		ant := schema.Annotations["order_config"].(map[string]any)
		require.Equal(t, "type annotations", ant["FieldName"])

		require.Len(t, schema.Fields, 9)
		require.Equal(t, "age", schema.Fields[0].Name)
		require.Equal(t, field.TypeInt, schema.Fields[0].Type)

		require.Equal(t, "name", schema.Fields[1].Name)
		require.Equal(t, field.TypeString, schema.Fields[1].Type)
		require.Equal(t, "unknown", schema.Fields[1].DefaultValue)
		require.NotEmpty(t, schema.Fields[1].Annotations)
		ant = schema.Fields[1].Annotations["order_config"].(map[string]any)
		require.Equal(t, "name", ant["FieldName"])

		require.Equal(t, "nillable", schema.Fields[2].Name)
		require.Equal(t, field.TypeString, schema.Fields[2].Type)
		require.True(t, schema.Fields[2].Nillable)
		require.False(t, schema.Fields[2].Optional)
		require.False(t, schema.Fields[2].Sensitive)

		require.Equal(t, "optional", schema.Fields[3].Name)
		require.Equal(t, field.TypeString, schema.Fields[3].Type)
		require.False(t, schema.Fields[3].Nillable)
		require.True(t, schema.Fields[3].Optional)

		require.Equal(t, "state", schema.Fields[4].Name)
		require.Equal(t, field.TypeEnum, schema.Fields[4].Type)
		require.Equal(t, "on", schema.Fields[4].Enums[0].V)
		require.Equal(t, "off", schema.Fields[4].Enums[1].V)

		require.Equal(t, "sensitive", schema.Fields[5].Name)
		require.Equal(t, field.TypeString, schema.Fields[5].Type)
		require.True(t, schema.Fields[5].Sensitive)
		require.Equal(t, reflect.Invalid, schema.Fields[5].DefaultKind)

		require.Equal(t, "creation_time", schema.Fields[6].Name)
		require.Equal(t, field.TypeTime, schema.Fields[6].Type)
		require.Nil(t, schema.Fields[6].DefaultValue)
		require.Equal(t, reflect.Func, schema.Fields[6].DefaultKind)

		require.Equal(t, "uuid", schema.Fields[7].Name)
		require.Equal(t, field.TypeUUID, schema.Fields[7].Type)
		require.True(t, schema.Fields[7].Default)

		require.Equal(t, "parent_id", schema.Fields[8].Name)
		require.Equal(t, field.TypeInt, schema.Fields[8].Type)
		require.True(t, schema.Fields[8].Optional)

		require.Len(t, schema.Edges, 3)
		require.Equal(t, "groups", schema.Edges[0].Name)
		require.Equal(t, "Group", schema.Edges[0].Type)
		require.False(t, schema.Edges[0].Inverse)
		require.NotEmpty(t, schema.Edges[0].Annotations)
		ant = schema.Edges[0].Annotations["order_config"].(map[string]any)
		require.Equal(t, "name", ant["FieldName"])

		require.Equal(t, "children", schema.Edges[1].Name)
		require.Equal(t, "parent_id", schema.Edges[1].StorageKey.Columns[0])
		require.Equal(t, "User", schema.Edges[1].Type)
		require.True(t, schema.Edges[1].Inverse)
		require.Equal(t, "parent", schema.Edges[1].Ref.Name)
		require.True(t, schema.Edges[1].Ref.Unique)
		require.True(t, schema.Edges[1].Ref.Required)
		require.True(t, schema.Edges[1].Ref.Immutable)
		require.Equal(t, "parent_id", schema.Edges[1].Ref.StorageKey.Columns[0])

		ant = schema.Edges[2].Annotations["order_config"].(map[string]any)
		require.Equal(t, "followers", ant["FieldName"])
		ant = schema.Edges[2].Ref.Annotations["order_config"].(map[string]any)
		require.Equal(t, "following", ant["FieldName"])

		require.Equal(t, []string{"name", "address"}, schema.Indexes[0].Fields)
		require.True(t, schema.Indexes[0].Unique)
		require.Equal(t, []string{"name"}, schema.Indexes[1].Fields)
		require.Equal(t, []string{"parent"}, schema.Indexes[1].Edges)
		require.Equal(t, "user_parent_name", schema.Indexes[1].StorageKey)
		require.True(t, schema.Indexes[1].Unique)
		ant = schema.Indexes[1].Annotations["partial_index"].(map[string]any)
		require.Equal(t, "age > 20", ant["WhereClause"])

		require.Equal(t, "some comment", schema.Fields[0].Comment)
		require.Empty(t, schema.Fields[1].Comment)
	}
}

type InvalidEdge struct {
	ent.Schema
}

// Edge panics because the edge declaration is invalid.
func (InvalidEdge) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("invalid", InvalidEdge{}.Type),
	}
}

func TestMarshalFails(t *testing.T) {
	buf, err := marshalSchema(t, InvalidEdge{})
	require.Error(t, err)
	require.Nil(t, buf)
}

type WithDefaults struct {
	ent.Schema
}

func (WithDefaults) Fields() []ent.Field {
	return []ent.Field{
		field.Int("int").
			Default(1),
		field.Float("float").
			Default(math.Pi),
		field.String("string").
			Default("foo"),
		field.Bool("string").
			Default(true),
		field.Time("updated_at").
			UpdateDefault(time.Now),
		// see issue #1146
		field.Int("int_default_func").
			DefaultFunc(func() int {
				return 1e9
			}),
		field.Float("balance").
			Default(0),
		field.JSON[[]http.Dir]("dirs").
			Default([]http.Dir{"/tmp"}),
		field.Float("float_default_func").
			DefaultFunc(func() float64 {
				return math.Pi
			}),
	}
}

func (WithDefaults) Edges() []ent.Edge {
	return nil
}

func (WithDefaults) Indexes() []ent.Index {
	return nil
}

func TestMarshalDefaults(t *testing.T) {
	d := WithDefaults{}
	buf, err := marshalSchema(t, d)
	require.NoError(t, err)

	schema := &Schema{}
	err = json.Unmarshal(buf, schema)
	require.NoError(t, err)

	require.Equal(t, "WithDefaults", schema.Name)
	require.True(t, schema.Fields[0].Default)
	require.True(t, schema.Fields[1].Default)
	require.True(t, schema.Fields[2].Default)
	require.True(t, schema.Fields[3].Default)
	require.False(t, schema.Fields[4].Default)
	require.True(t, schema.Fields[4].UpdateDefault)
	require.True(t, schema.Fields[5].Default)
	require.Equal(t, reflect.Func, schema.Fields[5].DefaultKind)
	require.True(t, schema.Fields[6].Default)
	require.True(t, schema.Fields[7].Default)
	require.Equal(t, reflect.Func, schema.Fields[8].DefaultKind)
}

type TimeMixin struct {
	mixin.Schema
}

func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").
			Immutable().
			DefaultFunc(time.Now),
		field.Time("updated_at").
			DefaultFunc(time.Now).
			UpdateDefault(time.Now),
	}
}

type HooksMixin struct {
	mixin.Schema
}

func (HooksMixin) Fields() []ent.Field {
	return []ent.Field{
		field.String("boring"),
	}
}

func (HooksMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Unique(),
	}
}

func (HooksMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("boring").
			Edges("user"),
	}
}

func (HooksMixin) Hooks() []ent.Hook {
	return []ent.Hook{
		func(ent.Mutator) ent.Mutator { return nil },
		func(ent.Mutator) ent.Mutator { return nil },
	}
}

type BoringPolicy struct{}

func (BoringPolicy) EvalMutation(context.Context, ent.Mutation) error { return nil }
func (BoringPolicy) EvalQuery(context.Context, ent.Query) error       { return nil }

type PrivacyMixin struct {
	mixin.Schema
}

func (PrivacyMixin) Policy() ent.Policy {
	return BoringPolicy{}
}

type WithMixin struct {
	ent.Schema
}

func (WithMixin) Mixin() []ent.Mixin {
	return []ent.Mixin{
		TimeMixin{},
		HooksMixin{},
		PrivacyMixin{},
	}
}

func (WithMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Int("field"),
	}
}

func (WithMixin) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("owner", User.Type),
	}
}

func (WithMixin) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("field").
			Edges("owner").
			Unique(),
	}
}

func (WithMixin) Hooks() []ent.Hook {
	return []ent.Hook{
		func(ent.Mutator) ent.Mutator { return nil },
	}
}

func (WithMixin) Policy() ent.Policy {
	return BoringPolicy{}
}

func TestMarshalMixin(t *testing.T) {
	d := WithMixin{}
	buf, err := marshalSchema(t, d)
	require.NoError(t, err)

	schema := &Schema{}
	err = json.Unmarshal(buf, schema)
	require.NoError(t, err)

	t.Run("Fields", func(t *testing.T) {
		require.Equal(t, "WithMixin", schema.Name)
		require.Equal(t, "created_at", schema.Fields[0].Name)
		require.True(t, schema.Fields[0].Default)
		require.True(t, schema.Fields[0].Position.MixedIn)
		require.Equal(t, 0, schema.Fields[0].Position.MixinIndex)
		require.Equal(t, 0, schema.Fields[0].Position.Index)

		require.Equal(t, "updated_at", schema.Fields[1].Name)
		require.True(t, schema.Fields[1].Default)
		require.True(t, schema.Fields[1].UpdateDefault)
		require.True(t, schema.Fields[1].Position.MixedIn)
		require.Equal(t, 0, schema.Fields[1].Position.MixinIndex)
		require.Equal(t, 1, schema.Fields[1].Position.Index)

		require.Equal(t, "boring", schema.Fields[2].Name)
		require.False(t, schema.Fields[2].Default)
		require.False(t, schema.Fields[2].UpdateDefault)
		require.True(t, schema.Fields[2].Position.MixedIn)
		require.Equal(t, 1, schema.Fields[2].Position.MixinIndex)
		require.Equal(t, 0, schema.Fields[2].Position.Index)

		require.Equal(t, "field", schema.Fields[3].Name)
		require.False(t, schema.Fields[3].Default)
		require.False(t, schema.Fields[3].Position.MixedIn)
		require.Equal(t, 0, schema.Fields[3].Position.Index)
	})

	t.Run("Hooks", func(t *testing.T) {
		require.True(t, schema.Hooks[0].MixedIn)
		require.True(t, schema.Hooks[1].MixedIn)

		require.Equal(t, 1, schema.Hooks[0].MixinIndex)
		require.Equal(t, 1, schema.Hooks[1].MixinIndex)
		require.Equal(t, 0, schema.Hooks[0].Index)
		require.Equal(t, 1, schema.Hooks[1].Index)

		require.False(t, schema.Hooks[2].MixedIn)
		require.Equal(t, 0, schema.Hooks[2].Index)
		require.Equal(t, 0, schema.Hooks[2].MixinIndex)
	})

	t.Run("Edges", func(t *testing.T) {
		require.Len(t, schema.Edges, 2)
		require.Equal(t, "user", schema.Edges[0].Name)
		require.Equal(t, "User", schema.Edges[0].Type)
		require.True(t, schema.Edges[0].Unique)

		require.Equal(t, "owner", schema.Edges[1].Name)
		require.Equal(t, "User", schema.Edges[1].Type)
		require.False(t, schema.Edges[1].Unique)
	})

	t.Run("Indexes", func(t *testing.T) {
		require.Len(t, schema.Indexes, 2)
		require.Equal(t, []string{"boring"}, schema.Indexes[0].Fields)
		require.Equal(t, []string{"user"}, schema.Indexes[0].Edges)
		require.False(t, schema.Indexes[0].Unique)

		require.Equal(t, []string{"field"}, schema.Indexes[1].Fields)
		require.Equal(t, []string{"owner"}, schema.Indexes[1].Edges)
		require.True(t, schema.Indexes[1].Unique)
	})

	t.Run("Policy", func(t *testing.T) {
		require.Len(t, schema.Policy, 2)
		require.True(t, schema.Policy[0].MixedIn)
		require.False(t, schema.Policy[1].MixedIn)
	})
}
