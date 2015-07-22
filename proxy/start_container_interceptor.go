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
		return fmt.Errorf("Failed: %s: %s", fmt.Sprintf(description, container.ID), string(stderr))
	} else if len(stderr) > 0 {
		Log.Warningf("%s: %s", fmt.Sprintf(description, container.ID), container.ID, string(stderr))
	}
	return nil
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	if strings.HasSuffix(r.URL.Path, "/restart") {
		container, err := inspectContainerInPath(i.proxy.client, r.URL.Path)
		if err != nil {
			return err
		}
		return callWeaveAndLog(container, "Notifying weave of restart of container %s", "notify-restart", container.ID)
	}
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
	if err := callWeaveAndLog(container, "Attaching container %s to weave network", args...); err != nil {
		return err
	}

	return i.proxy.client.KillContainer(docker.KillContainerOptions{ID: container.ID, Signal: docker.SIGUSR2})
}
