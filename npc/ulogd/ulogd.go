package ulogd

import (
	log "github.com/Sirupsen/logrus"
	"io"
	"os"
	"os/exec"
)

func waitForExit(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		log.Fatalf("ulogd terminated: %v", err)
	}
	log.Fatal("ulogd terminated normally")
}

func Start() error {
	cmd := exec.Command("/usr/sbin/ulogd", "-v")
	stdout, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go io.Copy(os.Stdout, stdout)
	go waitForExit(cmd)
	return nil
}
