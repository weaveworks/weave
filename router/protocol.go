package router

const (
	Protocol        = "weave"
	ProtocolVersion = 10
)

type ProtocolMsg byte

const (
	ProtocolConnectionEstablished ProtocolMsg = iota
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
	ProtocolNonce
	ProtocolFetchAll
	ProtocolUpdate
	ProtocolPMTUVerified
)

var (
	ProtocolConnectionEstablishedByte  = []byte{byte(ProtocolConnectionEstablished)}
	ProtocolFragmentationReceivedByte  = []byte{byte(ProtocolFragmentationReceived)}
	ProtocolStartFragmentationTestByte = []byte{byte(ProtocolStartFragmentationTest)}
	ProtocolNonceByte                  = []byte{byte(ProtocolNonce)}
	ProtocolFetchAllByte               = []byte{byte(ProtocolFetchAll)}
	ProtocolUpdateByte                 = []byte{byte(ProtocolUpdate)}
	ProtocolPMTUVerifiedByte           = []byte{byte(ProtocolPMTUVerified)}
)
