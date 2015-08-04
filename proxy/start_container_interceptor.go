package proxy

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct{ proxy *Proxy }

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) (err error) {
	if r.StatusCode < 200 || r.StatusCode >= 300 { // Docker didn't do the start
		return nil
	}

	container, err := inspectContainerInPath(i.proxy.client, r.Request.URL.Path)
	if err != nil {
		return err
	}

	defer func() {
		// If entrypoint is weavewait we need to USR2 it so it starts! Even if
		// networking has been disabled since creation (as kubernetes does)
		if err == nil &&
			len(container.Config.Entrypoint) > 0 &&
			container.Config.Entrypoint[0] == weaveWaitEntrypoint[0] {
			err = i.proxy.client.KillContainer(docker.KillContainerOptions{ID: container.ID, Signal: docker.SIGUSR2})
		}
	}()

	cidrs, err := i.proxy.weaveCIDRsFromConfig(container.Config, container.HostConfig)
	if err != nil {
		Log.Infof("Ignoring container %s due to %s", container.ID, err)
		return nil
	}
	Log.Infof("Attaching container %s with WEAVE_CIDR \"%s\" to weave network", container.ID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, "--or-die", container.ID)
	if _, stderr, err := callWeave(args...); err != nil {
		Log.Warningf("Attaching container %s to weave network failed: %s", container.ID, string(stderr))
		return errors.New(string(stderr))
	} else if len(stderr) > 0 {
		Log.Warningf("Attaching container %s to weave network: %s", container.ID, string(stderr))
	}

	return nil
}
