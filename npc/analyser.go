package npc

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	extnapi "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	networkingv1 "k8s.io/client-go/pkg/apis/networking/v1"

	"github.com/weaveworks/weave/npc/ipset"
)

func (ns *ns) analysePolicyLegacy(policy *extnapi.NetworkPolicy) (
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors map[string]*selectorSpec,
	err error) {

	nsSelectors = make(map[string]*selectorSpec)
	podSelectors = make(map[string]*selectorSpec)
	rules = make(map[string]*ruleSpec)

	dstSelector, err := newSelectorSpec(&policy.Spec.PodSelector, ns.name, ipset.HashIP)
	if err != nil {
		return nil, nil, nil, err
	}
	podSelectors[dstSelector.key] = dstSelector

	for _, ingressRule := range policy.Spec.Ingress {
		if ingressRule.Ports != nil && len(ingressRule.Ports) == 0 {
			// Ports is empty, this rule matches no ports (no traffic matches).
			continue
		}

		if ingressRule.From != nil && len(ingressRule.From) == 0 {
			// From is empty, this rule matches no sources (no traffic matches).
			continue
		}

		if ingressRule.From == nil {
			// From is not provided, this rule matches all sources (traffic not restricted by source).
			if ingressRule.Ports == nil {
				// Ports is not provided, this rule matches all ports (traffic not restricted by port).
				rule := newRuleSpecAllow(nil, nil, dstSelector, nil)
				rules[rule.key] = rule
			} else {
				// Ports is present and contains at least one item, then this rule allows traffic
				// only if the traffic matches at least one port in the ports list.
				withNormalisedProtoAndPortLegacy(ingressRule.Ports, func(proto, port string) {
					rule := newRuleSpecAllow(&proto, nil, dstSelector, &port)
					rules[rule.key] = rule
				})
			}
		} else {
			// From is present and contains at least on item, this rule allows traffic only if the
			// traffic matches at least one item in the from list.
			for _, peer := range ingressRule.From {
				var srcSelector *selectorSpec
				if peer.PodSelector != nil {
					srcSelector, err = newSelectorSpec(peer.PodSelector, ns.name, ipset.HashIP)
					if err != nil {
						return nil, nil, nil, err
					}
					podSelectors[srcSelector.key] = srcSelector
				}
				if peer.NamespaceSelector != nil {
					srcSelector, err = newSelectorSpec(peer.NamespaceSelector, "", ipset.ListSet)
					if err != nil {
						return nil, nil, nil, err
					}
					nsSelectors[srcSelector.key] = srcSelector
				}

				if ingressRule.Ports == nil {
					// Ports is not provided, this rule matches all ports (traffic not restricted by port).
					rule := newRuleSpecAllow(nil, srcSelector, dstSelector, nil)
					rules[rule.key] = rule
				} else {
					// Ports is present and contains at least one item, then this rule allows traffic
					// only if the traffic matches at least one port in the ports list.
					withNormalisedProtoAndPortLegacy(ingressRule.Ports, func(proto, port string) {
						rule := newRuleSpecAllow(&proto, srcSelector, dstSelector, &port)
						rules[rule.key] = rule
					})
				}
			}
		}
	}

	return rules, nsSelectors, podSelectors, nil
}

func (ns *ns) analysePolicy(policy *networkingv1.NetworkPolicy) (
	rules map[string]*ruleSpec,
	nsSelectors, podSelectors map[string]*selectorSpec,
	err error) {

	nsSelectors = make(map[string]*selectorSpec)
	podSelectors = make(map[string]*selectorSpec)
	rules = make(map[string]*ruleSpec)

	// If empty, matches all pods in a namespace
	dstSelector, err := newSelectorSpec(&policy.Spec.PodSelector, ns.name, ipset.HashIP)
	if err != nil {
		return nil, nil, nil, err
	}
	podSelectors[dstSelector.key] = dstSelector

	// If ingress is empty then this NetworkPolicy does not allow any traffic
	if policy.Spec.Ingress == nil || len(policy.Spec.Ingress) == 0 {
		rule := newRuleSpecDeny(dstSelector)
		rules[rule.key] = rule
		return
	}

	for _, ingressRule := range policy.Spec.Ingress {
		// If Ports is empty or missing, this rule matches all ports
		allPorts := ingressRule.Ports == nil || len(ingressRule.Ports) == 0
		// If From is empty or missing, this rule matches all sources
		allSources := ingressRule.From == nil || len(ingressRule.From) == 0

		if allSources {
			if allPorts {
				rule := newRuleSpecAllow(nil, nil, dstSelector, nil)
				rules[rule.key] = rule
				rule = newRuleSpecDeny(dstSelector)
				rules[rule.key] = rule
			} else {
				withNormalisedProtoAndPort(ingressRule.Ports, func(proto, port string) {
					rule := newRuleSpecAllow(&proto, nil, dstSelector, &port)
					rules[rule.key] = rule
					rule = newRuleSpecDeny(dstSelector)
					rules[rule.key] = rule
				})
			}
		} else {
			for _, peer := range ingressRule.From {
				var srcSelector *selectorSpec

				// NetworkPolicyPeer describes a peer to allow traffic from.
				// Exactly one of its fields must be specified.
				if peer.PodSelector != nil {
					srcSelector, err = newSelectorSpec(peer.PodSelector, ns.name, ipset.HashIP)
					if err != nil {
						return nil, nil, nil, err
					}
					podSelectors[srcSelector.key] = srcSelector

				} else if peer.NamespaceSelector != nil {
					srcSelector, err = newSelectorSpec(peer.NamespaceSelector, "", ipset.ListSet)
					if err != nil {
						return nil, nil, nil, err
					}
					nsSelectors[srcSelector.key] = srcSelector
				}

				if allPorts {
					rule := newRuleSpecAllow(nil, srcSelector, dstSelector, nil)
					rules[rule.key] = rule
					rule = newRuleSpecDeny(dstSelector)
					rules[rule.key] = rule
				} else {
					withNormalisedProtoAndPort(ingressRule.Ports, func(proto, port string) {
						rule := newRuleSpecAllow(&proto, srcSelector, dstSelector, &port)
						rules[rule.key] = rule
						rule = newRuleSpecDeny(dstSelector)
						rules[rule.key] = rule
					})
				}
			}
		}
	}

	return rules, nsSelectors, podSelectors, nil
}

func withNormalisedProtoAndPortLegacy(npps []extnapi.NetworkPolicyPort, f func(proto, port string)) {
	for _, npp := range npps {
		f(proto(npp.Protocol), port(npp.Port))
	}
}

func withNormalisedProtoAndPort(npps []networkingv1.NetworkPolicyPort, f func(proto, port string)) {
	for _, npp := range npps {
		f(proto(npp.Protocol), port(npp.Port))
	}
}

func proto(p *v1.Protocol) string {
	// If no proto is specified, default to TCP
	proto := string(api.ProtocolTCP)
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
