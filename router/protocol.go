package router

const (
	Protocol        = "weave"
	ProtocolVersion = 19
)

type ProtocolTag byte

const (
	ProtocolHeartbeat ProtocolTag = iota
	ProtocolConnectionEstablished
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
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
