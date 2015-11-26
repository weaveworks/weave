package plugin

import (
	"github.com/weaveworks/weave/common/docker"
)

type dockerer struct {
	client *docker.Client
}
