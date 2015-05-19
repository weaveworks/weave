package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/weaveworks/weave/net"
)

func main() {
	if _, err := net.EnsureInterface("ethwe", 20); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	if len(os.Args) <= 1 {
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[1], os.Args[2:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
