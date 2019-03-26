/*
In order to keep track of active weave peers, we use annotations on the Kubernetes cluster.

Kubernetes uses etcd to distribute and synchronise these annotations so we don't have to.
*/
package main

import (
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

func newConfigMapAnnotations(ns string, configMapName string, c kubernetes.Interface) *configMapAnnotations {
	return &configMapAnnotations{
		Namespace:     ns,
		ConfigMapName: configMapName,
		Client:        c.CoreV1(),
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
	if cml.cm == nil || cml.cm.Annotations == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server, or nil if error
	cml.cm.Annotations[cleanKey(key)] = value
	cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	return err
}

func (cml *configMapAnnotations) RemoveAnnotation(key string) (err error) {
	if cml.cm == nil || cml.cm.Annotations == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server, or nil if error
	delete(cml.cm.Annotations, cleanKey(key))
	cml.cm, err = cml.Client.ConfigMaps(cml.Namespace).Update(cml.cm)
	return err
}

func (cml *configMapAnnotations) RemoveAnnotationsWithValue(valueToRemove string) (err error) {
	if cml.cm == nil || cml.cm.Annotations == nil {
		return errors.New("endpoint not initialized, call Init first")
	}
	// speculatively change the state, then replace with whatever comes back
	// from Update(), which will be the latest on the server, or nil if error
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
