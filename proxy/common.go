package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"regexp"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

var (
	containerIDRegexp   = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/([^/]*)/.*")
	weaveWaitEntrypoint = []string{"/w/w"}
)

func callWeave(args ...string) ([]byte, []byte, error) {
	args = append([]string{"--local"}, args...)
	Log.Debug("Calling weave", args)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"PROCFS=/hostproc",
	}
	if bridge := os.Getenv("DOCKER_BRIDGE"); bridge != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_BRIDGE=%s", bridge))
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func marshalRequestBody(r *http.Request, body interface{}) error {
	newBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	return nil
}

func inspectContainerInPath(client *docker.Client, path string) (*docker.Container, error) {
	subs := containerIDRegexp.FindStringSubmatch(path)
	if subs == nil {
		err := fmt.Errorf("No container id found in request with path %s", path)
		Log.Warningln(err)
		return nil, err
	}
	containerID := subs[2]

	container, err := client.InspectContainer(containerID)
	if err != nil {
		Log.Warningf("Error inspecting container %s: %v", containerID, err)
	}
	return container, err
}
