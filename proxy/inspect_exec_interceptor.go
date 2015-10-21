package proxy

import (
	"net/http"

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

	exec := jsonObject{}
	if err := unmarshalResponseBody(r, &exec); err != nil {
		return err
	}

	container, err := exec.Object("Container")
	if err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(container, i.proxy); err != nil {
		Log.Warningf("Inspecting exec %s failed: %s", exec["Id"], err)
	}

	return marshalResponseBody(r, exec)
}
