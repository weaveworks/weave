package main

import (
	"testing"

	weavetest "github.com/rajch/weave/testing"
)

func TestMain(t *testing.T) {
	if weavetest.TrimTestArgs() {
		main()
	}
}
