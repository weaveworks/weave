package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

var (
	containerIDRegexp   = regexp.MustCompile("^(/v[0-9\\.]*)?/containers/([^/]*)/.*")
	weaveWaitEntrypoint = []string{"/w/w"}
)

func callWeave(args ...string) ([]byte, error) {
	args = append([]string{"--local"}, args...)
	Debug.Print("Calling weave", args)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PROCFS=/hostproc", "PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	out, err := cmd.CombinedOutput()
	return out, err
}

func weaveCIDRsFromConfig(config *docker.Config, noDefaultIPAM bool) ([]string, bool) {
	for _, e := range config.Env {
		if strings.HasPrefix(e, "WEAVE_CIDR=") {
			if e[11:] == "none" {
				return nil, false
			}
			return strings.Fields(e[11:]), true
		}
	}
	return nil, !noDefaultIPAM
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
		Warning.Println(err)
		return nil, err
	}
	containerID := subs[2]

	container, err := client.InspectContainer(containerID)
	if err != nil {
		Warning.Printf("Error inspecting container %s: %v", containerID, err)
	}
	return container, err
}
