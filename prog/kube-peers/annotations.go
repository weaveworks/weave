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

	v1 "k8s.io/api/core/v1"
	kubeErrors "k8s.io/apimachinery/pkg/api/errors"
	api "k8s.io/apimachinery/pkg/apis/meta/v1"
	wait "k8s.io/apimachinery/pkg/util/wait"
	kubernetes "k8s.io/client-go/kubernetes"
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
}

func (pl peerList) contains(peerName string) bool {
	for _, peer := range pl.Peers {
		if peer.PeerName == peerName {
			return true
		}
	}
	return false
}

func (pl *peerList) add(peerName string, name string) {
	pl.Peers = append(pl.Peers, peerInfo{PeerName: peerName, NodeName: name})
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

	// KubePeersAnnotationKey is the default annotation key
	KubePeersAnnotationKey = "kube-peers.weave.works/peers"
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

func (cml *configMapAnnotations) UpdateAnnotation(key, value string) error {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	cm := cml.cm
	cm.Annotations[key] = value
	cm, err := cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	if err == nil {
		cml.cm = cm
	}
	return err
}

func (cml *configMapAnnotations) RemoveAnnotation(key string) error {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	cm := cml.cm
	delete(cm.Annotations, key)
	cm, err := cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	if err == nil {
		cml.cm = cm
	}
	return err
}

func (cml *configMapAnnotations) RemoveAnnotationsWithValue(valueToRemove string) error {
	if cml.cm == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	cm := cml.cm
	for key, value := range cm.Annotations {
		if value == valueToRemove {
			delete(cm.Annotations, key)
		}
	}
	cm, err := cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	if err == nil {
		cml.cm = cm
	}
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
