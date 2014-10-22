package weavedns

import (
	"reflect"
	"testing"
)

func assertNoErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func assertStatus(t *testing.T, got int, wanted int, desc string) {
	if got != wanted {
		t.Fatalf("Expected %s %d but got %d", desc, wanted, got)
	}
}

func assertErrorInterface(t *testing.T, got interface{}, wanted interface{}, desc string) {
	got_t, wanted_t := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if !got_t.Implements(wanted_t) {
		t.Fatalf("Expected %s but got %s (%s)", wanted_t.String(), got_t.String(), desc)
	}
}

func assertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	got_t, wanted_t := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if got_t != wanted_t {
		t.Fatalf("Expected %s but got %s (%s)", wanted_t.String(), got_t.String(), desc)
	}
}
