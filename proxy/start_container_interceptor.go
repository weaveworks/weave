package proxy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct {
	client   *docker.Client
	withDNS  bool
	withIPAM bool
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
	if !ok && !i.withIPAM {
		Debug.Print("No Weave CIDR, ignoring")
		return nil
	}
	Info.Printf("Attaching container %s with WEAVE_CIDR \"%s\" to weave network", container.ID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, container.ID)
	if output, err := callWeave(args...); err != nil {
		i.client.KillContainer(docker.KillContainerOptions{ID: container.ID})
		Warning.Printf("Attaching container %s to weave network failed: %s", container.ID, string(output))
		return errors.New(string(output))
	}

	return i.client.KillContainer(docker.KillContainerOptions{ID: container.ID, Signal: docker.SIGUSR2})
}
