package router

const (
	Protocol        = "weave"
	ProtocolVersion = 102
)

type ProtocolTag byte

const (
	ProtocolHeartbeat ProtocolTag = iota
	ProtocolConnectionEstablished
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
	ProtocolNonce
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
