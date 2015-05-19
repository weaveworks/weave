package proxy

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/fsouza/go-dockerclient"
)

type createContainerInterceptor struct {
	client         *docker.Client
	withDNS        bool
	dockerBridgeIP string
}

type createContainerRequestBody struct {
	*docker.Config
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty" yaml:"HostConfig,omitempty"`
	MacAddress string             `json:"MacAddress,omitempty" yaml:"MacAddress,omitempty"`
}

func (i *createContainerInterceptor) InterceptRequest(r *http.Request) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	container := createContainerRequestBody{}
	if err := json.Unmarshal(body, &container); err != nil {
		return err
	}

	if _, ok := weaveCIDRsFromConfig(container.Config); ok {
		container.HostConfig.VolumesFrom = append(container.HostConfig.VolumesFrom, "weaveproxy")
		if err := i.setWeaveWaitEntrypoint(container.Config); err != nil {
			return err
		}
		if err := i.setWeaveDNS(&container); err != nil {
			return err
		}
	}

	newBody, err := json.Marshal(container)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))

	return nil
}

func (i *createContainerInterceptor) setWeaveWaitEntrypoint(container *docker.Config) error {
	var image *docker.Config
	if !configHasEntrypoint(container) || !configHasCmd(container) {
		imageInfo, err := i.client.InspectImage(container.Image)
		if err != nil {
			return err
		}
		image = imageInfo.Config
	}

	var entry, command []string
	if configHasEntrypoint(container) {
		entry = container.Entrypoint
	} else if configHasEntrypoint(image) {
		entry = image.Entrypoint
	}
	if configHasCmd(container) {
		command = container.Cmd
	} else if configHasCmd(image) {
		command = image.Cmd
	}

	container.Entrypoint = weaveWaitEntrypoint
	container.Cmd = append(entry, command...)
	return nil
}

func configHasEntrypoint(c *docker.Config) bool {
	return c != nil && len(c.Entrypoint) > 0
}
func configHasCmd(c *docker.Config) bool {
	return c != nil && len(c.Cmd) > 0
}

func (i *createContainerInterceptor) setWeaveDNS(container *createContainerRequestBody) error {
	if !i.withDNS {
		return nil
	}

	container.HostConfig.DNS = append(container.HostConfig.DNS, i.dockerBridgeIP)

	if len(container.HostConfig.DNSSearch) == 0 {
		container.HostConfig.DNSSearch = []string{"."}
	}
	return nil
}

func (i *createContainerInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
