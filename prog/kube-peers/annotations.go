/*
In order to keep track of active weave peers, we use annotations on the Kubernetes cluster.

Kubernetes uses etcd to distribute and synchronise these annotations so we don't have to.

This module deals with operations on the peerlist backed by Kubernetes' annotation mechanism.
*/
package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/pkg/errors"

	"k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

type configMapAnnotations struct {
	ConfigMapName string
	Namespace     string
	Client        corev1client.ConfigMapsGetter
	cm            *v1.ConfigMap
}

func newConfigMapAnnotations(ns string, configMapName string, clientset *kubernetes.Clientset) *configMapAnnotations {
	return &configMapAnnotations{
		Namespace:     ns,
		ConfigMapName: configMapName,
		Client:        clientset.CoreV1(),
	}
}

type peerList struct {
	Peers []peerInfo
}

type peerInfo struct {
	PeerName string // Weave internal unique ID
	NodeName string // Kubernetes node name
	BridgeIP string // Weave bridge IP. Could be empty.
}

func (pl peerList) contains(peerName string) bool {
	for _, peer := range pl.Peers {
		if peer.PeerName == peerName {
			return true
		}
	}
	return false
}

func (pl peerList) index(peerName, name, bridgeIP string) (int, bool) {
	for i, peer := range pl.Peers {
		if peer.PeerName == peerName {
			return i, peer.NodeName == name && peer.BridgeIP == bridgeIP
		}
	}

	return -1, false
}

func (pl *peerList) add(peerName, name, bridgeIP string) {
	pl.Peers = append(pl.Peers, peerInfo{PeerName: peerName, NodeName: name, BridgeIP: bridgeIP})
}

func (pl *peerList) replace(i int, peerName, name, bridgeIP string) {
	pl.Peers[i] = peerInfo{PeerName: peerName, NodeName: name, BridgeIP: bridgeIP}
}

func (pl *peerList) remove(peerNameToRemove string) {
	for i := 0; i < len(pl.Peers); {
		if pl.Peers[i].PeerName == peerNameToRemove {
			pl.Peers = append(pl.Peers[:i], pl.Peers[i+1:]...)
		} else {
			i++
		}
	}
}

const (
	retryPeriod  = time.Second * 2
	jitterFactor = 1.0

	// Prefix all our annotation keys with this string so they don't clash with anyone else's
	KubePeersPrefix = "kube-peers.weave.works/"
	// KubePeersAnnotationKey is the default annotation key
	KubePeersAnnotationKey = KubePeersPrefix + "peers"
)

func (cml *configMapAnnotations) Init() error {
	for {
		// Since it's potentially racy to GET, then CREATE if not found, we wrap in a check loop
		// so that if the configmap is created after our GET but before or CREATE, we'll gracefully
		// re-try to get the configmap.
		var err error
		cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Get(cml.ConfigMapName, api.GetOptions{})
		if err != nil {
			if !kubeErrors.IsNotFound(err) {
				return errors.Wrapf(err, "Unable to fetch ConfigMap %s/%s", cml.Namespace, cml.ConfigMapName)
			}
			cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Create(&v1.ConfigMap{
				ObjectMeta: api.ObjectMeta{
					Name:      cml.ConfigMapName,
					Namespace: cml.Namespace,
				},
			})
			if err != nil {
				if kubeErrors.IsAlreadyExists(err) {
					continue
				}
				return errors.Wrapf(err, "Unable to create ConfigMap %s/%s", cml.Namespace, cml.ConfigMapName)
			}
		}
		break
	}
	if cml.cm.Annotations == nil {
		cml.cm.Annotations = make(map[string]string)
	}
	return nil
}

func (cml *configMapAnnotations) GetPeerList() (*peerList, error) {
	var record peerList
	if cml.cm == nil {
		return nil, errors.New("endpoint not initialized, call Init first")
	}
	if recordBytes, found := cml.cm.Annotations[KubePeersAnnotationKey]; found {
		if err := json.Unmarshal([]byte(recordBytes), &record); err != nil {
			return nil, err
		}
	}
	return &record, nil
}

func (cml *configMapAnnotations) UpdatePeerList(list peerList) error {
	recordBytes, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return cml.UpdateAnnotation(KubePeersAnnotationKey, string(recordBytes))
}

// Clean up a string so it meets the Kubernetes requiremements for Annotation keys:
// name part must consist of alphanumeric characters, '-', '_' or '.', and must
// start and end with an alphanumeric character (e.g. 'MyName', or 'my.name', or '123-abc')
func cleanKey(key string) string {
	buf := []byte(key)
	for i, c := range buf {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '/' {
			continue
		}
		buf[i] = '_'
	}
	return string(buf)
}

func (cml *configMapAnnotations) GetAnnotation(key string) (string, bool) {
	value, ok := cml.cm.Annotations[cleanKey(key)]
	return value, ok
}

func (cml *configMapAnnotations) UpdateAnnotation(key, value string) (err error) {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server whatever happened
	cml.cm.Annotations[cleanKey(key)] = value
	cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	return err
}

func (cml *configMapAnnotations) RemoveAnnotation(key string) (err error) {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server whatever happened
	delete(cml.cm.Annotations, cleanKey(key))
	cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	return err
}

func (cml *configMapAnnotations) RemoveAnnotationsWithValue(valueToRemove string) (err error) {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server whatever happened
	for key, value := range cml.cm.Annotations {
		if value == valueToRemove {
			delete(cml.cm.Annotations, key) // don't need to clean this key as it came from the map
		}
	}
	cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	return err
}

// Loop with jitter, fetching the cml data and calling f() until it
// doesn't get an optimistic locking conflict.
// If it succeeds or gets any other kind of error, stop the loop.
func (cml *configMapAnnotations) LoopUpdate(f func() error) error {
	stop := make(chan struct{})
	var err error
	wait.JitterUntil(func() {
		if err = cml.Init(); err != nil {
			close(stop)
			return
		}
		err = f()
		if err != nil && kubeErrors.IsConflict(err) {
			log.Printf("Optimistic locking conflict: trying again: %s", err)
			return
		}
		close(stop)
	}, retryPeriod, jitterFactor, true, stop)
	return err
}
