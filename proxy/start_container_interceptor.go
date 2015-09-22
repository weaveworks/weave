package proxy

import (
	"net/http"

	"github.com/fsouza/go-dockerclient"
)

type startContainerInterceptor struct{ proxy *Proxy }

type startContainerRequestBody struct {
	HostConfig *docker.HostConfig `json:"HostConfig,omitempty" yaml:"HostConfig,omitempty"`
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	container, err := inspectContainerInPath(i.proxy.client, r.URL.Path)
	if err != nil {
		return err
	}

	if !containerShouldAttach(container) || r.Header.Get("Content-Type") != "application/json" {
		i.proxy.createWait(r, container.ID)
		return nil
	}

	hostConfig := &docker.HostConfig{}
	if err := unmarshalRequestBody(r, &hostConfig); err != nil {
		return err
	}

	i.proxy.addWeaveWaitVolume(hostConfig)
	if dnsDomain, withDNS := i.proxy.getDNSDomain(); withDNS {
		if err := i.proxy.setWeaveDNS(hostConfig, container.Config.Hostname, dnsDomain); err != nil {
			return err
		}
	}

	if err := marshalRequestBody(r, hostConfig); err != nil {
		return err
	}
	i.proxy.createWait(r, container.ID)
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	defer i.proxy.removeWait(r.Request)
	if r.StatusCode < 200 || r.StatusCode >= 300 { // Docker didn't do the start
		return nil
	}
	i.proxy.waitForStart(r.Request)
	return nil
}
