package npc

import (
	"log"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/weave/npc/ipset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	coreapi "k8s.io/client-go/pkg/api/v1"
	extnapi "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type mockSet struct {
	name    ipset.Name
	setType ipset.Type
	subSets map[string]bool
}

type mockIPSet struct {
	sets map[string]mockSet
}

func newMockIPSet() mockIPSet {
	i := mockIPSet{
		sets: make(map[string]mockSet),
	}

	return i
}

func (i *mockIPSet) Create(ipsetName ipset.Name, ipsetType ipset.Type) error {
	if _, ok := i.sets[string(ipsetName)]; ok {
		return errors.Errorf("ipset %s already exists", ipsetName)
	}
	i.sets[string(ipsetName)] = mockSet{name: ipsetName, setType: ipsetType, subSets: make(map[string]bool)}
	return nil
}

func (i *mockIPSet) AddEntry(ipsetName ipset.Name, entry string, comment string) error {
	log.Printf("adding entry %s to %s", entry, ipsetName)
	if _, ok := i.sets[entry]; !ok {
		return errors.Errorf("ipset %s does not exist", entry)
	}
	if _, ok := i.sets[string(ipsetName)].subSets[entry]; ok {
		return errors.Errorf("ipset %s is already a member of %s", entry, ipsetName)
	}
	i.sets[string(ipsetName)].subSets[entry] = true

	return nil
}

func (i *mockIPSet) DelEntry(ipsetName ipset.Name, entry string) error {
	log.Printf("deleting entry %s from %s", entry, ipsetName)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("ipset %s does not exist", ipsetName)
	}
	if _, ok := i.sets[string(ipsetName)].subSets[entry]; !ok {
		return errors.Errorf("ipset %s is not a member of %s", entry, ipsetName)
	}
	delete(i.sets[string(ipsetName)].subSets, entry)

	return nil
}

func (i *mockIPSet) Flush(ipsetName ipset.Name) error {
	return errors.New("Not Implemented")
}

func (i *mockIPSet) FlushAll() error {
	return errors.New("Not Implemented")
}

func (i *mockIPSet) Destroy(ipsetName ipset.Name) error {
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("ipset %s does not exist", ipsetName)
	}
	delete(i.sets, string(ipsetName))
	return nil
}

func (i *mockIPSet) DestroyAll() error {
	return errors.New("Not Implemented")
}

func (i *mockIPSet) List(prefix string) ([]ipset.Name, error) {
	return []ipset.Name{}, errors.New("Not Implemented")
}

type mockIPTables struct {
}

func (ipt *mockIPTables) Append(table, chain string, rulespec ...string) error {
	return nil
}

func (ipt *mockIPTables) Delete(table, chain string, rulespec ...string) error {
	return nil
}

func (ipt *mockIPTables) Insert(table, chain string, pos int, rulespec ...string) error {
	return nil
}

func TestRegressionPolicyNamespaceOrdering3059(t *testing.T) {
	// Test for race condition between namespace and networkpolicy events
	// https://github.com/weaveworks/weave/issues/3059

	sourceNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "source",
			Labels: map[string]string{
				"app": "source",
			},
		},
	}

	destinationNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "destination",
		},
	}

	port := intstr.FromInt(12345)

	networkPolicy := &extnapi.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-policy",
			Namespace: "destination",
		},
		Spec: extnapi.NetworkPolicySpec{
			Ingress: []extnapi.NetworkPolicyIngressRule{
				{
					From: []extnapi.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "source",
								},
							},
						},
					},
					Ports: []extnapi.NetworkPolicyPort{
						{
							Port: &port,
						},
					},
				},
			},
		},
	}

	// Namespaces first
	m := newMockIPSet()
	controller := New("foo", true, &mockIPTables{}, &m)

	const (
		selectorIPSetName = "weave-I239Zp%sCvoVt*D6u=A!2]YEk"
		sourceIPSetName   = "weave-HboJG1fGgG]/SR%k9H#hv5e96"
	)

	controller.AddNamespace(sourceNamespace)
	controller.AddNamespace(destinationNamespace)

	controller.AddNetworkPolicy(networkPolicy)

	require.Equal(t, true, m.sets[selectorIPSetName].subSets[sourceIPSetName])

	// NetworkPolicy first
	m = newMockIPSet()
	controller = New("foo", true, &mockIPTables{}, &m)

	controller.AddNetworkPolicy(networkPolicy)

	controller.AddNamespace(sourceNamespace)
	controller.AddNamespace(destinationNamespace)

	require.Equal(t, true, m.sets[selectorIPSetName].subSets[sourceIPSetName])

}
