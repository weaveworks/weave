package proxy

import (
	"net/http"

	"github.com/weaveworks/weave/common"
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

	if _, ok := exec["Container"]; !ok {
		return nil
	}

	container, err := exec.Object("Container")
	if err != nil {
		return err
	}

	if err := i.proxy.updateContainerNetworkSettings(container); err != nil {
		common.Log.Warningf("Inspecting exec %s failed: %s", exec["Id"], err)
	}

	return marshalResponseBody(r, exec)
}
