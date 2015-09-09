package router

import (
	"math"
	"time"
)

const (
	Port                = 6783
	HTTPPort            = Port + 1
	MaxUDPPacketSize    = 65535
	ChannelSize         = 16
	TCPHeartbeat        = 30 * time.Second
	GossipInterval      = 30 * time.Second
	MaxDuration         = time.Duration(math.MaxInt64)
	MaxTCPMsgSize       = 10 * 1024 * 1024
	FastHeartbeat       = 500 * time.Millisecond
	SlowHeartbeat       = 10 * time.Second
	MaxMissedHeartbeats = 6
	HeartbeatTimeout    = MaxMissedHeartbeats * SlowHeartbeat
)
