package proxy

import (
	"net/http"
)

type startContainerInterceptor struct{ proxy *Proxy }

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	container, err := inspectContainerInPath(i.proxy.client, r.URL.Path)
	if err == nil && containerShouldAttach(container) {
		i.proxy.createWait(r, container.ID)
	}
	return err
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	defer i.proxy.removeWait(r.Request)
	if r.StatusCode < 200 || r.StatusCode >= 300 { // Docker didn't do the start
		return nil
	}
	i.proxy.waitForStart(r.Request)
	return nil
}
