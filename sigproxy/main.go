package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func checkArguments() {
	if len(os.Args) == 1 {
		log.Fatal("USAGE: sigproxy <command> [arguments ...]")
	}
}

func installSignalHandler() chan os.Signal {
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT)

	return sc
}

func execCommand() *exec.Cmd {
	// First argument is command to run, remainder are its arguments
	cmd := exec.Command(os.Args[1], os.Args[2:]...)

	// These conveniently default to /dev/null otherwise...
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	return cmd
}

func forwardSignals(sc chan os.Signal, cmd *exec.Cmd) {
	for {
		// Signalling PID 0 delivers to our process group
		syscall.Kill(0, (<-sc).(syscall.Signal))
	}
}

func waitAndExit(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		// Exit status is platform specific so not readily accessible - casts
		// required to access system-dependent exit information
		if exitErr, ok := err.(*exec.ExitError); ok {
			if waitStatus, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				os.Exit(waitStatus.ExitStatus())
			} else {
				os.Exit(1)
			}
		} else {
			os.Exit(1)
		}
	} else {
		os.Exit(0)
	}
}

func main() {
	checkArguments()
	sc := installSignalHandler()
	cmd := execCommand()
	go forwardSignals(sc, cmd)
	waitAndExit(cmd)
}
