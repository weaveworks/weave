package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

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

	options := docker.CreateExecOptions{}
	if err := json.Unmarshal(body, &options); err != nil {
		return err
	}

	container, err := inspectContainerInPath(i.client, r.URL.Path)
	if err != nil {
		return err
	}

	if cidrs, ok := weaveCIDRsFromConfig(container.Config); ok {
		Info.Printf("Exec in container %s with WEAVE_CIDR \"%s\"", container.ID, strings.Join(cidrs, " "))
		options.Cmd = append(weaveWaitEntrypoint, options.Cmd...)
	}

	if err := marshalRequestBody(r, options); err != nil {
		return err
	}

	return nil
}

func (i *createExecInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
