package router

import (
	"time"
	"math"
)

const (
	Protocol           = "weave"
	ProtocolVersion    = 8
	EthernetOverhead   = 14
	UDPOverhead        = 28 // 20 bytes for IPv4, 8 bytes for UDP
	Port               = 6783
	StatusPort         = Port + 1
	DefaultPMTU        = 65535
	MaxUDPPacketSize   = 65536
	ChannelSize        = 16
	UDPNonceSendAt     = 8192
	FragTestSize       = 60001
	PMTUDiscoverySize  = 60000
	FastHeartbeat      = 500 * time.Millisecond
	SlowHeartbeat      = 10 * time.Second
	FetchAllInterval   = 30 * time.Second
	FragTestInterval   = 5 * time.Minute
	PMTUVerifyAttempts = 8
	PMTUVerifyTimeout  = 10 * time.Millisecond // gets doubled with every attempt
	MaxDuration        = time.Duration(math.MaxInt64)
)

const (
	ProtocolConnectionEstablished  = iota
	ProtocolFragmentationReceived  = iota
	ProtocolStartFragmentationTest = iota
	ProtocolNonce                  = iota
	ProtocolFetchAll               = iota
	ProtocolUpdate                 = iota
	ProtocolPMTUVerified           = iota
)

var (
	FragTest                           = make([]byte, FragTestSize)
	PMTUDiscovery                      = make([]byte, PMTUDiscoverySize)
	ProtocolConnectionEstablishedByte  = []byte{ProtocolConnectionEstablished}
	ProtocolFragmentationReceivedByte  = []byte{ProtocolFragmentationReceived}
	ProtocolStartFragmentationTestByte = []byte{ProtocolStartFragmentationTest}
	ProtocolNonceByte                  = []byte{ProtocolNonce}
	ProtocolFetchAllByte               = []byte{ProtocolFetchAll}
	ProtocolUpdateByte                 = []byte{ProtocolUpdate}
	ProtocolPMTUVerifiedByte           = []byte{ProtocolPMTUVerified}
)
