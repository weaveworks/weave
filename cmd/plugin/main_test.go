package main

import (
	"testing"

	weavetest "github.com/weaveworks/weave/pkg/testing"
)

func TestMain(t *testing.T) {
	weavetest.TrimTestArgs()
	main()
}
