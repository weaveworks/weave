package npc

const (
	TableFilter = "filter"

	MainChain           = "WEAVE-NPC"
	DefaultChain        = "WEAVE-NPC-DEFAULT"
	IngressChain        = "WEAVE-NPC-INGRESS"
	IngressIsolateChain = "WEAVE-NPC-INGRESS-ISOLATE"
	IngressDropChain    = "WEAVE-NPC-INGRESS-DROP"

	IpsetNamePrefix = "weave-"

	LocalIpset = IpsetNamePrefix + "local-pods"
)
