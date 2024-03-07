package main

import (
	"os"

	docker "github.com/fsouza/go-dockerclient"
)

const defaulDockerAPIVersion = "1.24"
const swarmDockerAPIVersion = "1.26"

// newVersionedDockerClientFromEnv offers some control over the version
// of the docker client weaveutil uses for a number of operations.
// The version specified by the caller, usually defaulDockerAPIVersion,
// may be overridden by setting an environment variable called
// DOCKER_API_VERSION.
func newVersionedDockerClientFromEnv(apiVersionString string) (*docker.Client, error) {
	overridenAPIVersion, ok := os.LookupEnv("DOCKER_API_VERSION")
	if ok && overridenAPIVersion != "" {
		apiVersionString = overridenAPIVersion
	}
	return docker.NewVersionedClientFromEnv(apiVersionString)
}
