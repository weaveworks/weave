package router

import (
	"math"
	"time"
)

const (
	EthernetOverhead    = 14
	UDPOverhead         = 28 // 20 bytes for IPv4, 8 bytes for UDP
	Port                = 6783
	HTTPPort            = Port + 1
	DefaultPMTU         = 65535
	MaxUDPPacketSize    = 65536
	ChannelSize         = 16
	UDPNonceSendAt      = 8192
	FragTestSize        = 60001
	PMTUDiscoverySize   = 60000
	FastHeartbeat       = 500 * time.Millisecond
	SlowHeartbeat       = 10 * time.Second
	FragTestInterval    = 5 * time.Minute
	ReadTimeout         = 1 * time.Minute
	PMTUVerifyAttempts  = 8
	PMTUVerifyTimeout   = 10 * time.Millisecond // gets doubled with every attempt
	MaxDuration         = time.Duration(math.MaxInt64)
	MaxMissedHeartbeats = 3
	HeartbeatTimeout    = MaxMissedHeartbeats * SlowHeartbeat
)

var (
	FragTest      = make([]byte, FragTestSize)
	PMTUDiscovery = make([]byte, PMTUDiscoverySize)
)
