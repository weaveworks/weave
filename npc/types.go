package npc

type policyType byte

const (
	ingressPolicy policyType = iota
	egressPolicy
)

func policyTypeStr(policyType policyType) string {
	if policyType == egressPolicy {
		return "egress"
	}
	return "ingress"
}
