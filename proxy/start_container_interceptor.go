package proxy

import (
	"net/http"
)

type startContainerInterceptor struct{ proxy *Proxy }

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	container, err := inspectContainerInPath(i.proxy.client, r.URL.Path)
	if err != nil {
		return err
	}

	// If the client has sent some JSON which might be a HostConfig, add our
	// parameters back into it, otherwise Docker will consider them overwritten
	if containerShouldAttach(container) && r.Header.Get("Content-Type") == "application/json" {
		hostConfig := map[string]interface{}{}
		if err := unmarshalRequestBody(r, &hostConfig); err != nil {
			return err
		}
		if len(hostConfig) > 0 {
			i.proxy.addWeaveWaitVolume(hostConfig)
			if dnsDomain := i.proxy.getDNSDomain(); dnsDomain != "" {
				if err := i.proxy.setWeaveDNS(hostConfig, container.Config.Hostname, dnsDomain); err != nil {
					return err
				}
			}

			if err := marshalRequestBody(r, hostConfig); err != nil {
				return err
			}
		}
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
