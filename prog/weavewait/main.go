package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	usr2 := make(chan os.Signal)
	signal.Notify(usr2, syscall.SIGUSR2)
	select {
	case <-usr2:
	case <-time.After(20 * time.Second):
		checkErr(errors.New("Container timed out waiting for signal from proxy"))
	}

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
