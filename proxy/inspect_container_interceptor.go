package proxy

import (
	"net/http"

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

	container := &docker.Container{}
	if err := unmarshalResponseBody(r, container); err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(container); err != nil {
		Log.Warningf("Inspecting container %s failed: %s", container.ID, err)
	}

	return marshalResponseBody(r, container)
}

func updateContainerNetworkSettings(container *docker.Container) error {
	mac, ips, nets, err := weaveContainerIPs(container)
	if err != nil || len(ips) == 0 {
		return err
	}

	container.NetworkSettings.MacAddress = mac
	container.NetworkSettings.IPAddress = ips[0].String()
	container.NetworkSettings.IPPrefixLen, _ = nets[0].Mask.Size()
	return nil
}
