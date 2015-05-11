package proxy

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	. "github.com/weaveworks/weave/common"
)

type startContainerInterceptor struct {
	*http.Transport
}

func StartContainerInterceptor(t *http.Transport) Interceptor {
	return &startContainerInterceptor{
		Transport: t,
	}
}

func (i *startContainerInterceptor) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (i *startContainerInterceptor) InterceptResponse(res *http.Response) (*http.Response, error) {
	containerId := containerFromPath(res.Request.URL.Path)
	if info, err := containerInfo(i, containerId); err == nil {
		if cidr, ok := weaveAddrFromConfig(info["Config"].(map[string]interface{})); ok {
			Info.Printf("Container %s was started with CIDR \"%s\"", containerId, cidr)
			if out, err := callWeave("attach", cidr, containerId); err != nil {
				Warning.Print("Calling weave failed:", err, string(out))
			}
		} else {
			Debug.Print("No Weave CIDR, ignoring")
		}
	} else {
		Warning.Print("Cound not parse container config from request", err)
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

func weaveAddrFromConfig(config map[string]interface{}) (string, bool) {
	env, found := config["Env"]
	if !found || env == nil {
		return "", false
	}

	entries, ok := env.([]interface{})
	if !ok {
		Warning.Print("Unexpected format for config: ", config)
		return "", false
	}

	for _, e := range entries {
		entry, entryIsAString := e.(string)
		if entryIsAString && strings.HasPrefix(entry, "WEAVE_CIDR=") {
			return strings.Trim(entry[11:], " "), true
		}
	}
	return "", false
}
