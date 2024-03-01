package npc

import (
	"context"
	"fmt"
	"log"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/rajch/weave/common/chains"
	"github.com/rajch/weave/net/ipset"
	"github.com/stretchr/testify/require"
	coreapi "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

type mockSet struct {
	name    ipset.Name
	setType ipset.Type
	subSets map[string]map[ipset.UID]bool
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
	i.sets[string(ipsetName)] = mockSet{name: ipsetName, setType: ipsetType, subSets: make(map[string]map[ipset.UID]bool)}
	return nil
}

func (i *mockIPSet) AddEntry(user ipset.UID, ipsetName ipset.Name, entry string, comment string) error {
	log.Printf("adding entry %s to %s for %s", entry, ipsetName, user)
	if _, ok := i.sets[string(ipsetName)]; !ok {
		return errors.Errorf("%s does not exist", entry)
	}
	if i.sets[string(ipsetName)].subSets[entry] == nil {
		i.sets[string(ipsetName)].subSets[entry] = make(map[ipset.UID]bool)
	}
	if _, ok := i.sets[string(ipsetName)].subSets[entry][user]; ok {
		return errors.Errorf("user %s already owns entry %s", user, entry)
	}
	i.sets[string(ipsetName)].subSets[entry][user] = true

	return nil
}

func (i *mockIPSet) DelEntry(user ipset.UID, ipsetName ipset.Name, entry string) error {
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

func (i *mockIPSet) EntryExists(user ipset.UID, ipsetName ipset.Name, entry string) bool {
	_, ok := i.sets[string(ipsetName)].subSets[entry][user]
	return ok
}

func (i *mockIPSet) Exists(ipsetName ipset.Name) (bool, error) {
	_, ok := i.sets[string(ipsetName)]
	return ok, nil
}

func (i *mockIPSet) entriesExist(ipsetName ipset.Name, entry string) bool {
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
	rules map[string]map[string]struct{} // chain -> rulespec -> struct{}
}

func newMockIPTables() *mockIPTables {
	return &mockIPTables{rules: make(map[string]map[string]struct{})}
}

func (ipt *mockIPTables) Append(table, chain string, rulespec ...string) error {
	if table != TableFilter {
		return fmt.Errorf("invalid table: %q", table)
	}

	if _, found := ipt.rules[chain]; !found {
		ipt.rules[chain] = make(map[string]struct{})
	}

	rule := strings.Join(rulespec, " ")
	if _, found := ipt.rules[chain][rule]; found {
		return fmt.Errorf("rule already exists. chain: %q, rule: %q", chain, rule)
	}

	ipt.rules[chain][rule] = struct{}{}

	return nil
}

func (ipt *mockIPTables) Delete(table, chain string, rulespec ...string) error {
	rule := strings.Join(rulespec, " ")

	if _, found := ipt.rules[chain][rule]; !found {
		return fmt.Errorf("rule does not exist. chain: %q rule: %q", chain, rule)
	}

	delete(ipt.rules[chain], rule)

	return nil
}

func (ipt *mockIPTables) Insert(table, chain string, pos int, rulespec ...string) error {
	return errors.New("Not Implemented")
}

func TestRegressionPolicyNamespaceOrdering3059(t *testing.T) {
	// Test for race condition between namespace and networkpolicy events
	// https://github.com/rajch/weave/issues/3059

	sourceNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "source",
			UID:  "source",
			Labels: map[string]string{
				"app": "source",
			},
		},
	}

	destinationNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "destination",
			UID:  "destination",
		},
	}

	port := intstr.FromInt(12345)

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "network-policy",
			Namespace: "destination",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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
	client := fake.NewSimpleClientset()
	ctx := context.Background()
	controller := New("foo", newMockIPTables(), &m, client)
	client.CoreV1().Namespaces().Create(ctx, sourceNamespace, metav1.CreateOptions{})
	client.CoreV1().Namespaces().Create(ctx, destinationNamespace, metav1.CreateOptions{})

	const (
		selectorIPSetName = "weave-I239Zp%sCvoVt*D6u=A!2]YEk"
		sourceIPSetName   = "weave-HboJG1fGgG]/SR%k9H#hv5e96"
	)

	controller.AddNamespace(ctx, sourceNamespace)
	controller.AddNamespace(ctx, destinationNamespace)

	controller.AddNetworkPolicy(ctx, networkPolicy)

	require.Equal(t, true, len(m.sets[selectorIPSetName].subSets[sourceIPSetName]) > 0)

	// NetworkPolicy first
	m = newMockIPSet()
	controller = New("foo", newMockIPTables(), &m, &fake.Clientset{})

	controller.AddNetworkPolicy(ctx, networkPolicy)

	controller.AddNamespace(ctx, sourceNamespace)
	controller.AddNamespace(ctx, destinationNamespace)

	require.Equal(t, true, len(m.sets[selectorIPSetName].subSets[sourceIPSetName]) > 0)
}

