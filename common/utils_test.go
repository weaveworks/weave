package common_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	commonutils "github.com/weaveworks/weave/common"
)

func TestExitCode4Helper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(4)
}

func TestExitCode1Helper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(1)
}

func TestExitCode0Helper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(0)
}

func TestExitCodes(t *testing.T) {
	execEnv := []string{"GO_WANT_HELPER_PROCESS=1"}
	c0 := exec.Command(os.Args[0], []string{"-test.run=TestExitCode0Helper", "--"}...)
	c0.Env = execEnv
	c1 := exec.Command(os.Args[0], []string{"-test.run=TestExitCode1Helper", "--"}...)
	c1.Env = execEnv
	c4 := exec.Command(os.Args[0], []string{"-test.run=TestExitCode4Helper", "--"}...)
	c4.Env = execEnv

	assert.False(t, commonutils.IsErrExitCode4(nil))
	assert.False(t, commonutils.IsErrExitCode4(fmt.Errorf("some error")))
	assert.False(t, commonutils.IsErrExitCode4(c0.Run()))
	assert.False(t, commonutils.IsErrExitCode4(c1.Run()))
	assert.True(t, commonutils.IsErrExitCode4(c4.Run()))
}
