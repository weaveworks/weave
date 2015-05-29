package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/weaveworks/weave/net"
)

func main() {
	_, err := net.EnsureInterface("ethwe", 20)
	checkErr(err)

	if len(os.Args) <= 1 {
		os.Exit(0)
	}

	binary, err := exec.LookPath(os.Args[1])
	checkErr(err)

	checkErr(syscall.Exec(binary, os.Args[1:], os.Environ()))
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
