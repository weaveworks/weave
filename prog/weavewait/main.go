package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	weavenet "github.com/weaveworks/weave/net"
	"github.com/weaveworks/weave/proxy"
)

func main() {
	args := os.Args[1:]

	if len(args) > 0 && args[0] == "-s" {
		args = args[1:]
	} else {
		usr2 := make(chan os.Signal)
		signal.Notify(usr2, syscall.SIGUSR2)
		<-usr2
	}

	_, err := weavenet.EnsureInterface("ethwe", -1)
	checkErr(err)

	if len(args) == 0 {
		checkErr(proxy.ErrNoCommandSpecified)
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
