package proxy

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct{ proxy *Proxy }

func callWeaveAndLog(container *docker.Container, description string, args ...string) error {
	if _, stderr, err := callWeave(args...); err != nil {
		return fmt.Errorf("Failed: %s container %s: %s", description, container.ID, string(stderr))
	} else if len(stderr) > 0 {
		Log.Warningf("%s container %s: %s", description, container.ID, string(stderr))
	}
	return nil
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	container, err := inspectContainerInPath(i.proxy.client, r.Request.URL.Path)
	if err != nil {
		return err
	}

	cidrs, err := i.proxy.weaveCIDRsFromConfig(container.Config, container.HostConfig)
	if err != nil {
		Log.Infof("Ignoring container %s due to %s", container.ID, err)
		return nil
	}
	Log.Infof("Attaching container %s with WEAVE_CIDR \"%s\" to weave network", container.ID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, "--or-die", container.ID)
	if err := callWeaveAndLog(container, "Attaching to weave network", args...); err != nil {
		return err
	}

	return i.proxy.client.KillContainer(docker.KillContainerOptions{ID: container.ID, Signal: docker.SIGUSR2})
}
