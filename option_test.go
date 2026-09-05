package ent_test

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"testing"

	"github.com/neko-sc/ent"
	"github.com/stretchr/testify/require"
)

func TestOptionJSON(t *testing.T) {
	type document struct {
		A ent.Option[int] `json:"a,omitzero"`
	}
	for _, test := range []struct {
		name    string
		option  ent.Option[int]
		encoded string
	}{
		{"unset", ent.Unset[int](), `{}`},
		{"null", ent.Null[int](), `{"a":null}`},
		{"zero", ent.Some(0), `{"a":0}`},
		{"value", ent.Some(1), `{"a":1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(document{A: test.option})
			require.NoError(t, err)
			require.JSONEq(t, test.encoded, string(encoded))
			var decoded document
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, test.option, decoded.A)
			encoded, err = jsonv2.Marshal(document{A: test.option})
			require.NoError(t, err)
			require.JSONEq(t, test.encoded, string(encoded))
			require.NoError(t, jsonv2.Unmarshal(encoded, &decoded))
			require.Equal(t, test.option, decoded.A)
		})
	}
}

func TestOptionStates(t *testing.T) {
	for _, test := range []struct {
		name                      string
		option                    ent.Option[int]
		set, null, unset, present bool
		value                     int
		debug                     string
	}{
		{"zero value", ent.Option[int]{}, false, false, true, false, 0, "Unset"},
		{"null", ent.Null[int](), true, true, false, false, 0, "Null"},
		{"value", ent.Some(7), true, false, false, true, 7, "Some(7)"},
		{"zero", ent.Some(0), true, false, false, true, 0, "Some(0)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.set, test.option.IsSet())
			require.Equal(t, test.null, test.option.IsNull())
			require.Equal(t, test.unset, test.option.IsUnset())
			require.Equal(t, test.unset, test.option.IsZero())
			value, present := test.option.Get()
			require.Equal(t, test.value, value)
			require.Equal(t, test.present, present)
			require.Equal(t, test.debug, test.option.String())
			if test.present {
				require.Equal(t, test.value, test.option.MustGet())
				require.Equal(t, test.value, *test.option.Ptr())
				require.Equal(t, test.value, test.option.Or(99))
			} else {
				require.Panics(t, func() { test.option.MustGet() })
				require.Nil(t, test.option.Ptr())
				require.Equal(t, 99, test.option.Or(99))
				encoded, err := json.Marshal(test.option)
				require.NoError(t, err)
				require.Equal(t, "null", string(encoded))
			}
		})
	}
}

func TestOptionPointersAndDecodeErrors(t *testing.T) {
	require.True(t, ent.FromPtr[int](nil).IsUnset())
	require.True(t, ent.NullIfNil[int](nil).IsNull())
	value := 4
	option := ent.FromPtr(&value)
	require.Equal(t, ent.Some(4), ent.NullIfNil(&value))
	value = 8
	require.Equal(t, 4, option.MustGet())
	*option.Ptr() = 9
	require.Equal(t, 4, option.MustGet())
	require.Error(t, json.Unmarshal([]byte(`"invalid"`), &option))
	require.Equal(t, ent.Some(4), option)
	require.NoError(t, json.Unmarshal([]byte(` null `), &option))
	require.True(t, option.IsNull())
	require.NoError(t, json.Unmarshal([]byte(`3`), &option))
	require.Equal(t, ent.Some(3), option)
	_, err := json.Marshal(ent.Some(make(chan int)))
	require.Error(t, err)
	require.True(t, ent.Some[*int](nil).IsSet())
	require.False(t, ent.Some[*int](nil).IsNull())
}
