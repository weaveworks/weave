package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct {
	client *docker.Client
}

func StartContainerInterceptor(client *docker.Client) Interceptor {
	return &startContainerInterceptor{
		client: client,
	}
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (i *startContainerInterceptor) InterceptResponse(res *http.Response) (*http.Response, error) {
	containerId := containerFromPath(res.Request.URL.Path)

	container, err := i.client.InspectContainer(containerId)
	if err != nil {
		Warning.Print("Error Inspecting Container: ", err)
		return res, nil
	}

	if cidr, ok := weaveAddrFromConfig(container.Config); ok {
		Info.Printf("Container %s was started with CIDR \"%s\"", containerId, cidr)
		args := []string{"attach"}
		args = append(args, strings.Split(strings.TrimSpace(cidr), " ")...)
		args = append(args, containerId)
		if out, err := callWeave(args...); err != nil {
			Warning.Print("Calling weave failed: ", err, string(out))
		}
	} else {
		Debug.Print("No Weave CIDR, ignoring")
	}

	return res, nil
}

func containerInfo(transport http.RoundTripper, containerId string) (body map[string]interface{}, err error) {
	body = nil
	client := &http.Client{
		Transport: transport,
	}
	if res, err := client.Get("http://localhost/v1.16/containers/" + containerId + "/json"); err == nil {
		if bs, err := ioutil.ReadAll(res.Body); err == nil {
			err = json.Unmarshal(bs, &body)
		} else {
			Warning.Print("Could not parse response from docker", err)
		}
	} else {
		Warning.Print("Error fetching container info from docker", err)
	}
	return
}

func containerFromPath(path string) string {
	if subs := regexp.MustCompile("^/v[0-9\\.]*/containers/([^/]*)/.*").FindStringSubmatch(path); subs != nil {
		return subs[1]
	}
	return ""
}

func callWeave(args ...string) ([]byte, error) {
	args = append([]string{"--local"}, args...)
	Debug.Print("Calling weave", args)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PROCFS=/hostproc", "PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	out, err := cmd.CombinedOutput()
	return out, err
}

func weaveAddrFromConfig(config *docker.Config) (string, bool) {
	for _, e := range config.Env {
		if strings.HasPrefix(e, "WEAVE_CIDR=") {
			result := strings.Trim(e[11:], " ")
			return result, result != ""
		}
	}
	return "", false
}
