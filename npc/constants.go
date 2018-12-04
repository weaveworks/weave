package npc

const (
	TableFilter = "filter"

	MainChain    = "WEAVE-NPC"
	DefaultChain = "WEAVE-NPC-DEFAULT"
	IngressChain = "WEAVE-NPC-INGRESS"

	EgressChain        = "WEAVE-NPC-EGRESS"
	EgressDefaultChain = "WEAVE-NPC-EGRESS-DEFAULT"
	EgressCustomChain  = "WEAVE-NPC-EGRESS-CUSTOM"
	EgressMarkChain    = "WEAVE-NPC-EGRESS-ACCEPT"
	EgressMark         = "0x40000/0x40000"

	IpsetNamePrefix = "weave-"

	LocalIpset = IpsetNamePrefix + "local-pods"
)
