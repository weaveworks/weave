package npc

import (
	"log"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/weave/npc/ipset"
	coreapi "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	return i.addEntry(ipsetName, entry, comment, true)
}

func (i *mockIPSet) AddEntryIfNotExist(ipsetName ipset.Name, entry string, comment string) error {
	return i.addEntry(ipsetName, entry, comment, false)
}

func (i *mockIPSet) addEntry(ipsetName ipset.Name, entry string, comment string, checkIfExists bool) error {
	log.Printf("adding entry %s to %s", entry, ipsetName)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("ipset %s does not exist", entry)
	}
	if checkIfExists {
		if _, ok := i.sets[string(ipsetName)].subSets[entry]; ok {
			return errors.Errorf("ipset %s is already a member of %s", entry, ipsetName)
		}
	}
	i.sets[string(ipsetName)].subSets[entry] = true

	return nil
}

func (i *mockIPSet) DelEntry(ipsetName ipset.Name, entry string) error {
	return i.delEntry(ipsetName, entry, true)
}

func (i *mockIPSet) DelEntryIfExists(ipsetName ipset.Name, entry string) error {
	return i.delEntry(ipsetName, entry, false)
}

func (i *mockIPSet) delEntry(ipsetName ipset.Name, entry string, checkIfExists bool) error {
	log.Printf("deleting entry %s from %s", entry, ipsetName)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("ipset %s does not exist", ipsetName)
	}
	if checkIfExists {
		if _, ok := i.sets[string(ipsetName)].subSets[entry]; !ok {
			return errors.Errorf("ipset %s is not a member of %s", entry, ipsetName)
		}
	}
	delete(i.sets[string(ipsetName)].subSets, entry)

	return nil
}

func (i *mockIPSet) Exist(ipsetName ipset.Name, entry string) bool {
	_, found := i.sets[string(ipsetName)].subSets[entry]
	return found
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

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-policy",
			Namespace: "destination",
		},
		Spec: networkingv1.NetworkPolicySpec{
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"app": "source",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
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
	controller := New("foo", false, &mockIPTables{}, &m)

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
	controller = New("foo", false, &mockIPTables{}, &m)

	controller.AddNetworkPolicy(networkPolicy)

	controller.AddNamespace(sourceNamespace)
	controller.AddNamespace(destinationNamespace)

	require.Equal(t, true, m.sets[selectorIPSetName].subSets[sourceIPSetName])

}

// Tests default-allow ipset behavior when running in non-legacy mode.
func TestDefaultAllow(t *testing.T) {
	const (
		defaultAllowIPSetName = "weave-E.1.0W^NGSp]0_t5WwH/]gX@L"
		fooPodIP              = "10.32.0.10"
		barPodIP              = "10.32.0.11"
		barPodNewIP           = "10.32.0.12"
	)

	m := newMockIPSet()
	controller := New("bar", false, &mockIPTables{}, &m)

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	controller.AddNamespace(defaultNamespace)

	// Should create an ipset for default-allow
	require.Contains(t, m.sets, defaultAllowIPSetName)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: fooPodIP}}
	controller.AddPod(podFoo)

	// Should add the foo pod to default-allow
	require.True(t, m.Exist(defaultAllowIPSetName, fooPodIP))

	podBar := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "bar",
			Namespace: "default",
			Name:      "bar",
			Labels:    map[string]string{"run": "bar"}},
		Status: coreapi.PodStatus{PodIP: barPodIP}}
	podBarNoIP := &coreapi.Pod{ObjectMeta: podBar.ObjectMeta}
	controller.AddPod(podBarNoIP)
	controller.UpdatePod(podBarNoIP, podBar)

	// Should add the bar pod to default-allow
	require.True(t, m.Exist(defaultAllowIPSetName, barPodIP))

	// Allow access from the bar pod to the foo pod
	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-from-bar-to-foo",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"run": "foo"}},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"run": "bar"},
					},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(netpol)

	// Should remove the foo pod from default-allow as the netpol selects it
	require.False(t, m.Exist(defaultAllowIPSetName, fooPodIP))
	require.True(t, m.Exist(defaultAllowIPSetName, barPodIP))

	podBarWithNewIP := *podBar
	podBarWithNewIP.Status.PodIP = barPodNewIP
	controller.UpdatePod(podBar, &podBarWithNewIP)

	// Should update IP addr of the bar pod in default-allow
	require.False(t, m.Exist(defaultAllowIPSetName, barPodIP))
	require.True(t, m.Exist(defaultAllowIPSetName, barPodNewIP))

	controller.UpdatePod(&podBarWithNewIP, podBarNoIP)
	// Should remove the bar pod from default-allow as it does not have any IP addr
	require.False(t, m.Exist(defaultAllowIPSetName, barPodNewIP))

	podFooWithNewLabel := *podFoo
	podFooWithNewLabel.ObjectMeta.Labels = map[string]string{"run": "new-foo"}
	controller.UpdatePod(podFoo, &podFooWithNewLabel)

	// Should bring back the foo pod to default-allow as it does not match dst of any netpol
	require.True(t, m.Exist(defaultAllowIPSetName, fooPodIP))

	controller.UpdatePod(&podFooWithNewLabel, podFoo)
	// Should remove from default-allow as it matches the netpol after the update
	require.False(t, m.Exist(defaultAllowIPSetName, fooPodIP))

	controller.DeleteNetworkPolicy(netpol)
	// Should bring back the foo pod to default-allow as no netpol selects it
	require.True(t, m.Exist(defaultAllowIPSetName, fooPodIP))

	controller.DeletePod(podFoo)
	// Should remove foo pod to default-allow
	require.False(t, m.Exist(defaultAllowIPSetName, fooPodIP))
}
