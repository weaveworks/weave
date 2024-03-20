package npc

import (
	"fmt"

	apiv1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/rajch/weave/net/ipset"
)

func (ns *ns) analysePolicy(policy *networkingv1.NetworkPolicy) (
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors, namespacedPodSelectors map[string]*selectorSpec,
	ipBlocks map[string]*ipBlockSpec,
	err error) {

	nsSelectors = make(map[string]*selectorSpec)
	podSelectors = make(map[string]*selectorSpec)
	namespacedPodSelectors = make(map[string]*selectorSpec)

	ipBlocks = make(map[string]*ipBlockSpec)
	rules = make(map[string]*ruleSpec)
	policyTypes := make([]policyType, 0)

	for _, pt := range policy.Spec.PolicyTypes {
		if pt == networkingv1.PolicyTypeIngress {
			policyTypes = append(policyTypes, policyTypeIngress)
		}
		if pt == networkingv1.PolicyTypeEgress {
			policyTypes = append(policyTypes, policyTypeEgress)
		}
	}
	// If empty, matches all pods in a namespace
	targetSelector, err := newSelectorSpec(&policy.Spec.PodSelector, nil, policyTypes, ns.name, ipset.HashIP)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// To prevent targetSelector being overwritten by a subsequent selector with
	// the same key, addIfNotExist MUST be used when adding to podSelectors.
	// Otherwise, policyTypes of the selector might be lost resulting in
	// an invalid content in any "default-allow" ipset.
	addIfNotExist(targetSelector, podSelectors)

	// If ingress is empty then this NetworkPolicy does not allow any ingress traffic
	if policy.Spec.Ingress != nil && len(policy.Spec.Ingress) != 0 {
		for _, ingressRule := range policy.Spec.Ingress {
			// If Ports is empty or missing, this rule matches all ports
			allPorts := ingressRule.Ports == nil || len(ingressRule.Ports) == 0
			// If From is empty or missing, this rule matches all sources
			allSources := ingressRule.From == nil || len(ingressRule.From) == 0

			if !allPorts {
				if err = checkForNamedPorts(ingressRule.Ports); err != nil {
					return nil, nil, nil, nil, nil, fmt.Errorf("named ports in network policies is not supported yet. "+
						"Rejecting network policy: %s from further processing. "+err.Error(), policy.Name)
				}
			}
			if allSources {
				if allPorts {
					rule := newRuleSpec(policyTypeIngress, nil, nil, targetSelector, nil)
					rules[rule.key] = rule
				} else {
					withNormalisedProtoAndPort(ingressRule.Ports, func(proto, port string) {
						rule := newRuleSpec(policyTypeIngress, &proto, nil, targetSelector, &port)
						rules[rule.key] = rule
					})
				}
			} else {
				for _, peer := range ingressRule.From {
					var srcSelector *selectorSpec
					var srcRuleHost ruleHost

					// NetworkPolicyPeer describes a peer to allow traffic from.
					if peer.PodSelector != nil && peer.NamespaceSelector != nil {
						srcSelector, err = newSelectorSpec(peer.PodSelector, peer.NamespaceSelector, nil, "", ipset.HashIP)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						addIfNotExist(srcSelector, namespacedPodSelectors)
						srcRuleHost = srcSelector
					} else if peer.PodSelector != nil {
						srcSelector, err = newSelectorSpec(peer.PodSelector, nil, nil, ns.name, ipset.HashIP)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						addIfNotExist(srcSelector, podSelectors)
						srcRuleHost = srcSelector
					} else if peer.NamespaceSelector != nil {
						srcSelector, err = newSelectorSpec(nil, peer.NamespaceSelector, nil, "", ipset.ListSet)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						nsSelectors[srcSelector.key] = srcSelector
						srcRuleHost = srcSelector
					} else if peer.IPBlock != nil {
						ipBlock := newIPBlockSpec(peer.IPBlock, ns.name)
						ipBlocks[ipBlock.key] = ipBlock
						srcRuleHost = ipBlock
					}

					if allPorts {
						rule := newRuleSpec(policyTypeIngress, nil, srcRuleHost, targetSelector, nil)
						rules[rule.key] = rule
					} else {
						withNormalisedProtoAndPort(ingressRule.Ports, func(proto, port string) {
							rule := newRuleSpec(policyTypeIngress, &proto, srcRuleHost, targetSelector, &port)
							rules[rule.key] = rule
						})
					}
				}
			}
		}
	}

	// If egress is empty then this NetworkPolicy does not allow any egress traffic
	if policy.Spec.Egress != nil && len(policy.Spec.Egress) != 0 {
		for _, egressRule := range policy.Spec.Egress {
			// If Ports is empty or missing, this rule matches all ports
			allPorts := egressRule.Ports == nil || len(egressRule.Ports) == 0
			// If To is empty or missing, this rule matches all destinations
			allDestinations := egressRule.To == nil || len(egressRule.To) == 0

			if !allPorts {
				if err = checkForNamedPorts(egressRule.Ports); err != nil {
					return nil, nil, nil, nil, nil, fmt.Errorf("named ports in network policies is not supported yet. "+
						"Rejecting network policy: %s from further processing. "+err.Error(), policy.Name)
				}
			}
			if allDestinations {
				if allPorts {
					rule := newRuleSpec(policyTypeEgress, nil, targetSelector, nil, nil)
					rules[rule.key] = rule
				} else {
					withNormalisedProtoAndPort(egressRule.Ports, func(proto, port string) {
						rule := newRuleSpec(policyTypeEgress, &proto, targetSelector, nil, &port)
						rules[rule.key] = rule
					})
				}
			} else {
				for _, peer := range egressRule.To {
					var dstSelector *selectorSpec
					var dstRuleHost ruleHost

					// NetworkPolicyPeer describes a peer to allow traffic to.
					if peer.PodSelector != nil && peer.NamespaceSelector != nil {
						dstSelector, err = newSelectorSpec(peer.PodSelector, peer.NamespaceSelector, nil, "", ipset.HashIP)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						addIfNotExist(dstSelector, namespacedPodSelectors)
						dstRuleHost = dstSelector
					} else if peer.PodSelector != nil {
						dstSelector, err = newSelectorSpec(peer.PodSelector, nil, nil, ns.name, ipset.HashIP)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						addIfNotExist(dstSelector, podSelectors)
						dstRuleHost = dstSelector

					} else if peer.NamespaceSelector != nil {
						dstSelector, err = newSelectorSpec(nil, peer.NamespaceSelector, nil, "", ipset.ListSet)
						if err != nil {
							return nil, nil, nil, nil, nil, err
						}
						nsSelectors[dstSelector.key] = dstSelector
						dstRuleHost = dstSelector
					} else if peer.IPBlock != nil {
						ipBlock := newIPBlockSpec(peer.IPBlock, ns.name)
						ipBlocks[ipBlock.key] = ipBlock
						dstRuleHost = ipBlock
					}

					if allPorts {
						rule := newRuleSpec(policyTypeEgress, nil, targetSelector, dstRuleHost, nil)
						rules[rule.key] = rule
					} else {
						withNormalisedProtoAndPort(egressRule.Ports, func(proto, port string) {
							rule := newRuleSpec(policyTypeEgress, &proto, targetSelector, dstRuleHost, &port)
							rules[rule.key] = rule
						})
					}
				}
			}
		}
	}

	return rules, nsSelectors, podSelectors, namespacedPodSelectors, ipBlocks, nil
}

