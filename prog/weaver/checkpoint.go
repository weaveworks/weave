package main

import (
	"sync/atomic"
	"syscall"
	"time"

	"github.com/weaveworks/go-checkpoint"
)

var checker *checkpoint.Checker
var newVersion atomic.Value

const (
	updateCheckPeriod = 6 * time.Hour
)

func checkForUpdates(dockerVersion string, network string) {
	newVersion.Store("")

	handleResponse := func(r *checkpoint.CheckResponse, err error) {
		if err != nil {
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
	var release interface{}
	var kernelVersion = ""

	syscall.Uname(&uts)
	release = uts.Release[:]

	switch release.(type) {
	case []int8:
		kernelVersion = intsToString(release.([]int8))
	case []uint8:
		kernelVersion = uintsToString(release.([]uint8))
	default:
		kernelVersion = "unknown"
	}

	flags := map[string]string{
		"docker-version": dockerVersion,
		"kernel-version": kernelVersion,
	}
	if network != "" {
		flags["network"] = network
	}

	// Start background version checking
	params := checkpoint.CheckParams{
		Product:       "weave-net",
		Version:       version,
		SignatureFile: "",
		Flags:         flags,
	}
	checker = checkpoint.CheckInterval(&params, updateCheckPeriod, handleResponse)
}

func intsToString(ca []int8) string {
	s := make([]byte, len(ca))
	i := 0
	for ; i < len(ca); i++ {
		if ca[i] == 0 {
			break
		}
		s[i] = uint8(ca[i])
	}
	return string(s[:i])
}

func uintsToString(ca []uint8) string {
	s := make([]byte, len(ca))
	i := 0
	for ; i < len(ca); i++ {
		if ca[i] == 0 {
			break
		}
		s[i] = uint8(ca[i])
	}
	return string(s[:i])
}
