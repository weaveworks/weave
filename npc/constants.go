package npc

const (
	TableFilter = "filter"

	MainChain           = "WEAVE-NPC"
	DefaultChain        = "WEAVE-NPC-DEFAULT"
	IngressChain        = "WEAVE-NPC-INGRESS"
	IngressIPBlockChain = "WEAVE-NPC-INGRESS-IPBLOCK"
	LocalIngressChain   = "WEAVE-NPC-LOCAL-INGRESS"

	IpsetNamePrefix = "weave-"

	LocalIpset = IpsetNamePrefix + "local-pods"

	BridgeIpset = IpsetNamePrefix + "bridges"
)
