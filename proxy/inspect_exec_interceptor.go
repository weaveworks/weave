package proxy

import (
	"net/http"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type inspectExecInterceptor struct{ proxy *Proxy }

func (i *inspectExecInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *inspectExecInterceptor) InterceptResponse(r *http.Response) error {
	if !i.proxy.RewriteInspect {
		return nil
	}

	exec := &docker.ExecInspect{}
	if err := unmarshalResponseBody(r, &exec); err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(&exec.Container); err != nil {
		Log.Warningf("Inspecting exec %s failed: %s", exec.ID, err)
	}

	return marshalResponseBody(r, exec)
}
