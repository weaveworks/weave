package main

import (
        "fmt"
        "syscall"
        "os/exec"
        "time"
        "os"
        "strings"
)

var (
        sudoBinary = "sudo"
        weaveBinary = "weave"
        dockerBinary = "docker"
)

func init() {
        var err error
        dockerBinary, err = exec.LookPath(dockerBinary)
        if err != nil {
                fmt.Printf("ERROR: couldn't resolve full path to the Docker binary (%v)", err)
                os.Exit(1)
        }
}

func getExitCode(err error) (int, error) {
        exitCode := 0
        if exiterr, ok := err.(*exec.ExitError); ok {
                if procExit := exiterr.Sys().(syscall.WaitStatus); ok {
                        return procExit.ExitStatus(), nil
                }
        }
        return exitCode, fmt.Errorf("failed to get exit code")
}

func processExitCode(err error) (exitCode int) {
        if err != nil {
                var exiterr error
                if exitCode, exiterr = getExitCode(err); exiterr != nil {
                        // TODO: Fix this so we check the error's text.
                        // we've failed to retrieve exit code, so we set it to 127
                        exitCode = 127
                }
        }
        return
}

func runCommandWithOutput(cmd *exec.Cmd) (output string, exitCode int, err error) {
        exitCode = 0
        out, err := cmd.CombinedOutput()
        exitCode = processExitCode(err)
        output = string(out)
        return
}

func waitInspect(name, expr, expected string, timeout int) error {
        after := time.After(time.Duration(timeout) * time.Second)

        for {
                cmd := exec.Command(dockerBinary, "inspect", "-f", expr, name)
                out, _, err := runCommandWithOutput(cmd)
                if err != nil {
                        return fmt.Errorf("error executing docker inspect: %v", err)
                }

                out = strings.TrimSpace(out)
                if out == expected {
                        break
                }

                select {
                case <-after:
                        return fmt.Errorf("condition \"%q == %q\" not true in time", out, expected)
                default:
                }

                time.Sleep(100 * time.Millisecond)
        }
        return nil
}

func logDone(message string) {
        fmt.Printf("PASS %s\n", message)
}

func getAllContainers() (string, error) {
        getContainersCmd := exec.Command(dockerBinary, "ps", "-q", "-a")
        out, exitCode, err := runCommandWithOutput(getContainersCmd)
        if exitCode != 0 && err == nil {
                err = fmt.Errorf("failed to get a list of containers: %v\n", out)
        }

        return out, err
}

func deleteAllContainers() error {
        containers, err := getAllContainers()
        if err != nil {
                fmt.Println(containers)
                return err
        }

        if err = deleteContainer(containers); err != nil {
                return err
        }
        return nil
}

func deleteContainer(container string) error {
        container = strings.Replace(container, "\n", " ", -1)
        container = strings.Trim(container, " ")
        killArgs := fmt.Sprintf("kill %v", container)
        killSplitArgs := strings.Split(killArgs, " ")
        killCmd := exec.Command(dockerBinary, killSplitArgs...)
        runCommand(killCmd)
        rmArgs := fmt.Sprintf("rm -v %v", container)
        rmSplitArgs := strings.Split(rmArgs, " ")
        rmCmd := exec.Command(dockerBinary, rmSplitArgs...)
        exitCode, err := runCommand(rmCmd)
        // set error manually if not set
        if exitCode != 0 && err == nil {
                err = fmt.Errorf("failed to remove container: `docker rm` exit is non-zero")
        }

        return err
}

func runCommand(cmd *exec.Cmd) (exitCode int, err error) {
        exitCode = 0
        err = cmd.Run()
        exitCode = processExitCode(err)
        return
}
