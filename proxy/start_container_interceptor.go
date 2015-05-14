package proxy

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct {
	client  *docker.Client
	withDNS bool
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (i *startContainerInterceptor) InterceptResponse(res *http.Response) (*http.Response, error) {
	containerID := containerFromPath(res.Request.URL.Path)

	container, err := i.client.InspectContainer(containerID)
	if err != nil {
		Warning.Print("Error Inspecting Container: ", err)
		return res, nil
	}

	if cidrs, ok := weaveCIDRsFromConfig(container.Config); ok {
		Info.Printf("Container %s was started with CIDR \"%s\"", containerID, strings.Join(cidrs, " "))
		if err := i.attachContainerToWeave(containerID, cidrs); err != nil {
			Warning.Print("Attaching container to weave failed: ", err)
			return res, nil
		}
	} else {
		Debug.Print("No Weave CIDR, ignoring")
	}

	return res, nil
}

func containerFromPath(path string) string {
	if subs := regexp.MustCompile("^/v[0-9\\.]*/containers/([^/]*)/.*").FindStringSubmatch(path); subs != nil {
		return subs[1]
	}
	return ""
}

func (i *startContainerInterceptor) attachContainerToWeave(containerID string, cidrs []string) error {
	args := []string{"attach"}
	args = append(args, cidrs...)
	args = append(args, containerID)
	_, err := callWeave(args...)
	return err
}
