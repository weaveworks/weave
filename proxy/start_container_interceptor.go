package proxy

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

var containerIDRegexp = regexp.MustCompile("^/v[0-9\\.]*/containers/([^/]*)/.*")

type startContainerInterceptor struct {
	client  *docker.Client
	withDNS bool
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) (*http.Response, error) {
	containerID := ""
	if subs := containerIDRegexp.FindStringSubmatch(r.Request.URL.Path); subs == nil {
		Warning.Printf("No container id found in request with path %s", r.Request.URL.Path)
		return r, nil
	} else {
		containerID = subs[1]
	}

	container, err := i.client.InspectContainer(containerID)
	if err != nil {
		Warning.Printf("Error inspecting container %s: %v", containerID, err)
		return r, nil
	}

	cidrs, ok := weaveCIDRsFromConfig(container.Config)
	if !ok {
		Debug.Print("No Weave CIDR, ignoring")
		return r, nil
	}
	Info.Printf("Container %s was started with CIDR \"%s\"", containerID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, containerID)
	if _, err := callWeave(args...); err != nil {
		Warning.Printf("Attaching container %s to weave failed: %v", containerID, err)
		return r, nil
	}
	return r, nil
}
