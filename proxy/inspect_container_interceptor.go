package proxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type inspectContainerInterceptor struct{ proxy *Proxy }

func (i *inspectContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *inspectContainerInterceptor) InterceptResponse(r *http.Response) error {
	if !i.proxy.RewriteInspect {
		return nil
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	container := &docker.Container{}
	if err := json.Unmarshal(body, container); err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(container); err != nil {
		Log.Warningf("Inspecting container %s failed: %s", container.ID, err)
	}

	newBody, err := json.Marshal(container)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.TransferEncoding = nil // Stop it being chunked, because that hangs

	return nil
}

func updateContainerNetworkSettings(container *docker.Container) error {
	stdout, stderr, err := callWeave("ps", container.ID)
	if err != nil || len(stderr) > 0 {
		return errors.New(string(stderr))
	}
	if len(stdout) <= 0 {
		return nil
	}

	fields := strings.Fields(string(stdout))
	if len(fields) <= 2 {
		return nil
	}

	ipParts := strings.SplitN(fields[2], "/", 2)
	if len(ipParts) <= 1 {
		return nil
	}

	mask, err := strconv.ParseInt(ipParts[1], 10, 0)
	if err != nil {
		return err
	}

	container.NetworkSettings.MacAddress = fields[1]
	container.NetworkSettings.IPAddress = ipParts[0]
	container.NetworkSettings.IPPrefixLen = int(mask)
	return nil
}
