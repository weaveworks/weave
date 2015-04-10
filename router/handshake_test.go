package router

import (
	wt "github.com/weaveworks/weave/testing"
	"testing"
)

func TestFieldValidator(t *testing.T) {
	testMap := map[string]string{
		"Protocol":        Protocol,
		"ProtocolVersion": "123",
	}
	fv := NewFieldValidator(testMap)
	err := fv.CheckEqual("ProtocolVersion", "123")
	wt.AssertNoErr(t, err)
	wt.AssertNoErr(t, fv.Err())
	err = fv.CheckEqual("ProtocolVersion", "124")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Expected error from incorrect string check")
	// Now see that a valid check does not blank out a previous error
	err = fv.CheckEqual("ProtocolVersion", "123")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Previous error should be retained")

	fv = NewFieldValidator(testMap)
	val, err := fv.Value("Protocol")
	wt.AssertNoErr(t, err)
	wt.AssertEqualString(t, val, Protocol, "Protocol")
	val, err = fv.Value("Protocolx")
	wt.AssertFalse(t, err == nil || fv.Err() == nil, "Expected error from incorrect string check")
}
