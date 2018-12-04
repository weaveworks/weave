package npc

type policyType byte

const (
	policyTypeIngress policyType = iota
	policyTypeEgress
)

func policyTypeStr(policyType policyType) string {
	if policyType == policyTypeEgress {
		return "egress"
	}
	return "ingress"
}
