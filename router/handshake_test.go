package router

import (
	wt "github.com/weaveworks/weave/testing"
	"testing"
)

func TestFieldValidator(t *testing.T) {
	testMap := map[string]string{"a": "a"}

	fv := NewFieldValidator(testMap)
	val, err := fv.Value("a")
	wt.AssertNoErr(t, err)
	wt.AssertNoErr(t, fv.Err())
	wt.AssertEqualString(t, val, "a", "")
	_, err = fv.Value("x")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Expected error")
	_, err = fv.Value("a")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Previous error should be retained")

	fv = NewFieldValidator(testMap)
	err = fv.CheckEqual("a", "a")
	wt.AssertNoErr(t, err)
	wt.AssertNoErr(t, fv.Err())
	err = fv.CheckEqual("a", "b")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Expected error")
	err = fv.CheckEqual("a", "a")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Previous error should be retained")
}
