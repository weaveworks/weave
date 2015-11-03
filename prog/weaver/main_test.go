package main

import (
	"os"
	"testing"

	weavetest "github.com/weaveworks/weave/testing"
)

func TestMain(t *testing.T) {
	weavetest.TrimTestArgs()
	if len(os.Args) > 1 {
		main()
	}
}
