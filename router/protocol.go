package router

const (
	Protocol        = "weave"
	ProtocolVersion = 10
)

type ProtocolMsg byte

type TaggedProtocolMsg struct {
	tag ProtocolMsg
	msg []byte
}

const (
	ProtocolConnectionEstablished ProtocolMsg = iota
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
	ProtocolNonce
	ProtocolFetchAll
	ProtocolUpdate
	ProtocolPMTUVerified
)
