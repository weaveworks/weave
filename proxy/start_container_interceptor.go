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

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) error {
	return nil
}

func (i *startContainerInterceptor) InterceptResponse(r *http.Response) error {
	subs := containerIDRegexp.FindStringSubmatch(r.Request.URL.Path)
	if subs == nil {
		Warning.Printf("No container id found in request with path %s", r.Request.URL.Path)
		return nil
	}
	containerID := subs[1]

	container, err := i.client.InspectContainer(containerID)
	if err != nil {
		Warning.Printf("Error inspecting container %s: %v", containerID, err)
		return nil
	}

	cidrs, ok := weaveCIDRsFromConfig(container.Config)
	if !ok {
		Debug.Print("No Weave CIDR, ignoring")
		return nil
	}
	Info.Printf("Container %s was started with CIDR \"%s\"", containerID, strings.Join(cidrs, " "))
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, containerID)
	if _, err := callWeave(args...); err != nil {
		Warning.Printf("Attaching container %s to weave failed: %v", containerID, err)
		return nil
	}
	return nil
}
