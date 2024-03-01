/*
This module deals with operations on the peerlist backed by Kubernetes' annotation mechanism.
*/
package main

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes"

	"github.com/rajch/weave/common"
)

const (
	configMapName      = "weave-net"
	configMapNamespace = "kube-system"
)

type peerList struct {
	Peers []peerInfo
}

type peerInfo struct {
	PeerName string // Weave internal unique ID
	NodeName string // Kubernetes node name
}

func (pl *peerList) contains(peerName string) bool {
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

func (cml *configMapAnnotations) UpdatePeerList(ctx context.Context, list peerList) error {
	recordBytes, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return cml.UpdateAnnotation(ctx, KubePeersAnnotationKey, string(recordBytes))
}

// update the list of all peers that have gone through this code path
func addMyselfToPeerList(cml *configMapAnnotations, c kubernetes.Interface, peerName, name string) (*peerList, error) {
	var list *peerList
	err := cml.LoopUpdate(func() error {
		var err error
		list, err = cml.GetPeerList()
		if err != nil {
			return err
		}
		if !list.contains(peerName) {
			list.add(peerName, name)
			err = cml.UpdatePeerList(context.Background(), *list)
			if err != nil {
				return err
			}
		}
		return nil
	})
	return list, err
}

func checkIamInPeerList(cml *configMapAnnotations, c kubernetes.Interface, peerName string) (bool, error) {
	if err := cml.Init(); err != nil {
		return false, err
	}
	list, err := cml.GetPeerList()
	if err != nil {
		return false, err
	}
	common.Log.Debugf("[kube-peers] Checking peer %q against list %v", peerName, list)
	return list.contains(peerName), nil
}
