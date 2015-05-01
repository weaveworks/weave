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

func containerInfo(proxy *proxy, containerId string) (body map[string]interface{}, err error) {
	body = nil
	client := &http.Client{
		Transport: proxy,
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

func isCreate(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/create$", r.URL.Path)
	return err == nil && ok
}

func isStart(r *http.Request) bool {
	ok, err := regexp.MatchString("^/v[0-9\\.]*/containers/[^/]*/start$", r.URL.Path)
	return err == nil && ok
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

func (proxy *proxy) InterceptRequest(r *http.Request) (*http.Request, error) {
	return r, nil
}

func (proxy *proxy) InterceptResponse(req *http.Request, res *http.Response) *http.Response {
	if isStart(req) {
		containerId := containerFromPath(req.URL.Path)
		if info, err := containerInfo(proxy, containerId); err == nil {
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
	}
	return res
}
