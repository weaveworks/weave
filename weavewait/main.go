package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/weaveworks/weave/net"
)

func main() {
	if _, err := net.EnsureInterface("ethwe", 20); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	if len(os.Args) <= 1 {
		os.Exit(0)
	}

	binary, err := exec.LookPath(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := syscall.Exec(binary, os.Args[1:], os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
