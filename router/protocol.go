package router

const (
	Protocol        = "weave"
	ProtocolVersion = 10
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
)

type ProtocolMsg struct {
	tag ProtocolTag
	msg []byte
}
