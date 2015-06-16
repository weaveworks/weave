package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
	"github.com/weaveworks/weave/nameserver"
)

type createContainerInterceptor struct{ proxy *Proxy }

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

	if cidrs, ok := weaveCIDRsFromConfig(container.Config); ok || i.proxy.WithIPAM {
		Info.Printf("Creating container with WEAVE_CIDR \"%s\"", strings.Join(cidrs, " "))
		if container.HostConfig == nil {
			container.HostConfig = &docker.HostConfig{}
		}
		container.HostConfig.VolumesFrom = append(container.HostConfig.VolumesFrom, "weaveproxy")
		if container.Config == nil {
			container.Config = &docker.Config{}
		}
		if err := i.setWeaveWaitEntrypoint(container.Config); err != nil {
			return err
		}
		if err := i.setWeaveDNS(&container, r); err != nil {
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
	if len(container.Entrypoint) == 0 {
		image, err := i.proxy.client.InspectImage(container.Image)
		if err != nil {
			return err
		}

		if len(container.Cmd) == 0 {
			container.Cmd = image.Config.Cmd
		}

		if container.Entrypoint == nil {
			container.Entrypoint = image.Config.Entrypoint
		}
	}

	container.Entrypoint = append(weaveWaitEntrypoint, container.Entrypoint...)
	return nil
}

func (i *createContainerInterceptor) setWeaveDNS(container *createContainerRequestBody, r *http.Request) error {
	if !i.proxy.WithDNS {
		return nil
	}

	container.HostConfig.DNS = append(container.HostConfig.DNS, i.proxy.dockerBridgeIP)

	if len(container.HostConfig.DNSSearch) == 0 {
		container.HostConfig.DNSSearch = []string{"."}
	}

	name := r.URL.Query().Get("name")
	if container.Hostname == "" && name != "" {
		container.Hostname = name
		// Strip trailing period because it's unusual to see it used on the end of a host name
		container.Domainname = strings.TrimSuffix(i.getDNSDomain(), ".")
	}

	return nil
}

func (i *createContainerInterceptor) getDNSDomain() (domain string) {
	domain = nameserver.DefaultLocalDomain
	dnsContainer, err := i.proxy.client.InspectContainer("weavedns")
	if err != nil ||
		dnsContainer.NetworkSettings == nil ||
		dnsContainer.NetworkSettings.IPAddress == "" {
		return
	}

	url := fmt.Sprintf("http://%s:%d/domain", dnsContainer.NetworkSettings.IPAddress, nameserver.DefaultHTTPPort)
	resp, err := http.Get(url)
	if err != nil || resp.StatusCode != http.StatusOK {
		return
	}
	defer resp.Body.Close()

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}

	return string(b)
}

func (i *createContainerInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
