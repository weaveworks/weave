package main

import (
	"os"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	checkpoint "github.com/weaveworks/go-checkpoint"
	weave "github.com/weaveworks/weave/router"
	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var checker *checkpoint.Checker
var newVersion atomic.Value
var success atomic.Value

const (
	updateCheckPeriod  = 6 * time.Hour
	configMapName      = "weave-net"
	configMapNamespace = "kube-system"
)

func checkForUpdates(dockerVersion string, router *weave.NetworkRouter, peers []string) {
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

	flags := make(map[string]string)
	flags["docker-version"] = dockerVersion
	flags["kernel-version"] = kernelVersion

	checkpointKubernetes(flags, peers)

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

// checkpoint Kubernetes specific details
func checkpointKubernetes(flags map[string]string, peers []string) {
	// checks if weaver is running in Kubernetes
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if len(host) == 0 {
		return
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		Log.Printf("Could not get Kubernetes in-cluster config: %v", err)
		return
	}
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		Log.Printf("Could not make Kubernetes client: %v", err)
		return
	}
	k8sVersion, err := c.Discovery().ServerVersion()
	if err != nil {
		Log.Printf("Could not get Kubernetes version: %v", err)
		return
	}
	flags["kubernetes-version"] = k8sVersion.String()

	// use UID of `weave-net` configmap as unique ID of the Kubenerets cluster
	cm, err := c.CoreV1().ConfigMaps(configMapNamespace).Get(configMapName, api.GetOptions{})
	if err != nil {
		Log.Printf("Unable to fetch ConfigMap %s/%s to infer unique cluster ID", configMapNamespace, configMapName)
		return
	}
	flags["kubernetes-cluster-uid"] = string(cm.ObjectMeta.UID)
	flags["kubernetes-cluster-size"] = strconv.Itoa(len(peers))
}
