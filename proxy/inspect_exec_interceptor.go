package proxy

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
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

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	exec := &docker.ExecInspect{}
	if err := json.Unmarshal(body, exec); err != nil {
		return err
	}

	if err := updateContainerNetworkSettings(&exec.Container); err != nil {
		Log.Warningf("Inspecting exec %s failed: %s", exec.ID, err)
	}

	newBody, err := json.Marshal(exec)
	if err != nil {
		return err
	}
	r.Body = ioutil.NopCloser(bytes.NewReader(newBody))
	r.ContentLength = int64(len(newBody))
	r.TransferEncoding = nil // Stop it being chunked, because that hangs

	return nil
}
