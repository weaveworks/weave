package main

import (
	"testing"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/stretchr/testify/require"
)

func TestParseContainerArgs(t *testing.T) {
	args := []string{"--name", "weave", "--privileged", "--net", "host",
		"-l", "foo=bar", "-l", "foobar",
		"-v", "/var/run/docker.sock:/var/run/docker.sock", "-v", "/etc:/host/etc",
		"--restart", "always", "--pid", "host", "--volumes-from", "weavedb",
		"-e", "WEAVE_DEBUG", "-e", "EXEC_IMAGE=weaveworks/weaveexec:latest",
		"weaveworks/weave:latest", "cmd", "arg1", "arg2"}
	expected := docker.CreateContainerOptions{
		Name: "weave",
		Config: &docker.Config{
			Image:  "weaveworks/weave:latest",
			Env:    []string{"WEAVE_DEBUG", "EXEC_IMAGE=weaveworks/weaveexec:latest"},
			Cmd:    []string{"cmd", "arg1", "arg2"},
			Labels: map[string]string{"foo": "bar", "foobar": ""},
		},
		HostConfig: &docker.HostConfig{
			NetworkMode:   "host",
			PidMode:       "host",
			Privileged:    true,
			RestartPolicy: docker.RestartPolicy{Name: "always"},
			Binds:         []string{"/var/run/docker.sock:/var/run/docker.sock", "/etc:/host/etc"},
			VolumesFrom:   []string{"weavedb"},
		},
	}
	have := parseContainerArgs(args)
	require.Equal(t, expected, have)
}