// Tests default-allow ipset behavior
func TestDefaultAllow(t *testing.T) {
	const (
		ingressDefaultAllowIPSetName = "weave-;rGqyMIl1HN^cfDki~Z$3]6!N"
		egressDefaultAllowIPSetName  = "weave-s_+ChJId4Uy_$}G;WdH|~TK)I"
		fooPodIP                     = "10.32.0.10"
		barPodIP                     = "10.32.0.11"
		barPodNewIP                  = "10.32.0.12"
	)

	m := newMockIPSet()
	controller := New("bar", newMockIPTables(), &m, &fake.Clientset{})

	ctx := context.Background()
	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			UID:  "default",
		},
	}
	controller.AddNamespace(ctx, defaultNamespace)

	// Should create an ipset for default-allow
	require.Contains(t, m.sets, ingressDefaultAllowIPSetName)
	require.Contains(t, m.sets, egressDefaultAllowIPSetName)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: fooPodIP}}
	controller.AddPod(ctx, podFoo)

	// Should add the foo pod to default-allow
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))

	podBar := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "bar",
			Namespace: "default",
			Name:      "bar",
			Labels:    map[string]string{"run": "bar"}},
		Status: coreapi.PodStatus{PodIP: barPodIP}}
	podBarNoIP := &coreapi.Pod{ObjectMeta: podBar.ObjectMeta}
	controller.AddPod(ctx, podBarNoIP)

	controller.UpdatePod(ctx, podBarNoIP, podBar)

	// Should add the bar pod to default-allow
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, barPodIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, barPodIP))

	// Allow access from the bar pod to the foo pod
	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-from-bar-to-foo",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"run": "foo"}},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"run": "bar"},
					},
				}},
			}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{CIDR: "192.168.48.0/24"},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(ctx, netpol)

	// Should remove the foo pod from default-allow as the netpol selects it
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.False(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, barPodIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, barPodIP))

	podBarWithNewIP := *podBar
	podBarWithNewIP.Status.PodIP = barPodNewIP
	controller.UpdatePod(ctx, podBar, &podBarWithNewIP)

	// Should update IP addr of the bar pod in default-allow
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, barPodIP))
	require.False(t, m.entriesExist(egressDefaultAllowIPSetName, barPodIP))
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, barPodNewIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, barPodNewIP))

	controller.UpdatePod(ctx, &podBarWithNewIP, podBarNoIP)
	// Should remove the bar pod from default-allow as it does not have any IP addr
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, barPodNewIP))
	require.False(t, m.entriesExist(egressDefaultAllowIPSetName, barPodNewIP))

	podFooWithNewLabel := *podFoo
	podFooWithNewLabel.ObjectMeta.Labels = map[string]string{"run": "new-foo"}
	controller.UpdatePod(ctx, podFoo, &podFooWithNewLabel)

	// Should bring back the foo pod to default-allow as it does not match dst of any netpol
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))

	controller.UpdatePod(ctx, &podFooWithNewLabel, podFoo)
	// Should remove from default-allow as it matches the netpol after the update
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.False(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))

	controller.DeleteNetworkPolicy(ctx, netpol)
	// Should bring back the foo pod to default-allow as no netpol selects it
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.True(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))

	controller.DeletePod(ctx, podFoo)
	// Should remove foo pod from default-allow
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, fooPodIP))
	require.False(t, m.entriesExist(egressDefaultAllowIPSetName, fooPodIP))

	controller.DeleteNamespace(ctx, defaultNamespace)
	// Should remove default ipset
	require.NotContains(t, m.sets, ingressDefaultAllowIPSetName)
	require.NotContains(t, m.sets, egressDefaultAllowIPSetName)
}

