package main

import (
	"sync/atomic"
	"syscall"
	"time"

	"github.com/weaveworks/go-checkpoint"
	weave "github.com/weaveworks/weave/router"
)

var checker *checkpoint.Checker
var newVersion atomic.Value
var success atomic.Value

const (
	updateCheckPeriod = 6 * time.Hour
)

func checkForUpdates(dockerVersion string, router *weave.NetworkRouter) {
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
