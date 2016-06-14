package main

import (
	"sync/atomic"
	"time"

	"github.com/weaveworks/go-checkpoint"
)

var checker *checkpoint.Checker
var newVersion atomic.Value

const (
	updateCheckPeriod = 6 * time.Hour
)

func checkForUpdates() {
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

	// Start background version checking
	params := checkpoint.CheckParams{
		Product:       "weave-net",
		Version:       version,
		SignatureFile: "",
	}
	checker = checkpoint.CheckInterval(&params, updateCheckPeriod, handleResponse)
}
