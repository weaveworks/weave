package common

import (
	"os/exec"
	"strings"
	"syscall"
)

// Assert test is true, panic otherwise
func Assert(test bool) {
	if !test {
		panic("Assertion failure")
	}
}

func ErrorMessages(errors []error) string {
	var result []string
	for _, err := range errors {
		result = append(result, err.Error())
	}
	return strings.Join(result, "\n")
}

// IsErrExitCode4 checks if the error is exit code 4 from an exec.Command.
// Useful for use with iptables when you can't access the lock file and need to retry.
func IsErrExitCode4(err error) bool {
	if ierr, ok := err.(*exec.ExitError); ok {
		if status, ok := ierr.Sys().(syscall.WaitStatus); ok {
			// (magic exit code 4 found in iptables source code; undocumented)
			if status.ExitStatus() == 4 {
				return true
			}
		}
	}
	return false
}
