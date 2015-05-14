package proxy

import (
	"os/exec"
	"strings"

	"github.com/fsouza/go-dockerclient"
	. "github.com/weaveworks/weave/common"
)

func callWeave(args ...string) ([]byte, error) {
	args = append([]string{"--local"}, args...)
	Debug.Print("Calling weave", args)
	cmd := exec.Command("./weave", args...)
	cmd.Env = []string{"PROCFS=/hostproc", "PATH=/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	out, err := cmd.CombinedOutput()
	return out, err
}

func weaveCIDRsFromConfig(config *docker.Config) ([]string, bool) {
	for _, e := range config.Env {
		if strings.HasPrefix(e, "WEAVE_CIDR=") {
			result := strings.Trim(e[11:], " ")
			return strings.Split(strings.TrimSpace(result), " "), result != ""
		}
	}
	return nil, false
}
