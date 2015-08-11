package common

import (
	"os"
	"os/signal"
	"runtime"
	"syscall"
)

// A subsystem/server/... that can be stopped or queried about the status with a signal
type SignalReceiver interface {
	Stop() error
}

func SignalHandlerLoop(ss ...SignalReceiver) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGUSR1, syscall.SIGTERM)
	buf := make([]byte, 1<<20)
	for {
		switch <-sigs {
		case syscall.SIGINT, syscall.SIGTERM:
			Log.Infof("=== received SIGINT/SIGTERM ===\n*** exiting")
			for _, subsystem := range ss {
				subsystem.Stop()
			}
			return
		case syscall.SIGQUIT:
			stacklen := runtime.Stack(buf, true)
			Log.Infof("=== received SIGQUIT ===\n*** goroutine dump...\n%s\n*** end", buf[:stacklen])
		}
	}
}
