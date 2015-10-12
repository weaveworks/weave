package proxy

import (
	"net/http"

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

	container := jsonObject{}
	if err := unmarshalResponseBody(r, &container); err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(container); err != nil {
		Log.Warningf("Inspecting container %s failed: %s", container["Id"], err)
	}

	return marshalResponseBody(r, container)
}

func updateContainerNetworkSettings(container jsonObject) error {
	containerID, err := container.String("Id")
	if err != nil {
		return err
	}

	mac, ips, nets, err := weaveContainerIPs(containerID)
	if err != nil || len(ips) == 0 {
		return err
	}

	networkSettings, err := container.Object("NetworkSettings")
	if err != nil {
		return err
	}
	networkSettings["MacAddress"] = mac
	networkSettings["IPAddress"] = ips[0].String()
	networkSettings["IPPrefixLen"], _ = nets[0].Mask.Size()
	return nil
}
