package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	weavenet "github.com/weaveworks/weave/net"
)

var (
	ErrNoCommandSpecified = errors.New("No command specified")
)

func main() {
	var (
		args      = os.Args[1:]
		notInExec = true
	)

	if len(args) > 0 && args[0] == "-s" {
		notInExec = false
		args = args[1:]
	}

	if notInExec {
		usr2 := make(chan os.Signal)
		signal.Notify(usr2, syscall.SIGUSR2)
		<-usr2
	}

	_, err := weavenet.EnsureInterface("ethwe", -1)
	checkErr(err)

	if len(args) == 0 {
		checkErr(ErrNoCommandSpecified)
	}

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
