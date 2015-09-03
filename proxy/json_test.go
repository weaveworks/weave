package proxy

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLookupObject(t *testing.T) {
	tests := []struct {
		root   jsonObject
		key    string
		result jsonObject
		err    error
	}{
		{
			jsonObject{},
			"a",
			jsonObject{},
			nil,
		},
		{
			jsonObject{"a": map[string]interface{}{"b": int(1)}},
			"a",
			jsonObject{"b": int(1)},
			nil,
		},
		{
			jsonObject{"nonObject": int(1)},
			"nonObject",
			nil,
			&UnmarshalWrongTypeError{Field: "nonObject", Expected: "object", Got: 1},
		},
	}
	for _, test := range tests {
		gotResult, gotErr := test.root.Object(test.key)
		msg := fmt.Sprintf("%q.Object(%q) => %q, %q", test.root, test.key, gotResult, gotErr)
		assert.Equal(t, gotResult, test.result, msg)
		assert.Equal(t, gotErr, test.err, msg)
	}
}

func TestLookupString(t *testing.T) {
	tests := []struct {
		root   jsonObject
		path   []string
		result string
		err    error
	}{
		{
			jsonObject{},
			[]string{"a", "b"},
			"",
			nil,
		},
		{
			jsonObject{"nonString": int(1)},
			[]string{"nonString"},
			"",
			&UnmarshalWrongTypeError{Field: "nonString", Expected: "string", Got: 1},
		},
		{
			jsonObject{"nonObject": int(1)},
			[]string{"nonObject", "b"},
			"",
			&UnmarshalWrongTypeError{Field: "nonObject", Expected: "object", Got: 1},
		},
	}
	for _, test := range tests {
		gotResult, gotErr := test.root.String(test.path[0], test.path[1:]...)
		msg := fmt.Sprintf("%q.String(%q) => %q, %q", test.root, test.path, gotResult, gotErr)
		assert.Equal(t, gotResult, test.result, msg)
		assert.Equal(t, gotErr, test.err, msg)
	}
}

func TestLookupStringArray(t *testing.T) {
	tests := []struct {
		root   jsonObject
		path   string
		result []string
		err    error
	}{
		{
			jsonObject{},
			"a",
			nil,
			nil,
		},
		{
			jsonObject{"a": []string{"foo"}},
			"a",
			[]string{"foo"},
			nil,
		},
		{
			jsonObject{"a": []string{}},
			"a",
			[]string{},
			nil,
		},
		{
			jsonObject{"string": "foo"},
			"string",
			nil,
			&UnmarshalWrongTypeError{Field: "string", Expected: "array of strings", Got: "foo"},
		},
	}
	for _, test := range tests {
		gotResult, gotErr := test.root.StringArray(test.path)
		msg := fmt.Sprintf("%q.String(%q) => %q, %q", test.root, test.path, gotResult, gotErr)
		assert.Equal(t, gotResult, test.result, msg)
		assert.Equal(t, gotErr, test.err, msg)
	}
}
