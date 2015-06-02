package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/weaveworks/weave/net"
)

func main() {
	if len(os.Args) <= 1 {
		os.Exit(0)
	}

	args := os.Args[1:]
	signalWait := 20
	if args[0] == "-s" {
		signalWait = 0
		args = args[1:]
	}
	interfaceWait := 20 - signalWait

	usr2 := make(chan os.Signal)
	signal.Notify(usr2, syscall.SIGUSR2)
	select {
	case <-usr2:
	case <-time.After(time.Duration(signalWait) * time.Second):
	}
	_, err := net.EnsureInterface("ethwe", interfaceWait)
	checkErr(err)

	binary, err := exec.LookPath(args[0])
	checkErr(err)

	checkErr(syscall.Exec(binary, args, os.Environ()))
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
