package common

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

// A subsystem/server/... that can be stopped or queried about the status with a signal
type SignalsReceiver interface {
	Status() string
	Stop() error
}

func SignalHandlerLoop(ss ...SignalsReceiver) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGUSR1)
	buf := make([]byte, 1<<20)
	for {
		sig := <-sigs
		switch sig {
		case syscall.SIGINT:
			Info.Printf("=== received SIGINT ===\n*** exiting\n")
			for _, subsystem := range ss {
				subsystem.Stop()
			}
			os.Exit(0)
		case syscall.SIGQUIT:
			stacklen := runtime.Stack(buf, true)
			Info.Printf("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end\n", buf[:stacklen])
		case syscall.SIGUSR1:
			for _, subsystem := range ss {
				Info.Printf("=== received SIGUSR1 ===\n*** status...\n%s\n*** end\n", subsystem.Status())
			}
		}
	}
}
