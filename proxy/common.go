package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
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

func unmarshalRequestBody(r *http.Request, target interface{}) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber() // don't want large numbers in scientific format
	return d.Decode(&target)
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

func unmarshalResponseBody(r *http.Response, target interface{}) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := r.Body.Close(); err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(body))

	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber() // don't want large numbers in scientific format
	return d.Decode(&target)
}

func marshalResponseBody(r *http.Response, body interface{}) error {
	newBody, err := json.Marshal(body)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	// Stop it being chunked, because that hangs
	r.TransferEncoding = nil
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

func weaveContainerIPs(containerID string) (mac string, ips []net.IP, nets []*net.IPNet, err error) {
	stdout, stderr, err := callWeave("ps", containerID)
	if err != nil || len(stderr) > 0 {
		err = errors.New(string(stderr))
		return
	}
	if len(stdout) <= 0 {
		return
	}

	fields := strings.Fields(string(stdout))
	if len(fields) < 2 {
		return
	}
	mac = fields[1]

	var ip net.IP
	var ipnet *net.IPNet
	for _, cidr := range fields[2:] {
		ip, ipnet, err = net.ParseCIDR(cidr)
		if err != nil {
			return
		}
		ips = append(ips, ip)
		nets = append(nets, ipnet)
	}
	return
}
