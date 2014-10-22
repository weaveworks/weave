package weavedns

import (
	"testing"
)

func assertNoErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}
