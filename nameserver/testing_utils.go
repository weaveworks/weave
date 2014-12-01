package nameserver

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
	if got == nil {
		t.Fatalf("Expected %s but got nil (%s)", reflect.TypeOf(wanted).Elem(), desc)
	}
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if !gotT.Implements(wantedT) {
		t.Fatalf("Expected %s but got %s (%s)", wantedT, gotT, desc)
	}
}

func assertErrorType(t *testing.T, got interface{}, wanted interface{}, desc string) {
	if got == nil {
		t.Fatalf("Expected %s but got nil (%s)", reflect.TypeOf(wanted).Elem(), desc)
	}
	gotT, wantedT := reflect.TypeOf(got), reflect.TypeOf(wanted).Elem()
	if gotT != wantedT {
		t.Fatalf("Expected %s but got %s (%s)", wantedT, gotT, desc)
	}
}
