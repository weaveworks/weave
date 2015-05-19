package proxy

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type createExecInterceptor struct {
	client *docker.Client
}

type createExecRequestBody struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty" yaml:"HostConfig,omitempty"`
	MacAddress string             `json:"MacAddress,omitempty" yaml:"MacAddress,omitempty"`
}

func (i *createExecInterceptor) InterceptRequest(r *http.Request) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()
	Info.Println("CreateExecInterceptor Intercepting: ", string(body))

	options := docker.CreateExecOptions{}
	if err := json.Unmarshal(body, &options); err != nil {
		return err
	}

	subs := containerIDRegexp.FindStringSubmatch(r.URL.Path)
	if subs == nil {
		Warning.Printf("No container id found in request with path %s", r.URL.Path)
		return nil
	}
	containerID := subs[1]

	container, err := i.client.InspectContainer(containerID)
	if err != nil {
		Warning.Printf("Error inspecting container %s: %v", containerID, err)
		return nil
	}

	if _, ok := weaveCIDRsFromConfig(container.Config); ok {
		options.Cmd = append(weaveWaitEntrypoint, options.Cmd...)
	}

	newBody, err := json.Marshal(options)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))

	return nil
}

func (i *createExecInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
