package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
)

type createContainerInterceptor struct {
	client  *docker.Client
	withDNS bool
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

	if _, ok := weaveCIDRsFromConfig(container.Config); ok {
		i.addToVolumesFrom(container.HostConfig, "weaveproxy")
		if err := i.setWeaveWaitEntrypoint(container.Config); err != nil {
			return nil, err
		}
		if err := i.setWeaveDNS(&container); err != nil {
			return nil, err
		}
	}

	newBody, err := json.Marshal(container)
	if err != nil {
		return nil, err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))

	return r, nil
}

func (i *createContainerInterceptor) addToVolumesFrom(config *docker.HostConfig, mounts ...string) {
	config.VolumesFrom = append(config.VolumesFrom, mounts...)
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

	container.Entrypoint = []string{"/home/weavewait/weavewait"}
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

	dockerBridgeIP, err := getDockerBridgeIP()
	if err != nil {
		return err
	}
	container.HostConfig.DNS = append(container.HostConfig.DNS, dockerBridgeIP)

	if len(container.HostConfig.DNSSearch) == 0 {
		container.HostConfig.DNSSearch = []string{"."}
	}
	return nil
}

func getDockerBridgeIP() (string, error) {
	out, err := callWeave("dns-args")
	if err != nil {
		return "", fmt.Errorf("Error fetching weave dns-args: %s", err)
	}

	var dockerBridgeIP string
	segments := strings.Split(string(out), " ")
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == "--dns" {
			dockerBridgeIP = segments[i+1]
		}
	}

	if dockerBridgeIP == "" {
		return "", fmt.Errorf("Docker bridge IP not found in weave output: %s", string(out))
	}
	return dockerBridgeIP, nil
}

func (i *createContainerInterceptor) InterceptResponse(res *http.Response) (*http.Response, error) {
	return res, nil
}
