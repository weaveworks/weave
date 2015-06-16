package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFieldValidator(t *testing.T) {
	testMap := map[string]string{"a": "a"}

	fv := NewFieldValidator(testMap)
	val, err := fv.Value("a")
	require.NoError(t, err)
	require.NoError(t, fv.Err())
	require.Equal(t, "a", val, "")
	_, err = fv.Value("x")
	require.False(t, err == nil || fv.Err() == nil, "Expected error")
	_, err = fv.Value("a")
	require.False(t, err == nil || fv.Err() == nil, "Previous error should be retained")

	fv = NewFieldValidator(testMap)
	err = fv.CheckEqual("a", "a")
	require.NoError(t, err)
	require.NoError(t, fv.Err())
	err = fv.CheckEqual("a", "b")
	require.False(t, err == nil || fv.Err() == nil, "Expected error")
	err = fv.CheckEqual("a", "a")
	require.False(t, err == nil || fv.Err() == nil, "Previous error should be retained")
}