func TestOutOfOrderPodEvents(t *testing.T) {
	const (
		ingressDefaultAllowIPSetName = "weave-;rGqyMIl1HN^cfDki~Z$3]6!N"
		runBarIPSetName              = "weave-bZ~x=yBgzH)Ht()K*Uv3z{M]Y"
		podIP                        = "10.32.0.10"
	)

	m := newMockIPSet()
	client := fake.NewSimpleClientset()
	controller := New("qux", newMockIPTables(), &m, client)
	ctx := context.Background()

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			UID:  "default",
		},
	}
	client.CoreV1().Namespaces().Create(ctx, defaultNamespace, metav1.CreateOptions{})
	controller.AddNamespace(ctx, defaultNamespace)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(ctx, podFoo)

	// Should be in default-allow as no netpol selects podFoo
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))

	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "allow-from-bar-to-foo",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
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
	controller.AddNetworkPolicy(ctx, netpol)

	// Shouldn't be in default-allow as netpol above selects podFoo
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))

	podBar := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "bar",
			Namespace: "default",
			Name:      "bar",
			Labels:    map[string]string{"run": "bar"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(ctx, podBar)

	// Should be in default-allow as no netpol selects podBar
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))
	require.True(t, m.EntryExists(uid(podBar), ingressDefaultAllowIPSetName, podIP))
	// Should be in run=bar ipset
	require.True(t, m.entriesExist(runBarIPSetName, podIP))

	controller.DeletePod(ctx, podFoo)
	// Multiple duplicate events should not affect npc state
	controller.DeletePod(ctx, podFoo)
	controller.DeletePod(ctx, podFoo)

	// Should be in default-allow as no netpol selects podBar and podFoo removal
	// should not affect podBar in default-allow
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))

	controller.DeletePod(ctx, podBar)

	// Should remove from default-allow and run=bar ipsets
	require.Equal(t, 0, len(m.sets[ingressDefaultAllowIPSetName].subSets))
	require.False(t, m.entriesExist(runBarIPSetName, podIP))
}

// Test case for https://github.com/rajch/weave/issues/3222
func TestNewTargetSelector(t *testing.T) {
	const (
		ingressDefaultAllowIPSetName = "weave-;rGqyMIl1HN^cfDki~Z$3]6!N"
		podIP                        = "10.32.0.10"
	)

	m := newMockIPSet()
	client := fake.NewSimpleClientset()
	controller := New("baz", newMockIPTables(), &m, client)
	ctx := context.Background()

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			UID:  "default",
		},
	}
	client.CoreV1().Namespaces().Create(ctx, defaultNamespace, metav1.CreateOptions{})
	controller.AddNamespace(ctx, defaultNamespace)

	podFoo := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "foo",
			Namespace: "default",
			Name:      "foo",
			Labels:    map[string]string{"run": "foo"}},
		Status: coreapi.PodStatus{PodIP: podIP}}
	controller.AddPod(ctx, podFoo)

	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))

	netpolBar := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-bar",
			Name:      "allow-from-default-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(ctx, netpolBar)

	// netpolBar target selector selects podFoo
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))

	netpolFoo := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-foo",
			Name:      "allow-from-default-2",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			PodSelector: metav1.LabelSelector{},
			Ingress: []networkingv1.NetworkPolicyIngressRule{{
				From: []networkingv1.NetworkPolicyPeer{{
					PodSelector: &metav1.LabelSelector{},
				}},
			}},
		},
	}
	controller.AddNetworkPolicy(ctx, netpolFoo)

	controller.DeleteNetworkPolicy(ctx, netpolBar)
	// netpolFoo target-selects podFoo
	require.False(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))
	controller.DeleteNetworkPolicy(ctx, netpolFoo)
	// No netpol target-selects podFoo
	require.True(t, m.entriesExist(ingressDefaultAllowIPSetName, podIP))
}

