package router

const (
	Protocol        = "weave"
	ProtocolVersion = 11
)

type ProtocolTag byte

const (
	ProtocolConnectionEstablished ProtocolTag = iota
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
	ProtocolNonce
	ProtocolFetchAll
	ProtocolUpdate
	ProtocolPMTUVerified
	ProtocolGossip
	ProtocolGossipUnicast
	ProtocolGossipBroadcast
)

type ProtocolMsg struct {
	tag ProtocolTag
	msg []byte
}

type ProtocolSender interface {
	SendProtocolMsg(m ProtocolMsg)
}
