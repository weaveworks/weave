package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type createExecInterceptor struct{ proxy *Proxy }

func (i *createExecInterceptor) InterceptRequest(r *http.Request) error {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()

	options := docker.CreateExecOptions{}
	if err := json.Unmarshal(body, &options); err != nil {
		return err
	}

	container, err := inspectContainerInPath(i.proxy.client, r.URL.Path)
	if err != nil {
		return err
	}

	_, hasWeaveWait := container.Volumes["/w"]
	cidrs, err := i.proxy.weaveCIDRsFromConfig(container.Config, container.HostConfig)
	if err != nil {
		Log.Infof("Leaving container %s alone because %s", container.ID, err)
	} else if hasWeaveWait {
		Log.Infof("Exec in container %s with WEAVE_CIDR \"%s\"", container.ID, strings.Join(cidrs, " "))
		cmd := append(weaveWaitEntrypoint, "-s")
		options.Cmd = append(cmd, options.Cmd...)
	}

	if err := marshalRequestBody(r, options); err != nil {
		return err
	}

	return nil
}

func (i *createExecInterceptor) InterceptResponse(r *http.Response) error {
	return nil
}