func TestEgressPolicyWithIPBlock(t *testing.T) {
	const (
		fooPodIP                    = "10.32.0.10"
		exceptIPSetName             = "weave-j:W$5om!$8JS})buYAD7q^#sX"
		exceptIPSetNameInNonDefault = "weave-|*P{aL2C@#N/1IZ!IvQItv_pt"
	)

	m := newMockIPSet()
	ipt := newMockIPTables()
	client := fake.NewSimpleClientset()
	controller := New("foo", ipt, &m, client)
	ctx := context.Background()

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			UID:  "default",
		},
	}
	nonDefaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-default",
			UID:  "non-default",
		},
	}
	client.CoreV1().Namespaces().Create(ctx, defaultNamespace, metav1.CreateOptions{})
	client.CoreV1().Namespaces().Create(ctx, nonDefaultNamespace, metav1.CreateOptions{})

	netpolFoo := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-foo",
			Name:      "allow-from-bar-to-foo",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"run": "bar"}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "192.168.48.0/24",
						Except: []string{
							"192.168.48.1/32",
							"192.168.48.2/32",
						},
					},
				}},
			}},
		},
	}
	err := controller.AddNetworkPolicy(ctx, netpolFoo)
	require.NoError(t, err)

	require.Equal(t, 2, len(m.sets[exceptIPSetName].subSets))
	require.True(t, m.entriesExist(exceptIPSetName, "192.168.48.1/32"))
	require.True(t, m.entriesExist(exceptIPSetName, "192.168.48.2/32"))

	// Each egress rule is represented as two iptables rules (-J MARK and -J RETURN).
	require.Equal(t, 2, len(ipt.rules[chains.EgressCustomChain]))
	for rule := range ipt.rules[chains.EgressCustomChain] {
		require.Contains(t, rule, "-d 192.168.48.0/24 -m set ! --match-set "+exceptIPSetName+" dst")
	}

	// Check that we create a new ipset for the ipBlock bellow. An ipset with the
	// same content already exists (created by netpolFoo), but we need to create
	// a new one, as netpolBar is in a different namespace and weave-npc does
	// object refcounting only within namespace boundaries.
	netpolBar := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "netpol-foo-bar",
			Name:      "allow-from-bar-to-foo-in-non-default",
			Namespace: "non-default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"run": "bar"}},
			Egress: []networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "192.168.48.0/24",
						Except: []string{
							"192.168.48.1/32",
							"192.168.48.2/32",
						},
					},
				}},
			}},
		},
	}
	err = controller.AddNetworkPolicy(ctx, netpolBar)
	require.NoError(t, err)
	require.Equal(t, 2, len(m.sets[exceptIPSetNameInNonDefault].subSets))
}

// Test case for https://github.com/rajch/weave/issues/3653
func TestIngressPolicyWithIPBlockAndPortSpecified(t *testing.T) {
	const (
		barPodIP        = "10.32.0.11"
		runBarIPSetName = "weave-bZ~x=yBgzH)Ht()K*Uv3z{M]Y"
	)

	m := newMockIPSet()
	ipt := newMockIPTables()
	client := fake.NewSimpleClientset()
	controller := New("any", ipt, &m, client)
	ctx := context.Background()

	defaultNamespace := &coreapi.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
			UID:  "default",
		},
	}

	client.CoreV1().Namespaces().Create(ctx, defaultNamespace, metav1.CreateOptions{})

	podBar := &coreapi.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "bar",
			Namespace: "default",
			Name:      "bar",
			Labels:    map[string]string{"run": "bar"}},
		Status: coreapi.PodStatus{PodIP: barPodIP}}
	controller.AddPod(ctx, podBar)
	defer controller.DeletePod(ctx, podBar)

	portProtocol := coreapi.ProtocolTCP
	port := intstr.FromInt(80)
	netpolicty := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			UID:       "ipblock-bar",
			Name:      "allow-ipblock-to-bar",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			PodSelector: metav1.LabelSelector{MatchLabels: podBar.Labels},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &portProtocol,
							Port:     &port,
						},
					},
					From: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "192.168.48.4/32",
							},
						},
					},
				},
			},
		},
	}

	err := controller.AddNetworkPolicy(ctx, netpolicty)
	require.NoError(t, err)
	defer controller.DeleteNetworkPolicy(ctx, netpolicty)

	require.Equal(t, 1, len(m.sets[runBarIPSetName].subSets))
	require.True(t, m.entriesExist(runBarIPSetName, barPodIP))

	require.Equal(t, 1, len(ipt.rules[chains.IngressChain]))
	for rule := range ipt.rules[chains.IngressChain] {
		require.Contains(t, rule, "-s 192.168.48.4/32 -m set --match-set "+runBarIPSetName+" dst --dport 80")
	}
}
