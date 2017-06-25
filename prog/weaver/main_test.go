package main

import (
	"testing"

	weavetest "github.com/weaveworks/weave/testing"
)

func TestMain(t *testing.T) {
	if weavetest.TrimTestArgs() {
		main()
	}
}
