package main

import (
        "os/exec"
        "testing"
        "strings"
)

func TestWeaveLaunch(t *testing.T) {
        cmd := exec.Command(sudoBinary, weaveBinary, "launch")
        out, err := cmd.Output()
        if err != nil {
                t.Fatal(err)
        }

        id := strings.TrimSpace(string(out))
        if err := waitInspect(id, "{{ .State.Running }}", "true", 5); err != nil {
                t.Fatal(err)
        }
        deleteAllContainers()
        logDone("Weave launch succeeded")
}
