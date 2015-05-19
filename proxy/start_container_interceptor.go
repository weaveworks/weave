package proxy

import (
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct {
	client  *docker.Client
	withDNS bool
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	container, err := inspectContainerInPath(i.client, r.Request.URL.Path)
	if err != nil {
		return err
	}

	cidrs, ok := weaveCIDRsFromConfig(container.Config)
	if !ok {
		Debug.Print("No Weave CIDR, ignoring")
		return nil
	}
	Info.Printf("Attaching container %s with WEAVE_CIDR \"%s\" to weave network", container.ID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, container.ID)
	if _, err := callWeave(args...); err != nil {
		Warning.Printf("Attaching container %s to weave network failed: %v", container.ID, err)
		return nil
	}
	return nil
}
