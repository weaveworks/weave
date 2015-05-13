package proxy

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/fsouza/go-dockerclient"
)

type createContainerInterceptor struct {
	client *docker.Client
}

func CreateContainerInterceptor(client *docker.Client) *createContainerInterceptor {
	return &createContainerInterceptor{
		client: client,
	}
}

type createContainerRequestBody struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty" yaml:"HostConfig,omitempty"`
	MacAddress string             `json:"MacAddress,omitempty" yaml:"MacAddress,omitempty"`
}

func (i *createContainerInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body.Close()

	container := createContainerRequestBody{}
	if err := json.Unmarshal(body, &container); err != nil {
		return nil, err
	}

	if _, ok := weaveAddrFromConfig(container.Config); ok {
		image, err := i.client.InspectImage(container.Config.Image)
		if err != nil {
			return nil, err
		}

		addToVolumesFrom(container.HostConfig, "weaveproxy")
		setWeaveWaitEntrypoint(image.Config, container.Config)
	}

	newBody, err := json.Marshal(container)
	if err != nil {
		return nil, err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))

	return r, nil
}

func addToVolumesFrom(config *docker.HostConfig, mounts ...string) error {
	config.VolumesFrom = append(config.VolumesFrom, mounts...)
	return nil
}

func setWeaveWaitEntrypoint(image, container *docker.Config) {
	container.Cmd = combineEntrypoints(image, container)
	container.Entrypoint = []string{"/home/weavewait/weavewait"}
}

func combineEntrypoints(image, container *docker.Config) []string {
	var entry, command []string
	if len(container.Entrypoint) > 0 {
		entry = container.Entrypoint
	} else if len(image.Entrypoint) > 0 {
		entry = image.Entrypoint
	}
	if len(container.Cmd) > 0 {
		command = container.Cmd
	} else if len(image.Cmd) > 0 {
		command = image.Cmd
	}

	return append(entry, command...)
}

func (i *createContainerInterceptor) InterceptResponse(res *http.Response) (*http.Response, error) {
	return res, nil
}