func addIfNotExist(s *selectorSpec, ss map[string]*selectorSpec) {
	if _, ok := ss[s.key]; !ok {
		ss[s.key] = s
	}
}

func withNormalisedProtoAndPort(npps []networkingv1.NetworkPolicyPort, f func(proto, port string)) {
	for _, npp := range npps {
		f(proto(npp.Protocol), port(npp.Port))
	}
}

func proto(p *apiv1.Protocol) string {
	// If no proto is specified, default to TCP
	proto := string(apiv1.ProtocolTCP)
	if p != nil {
		proto = string(*p)
	}

	return proto
}

func port(p *intstr.IntOrString) string {
	// If no port is specified, match any port. Let iptables executable handle
	// service name resolution
	port := "0:65535"
	if p != nil {
		switch p.Type {
		case intstr.Int:
			port = fmt.Sprintf("%d", p.IntVal)
		case intstr.String:
			port = p.StrVal
		}
	}

	return port
}

func checkForNamedPorts(ports []networkingv1.NetworkPolicyPort) error {
	for _, npProtocolPort := range ports {
		if npProtocolPort.Port != nil && npProtocolPort.Port.Type == intstr.String {
			return fmt.Errorf("named port %s in network policy", port(npProtocolPort.Port))
		}
	}
	return nil
}
