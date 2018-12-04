package npc

import (
	"log"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/weaveworks/weave/net/ipset"
	coreapi "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type mockSet struct {
	name    ipset.Name
	setType ipset.Type
	subSets map[string]map[types.UID]bool
}

type mockIPSet struct {
	sets map[string]mockSet
}

func newMockIPSet() mockIPSet {
	return mockIPSet{sets: make(map[string]mockSet)}
}

func (i *mockIPSet) Create(ipsetName ipset.Name, ipsetType ipset.Type) error {
	if _, ok := i.sets[string(ipsetName)]; ok {
		return errors.Errorf("ipset %s already exists", ipsetName)
	}
	i.sets[string(ipsetName)] = mockSet{name: ipsetName, setType: ipsetType, subSets: make(map[string]map[types.UID]bool)}
	return nil
}

func (i *mockIPSet) AddEntry(user types.UID, ipsetName ipset.Name, entry string, comment string) error {
	log.Printf("adding entry %s to %s for %s", entry, ipsetName, user)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("%s does not exist", entry)
	}
	if i.sets[string(ipsetName)].subSets[entry] == nil {
		i.sets[string(ipsetName)].subSets[entry] = make(map[types.UID]bool)
	}
	if _, ok := i.sets[string(ipsetName)].subSets[entry][user]; ok {
		return errors.Errorf("user %s already owns entry %s", user, entry)
	}
	i.sets[string(ipsetName)].subSets[entry][user] = true

	return nil
}

func (i *mockIPSet) DelEntry(user types.UID, ipsetName ipset.Name, entry string) error {
	log.Printf("deleting entry %s from %s for %s", entry, ipsetName, user)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("ipset %s does not exist", ipsetName)
	}
	if _, ok := i.sets[string(ipsetName)].subSets[entry][user]; !ok {
		return errors.Errorf("user %s does not own entry %s", user, entry)
	}
	delete(i.sets[string(ipsetName)].subSets[entry], user)

	if len(i.sets[string(ipsetName)].subSets[entry]) == 0 {
		delete(i.sets[string(ipsetName)].subSets, entry)
	}

	return nil
}

func (i *mockIPSet) Exist(user types.UID, ipsetName ipset.Name, entry string) bool {
	_, ok := i.sets[string(ipsetName)].subSets[entry][user]
	return ok
}

func (i *mockIPSet) entryExists(ipsetName ipset.Name, entry string) bool {
	return len(i.sets[string(ipsetName)].subSets[entry]) > 0
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

	require.Equal(t, true, len(m.sets[selectorIPSetName].subSets[sourceIPSetName]) > 0)

	// NetworkPolicy first
	m = newMockIPSet()
	controller = New("foo", false, &mockIPTables{}, &m)

	controller.AddNetworkPolicy(networkPolicy)

	controller.AddNamespace(sourceNamespace)
	controller.AddNamespace(destinationNamespace)

	require.Equal(t, true, len(m.sets[selectorIPSetName].subSets[sourceIPSetName]) > 0)
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
	require.True(t, m.entryExists(defaultAllowIPSetName, fooPodIP))

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
	require.True(t, m.entryExists(defaultAllowIPSetName, barPodIP))

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
	require.False(t, m.entryExists(defaultAllowIPSetName, fooPodIP))
	require.True(t, m.entryExists(defaultAllowIPSetName, barPodIP))

	podBarWithNewIP := *podBar
	podBarWithNewIP.Status.PodIP = barPodNewIP
	controller.UpdatePod(podBar, &podBarWithNewIP)

	// Should update IP addr of the bar pod in default-allow
	require.False(t, m.entryExists(defaultAllowIPSetName, barPodIP))
	require.True(t, m.entryExists(defaultAllowIPSetName, barPodNewIP))

	controller.UpdatePod(&podBarWithNewIP, podBarNoIP)
	// Should remove the bar pod from default-allow as it does not have any IP addr
	require.False(t, m.entryExists(defaultAllowIPSetName, barPodNewIP))

	podFooWithNewLabel := *podFoo
	podFooWithNewLabel.ObjectMeta.Labels = map[string]string{"run": "new-foo"}
	controller.UpdatePod(podFoo, &podFooWithNewLabel)

	// Should bring back the foo pod to default-allow as it does not match dst of any netpol
	require.True(t, m.entryExists(defaultAllowIPSetName, fooPodIP))

	controller.UpdatePod(&podFooWithNewLabel, podFoo)
	// Should remove from default-allow as it matches the netpol after the update
	require.False(t, m.entryExists(defaultAllowIPSetName, fooPodIP))

	controller.DeleteNetworkPolicy(netpol)
	// Should bring back the foo pod to default-allow as no netpol selects it
	require.True(t, m.entryExists(defaultAllowIPSetName, fooPodIP))

	controller.DeletePod(podFoo)
	// Should remove foo pod from default-allow
	require.False(t, m.entryExists(defaultAllowIPSetName, fooPodIP))

	controller.DeleteNamespace(defaultNamespace)
	// Should remove default ipset
	require.NotContains(t, m.sets, defaultAllowIPSetName)

}

