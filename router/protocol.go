package router

const (
	Protocol        = "weave"
	ProtocolVersion = 10
)

const (
	ProtocolConnectionEstablished = iota
	ProtocolFragmentationReceived
	ProtocolStartFragmentationTest
	ProtocolNonce
	ProtocolFetchAll
	ProtocolUpdate
	ProtocolPMTUVerified
)

var (
	ProtocolConnectionEstablishedByte  = []byte{ProtocolConnectionEstablished}
	ProtocolFragmentationReceivedByte  = []byte{ProtocolFragmentationReceived}
	ProtocolStartFragmentationTestByte = []byte{ProtocolStartFragmentationTest}
	ProtocolNonceByte                  = []byte{ProtocolNonce}
	ProtocolFetchAllByte               = []byte{ProtocolFetchAll}
	ProtocolUpdateByte                 = []byte{ProtocolUpdate}
	ProtocolPMTUVerifiedByte           = []byte{ProtocolPMTUVerified}
)
