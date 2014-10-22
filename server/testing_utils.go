package weavedns

import (
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
