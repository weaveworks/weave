package main

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	weave "github.com/rajch/weave/router"
	checkpoint "github.com/weaveworks/go-checkpoint"
)

var checker *checkpoint.Checker
var newVersion atomic.Value
var success atomic.Value

const (
	updateCheckPeriod = 6 * time.Hour
)

func checkForUpdates(dockerVersion string, router *weave.NetworkRouter, clusterSize uint) {
	newVersion.Store("")
	success.Store(true)

	handleResponse := func(r *checkpoint.CheckResponse, err error) {
		if err != nil {
			success.Store(false)
			Log.Printf("Error checking version: %v", err)
			return
		}
		if r.Outdated {
			newVersion.Store(r.CurrentVersion)
			Log.Printf("Weave version %s is available; please update at %s",
				r.CurrentVersion, r.CurrentDownloadURL)
		}
	}

	var uts syscall.Utsname
	syscall.Uname(&uts)

	release := uts.Release[:]
	releaseBytes := make([]byte, len(release))
	i := 0
	for ; i < len(release); i++ {
		if release[i] == 0 {
			break
		}
		releaseBytes[i] = uint8(release[i])
	}
	kernelVersion := string(releaseBytes[:i])

	flags := map[string]string{
		"docker-version": dockerVersion,
		"kernel-version": kernelVersion,
	}

	checkpointKubernetes(context.Background(), flags, clusterSize)

	// Start background version checking
	params := checkpoint.CheckParams{
		Product:       "weave-net",
		Version:       version,
		SignatureFile: "",
		Flags:         flags,
		ExtraFlags:    func() []checkpoint.Flag { return checkpointFlags(router) },
	}
	checker = checkpoint.CheckInterval(&params, updateCheckPeriod, handleResponse)
}

func checkpointFlags(router *weave.NetworkRouter) []checkpoint.Flag {
	flags := []checkpoint.Flag{}
	status := weave.NewNetworkRouterStatus(router)
	for _, conn := range status.Connections {
		if connectionName, ok := conn.Attrs["name"].(string); ok {
			if _, encrypted := conn.Attrs["encrypted"]; encrypted {
				connectionName = connectionName + " encrypted"
			}
			flags = append(flags, checkpoint.Flag{Key: "network", Value: connectionName})
		}
	}
	return flags
}

// checkpoint Kubernetes specific details, passed in from launch script
func checkpointKubernetes(ctx context.Context, flags map[string]string, clusterSize uint) {
	version := os.Getenv("WEAVE_KUBERNETES_VERSION")
	if len(version) == 0 {
		return // not running under Kubernetes
	}
	flags["kubernetes-version"] = version
	flags["kubernetes-cluster-uid"] = string(os.Getenv("WEAVE_KUBERNETES_UID"))
	flags["kubernetes-cluster-size"] = fmt.Sprint(clusterSize)
}