func TestOutOfOrderPodEvents(t *testing.T) {
	const (
		defaultAllowIPSetName = "weave-E.1.0W^NGSp]0_t5WwH/]gX@L"
		runBarIPSetName       = "weave-bZ~x=yBgzH)Ht()K*Uv3z{M]Y"
		podIP                 = "10.32.0.10"
	)

	m := newMockIPSet()
	controller := New("qux", false, &mockIPTables{}, &m)

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	controller.AddNamespace(defaultNamespace)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(podFoo)

	// Should be in default-allow as no netpol selects podFoo
	require.True(t, m.entryExists(defaultAllowIPSetName, podIP))

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

	// Shouldn't be in default-allow as netpol above selects podFoo
	require.False(t, m.entryExists(defaultAllowIPSetName, podIP))

	podBar := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "bar",
			Namespace: "default",
			Name:      "bar",
			Labels:    map[string]string{"run": "bar"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(podBar)

	// Should be in default-allow as no netpol selects podBar
	require.True(t, m.entryExists(defaultAllowIPSetName, podIP))
	require.True(t, m.Exist(podBar.ObjectMeta.UID, defaultAllowIPSetName, podIP))
	// Should be in run=bar ipset
	require.True(t, m.entryExists(runBarIPSetName, podIP))

	controller.DeletePod(podFoo)
	// Multiple duplicate events should not affect npc state
	controller.DeletePod(podFoo)
	controller.DeletePod(podFoo)

	// Should be in default-allow as no netpol selects podBar and podFoo removal
	// should not affect podBar in default-allow
	require.True(t, m.entryExists(defaultAllowIPSetName, podIP))

	controller.DeletePod(podBar)

	// Should remove from default-allow and run=bar ipsets
	require.Equal(t, 0, len(m.sets[defaultAllowIPSetName].subSets))
	require.False(t, m.entryExists(runBarIPSetName, podIP))
}

// Test case for https://github.com/weaveworks/weave/issues/3222
func TestNewDstSelector(t *testing.T) {
	const (
		defaultAllowIPSetName = "weave-E.1.0W^NGSp]0_t5WwH/]gX@L"
		podIP                 = "10.32.0.10"
	)

	m := newMockIPSet()
	controller := New("baz", false, &mockIPTables{}, &m)

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	controller.AddNamespace(defaultNamespace)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(podFoo)

	require.True(t, m.entryExists(defaultAllowIPSetName, podIP))

	netpolBar := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-bar",
			Name:      "allow-from-default-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(netpolBar)

	// netpolBar dst selector selects podFoo
	require.False(t, m.entryExists(defaultAllowIPSetName, podIP))

	netpolFoo := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-foo",
			Name:      "allow-from-default-2",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(netpolFoo)

	controller.DeleteNetworkPolicy(netpolBar)
	// netpolFoo dst-selects podFoo
	require.False(t, m.entryExists(defaultAllowIPSetName, podIP))
	controller.DeleteNetworkPolicy(netpolFoo)
	// No netpol dst-selects podFoo
	require.True(t, m.entryExists(defaultAllowIPSetName, podIP))
}
