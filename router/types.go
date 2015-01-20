package router

import (
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"net"
	"sync"
	"time"
)

type Router struct {
	Iface           *net.Interface
	Ourself         *LocalPeer
	Macs            *MacCache
	Peers           *Peers
	Routes          *Routes
	ConnectionMaker *ConnectionMaker
	UDPListener     *net.UDPConn
	Password        *[]byte
	ConnLimit       int
	BufSz           int
	LogFrame        func(string, []byte, *layers.Ethernet)
}

type Connection interface {
	Local() *Peer
	Remote() *Peer
	RemoteTCPAddr() string
	Established() bool
	Shutdown(error)
}

type RemoteConnection struct {
	local         *Peer
	remote        *Peer
	remoteTCPAddr string
}

type LocalConnection struct {
	sync.RWMutex
	RemoteConnection
	TCPConn           *net.TCPConn
	tcpSender         TCPSender
	remoteUDPAddr     *net.UDPAddr
	established       bool
	receivedHeartbeat bool
	stackFrag         bool
	effectivePMTU     int
	SessionKey        *[32]byte
	heartbeatFrame    *ForwardedFrame
	heartbeat         *time.Ticker
	fetchAll          *time.Ticker
	fragTest          *time.Ticker
	forwardChan       chan<- *ForwardedFrame
	forwardChanDF     chan<- *ForwardedFrame
	stopForward       chan<- interface{}
	stopForwardDF     chan<- interface{}
	verifyPMTU        chan<- int
	Decryptor         Decryptor
	Router            *Router
	UID               uint64
	queryChan         chan<- *ConnectionInteraction
}

type ConnectionInteraction struct {
	Interaction
	payload interface{}
}

type Forwarder struct {
	conn            *LocalConnection
	ch              <-chan *ForwardedFrame
	stop            <-chan interface{}
	verifyPMTUTick  <-chan time.Time
	verifyPMTU      <-chan int
	pmtuVerifyCount uint
	enc             Encryptor
	udpSender       UDPSender
	maxPayload      int
	pmtuVerified    bool
	highestGoodPMTU int
	unverifiedPMTU  int
	lowestBadPMTU   int
}

type UDPPacket struct {
	Name   PeerName
	Packet []byte
	Sender *net.UDPAddr
}

type EthernetDecoder struct {
	eth     layers.Ethernet
	ip      layers.IPv4
	decoded []gopacket.LayerType
	parser  *gopacket.DecodingLayerParser
}

type MsgTooBigError struct {
	PMTU int // actual pmtu, i.e. what the kernel told us
}

type FrameTooBigError struct {
	EPMTU int // effective pmtu, i.e. what we tell packet senders
}

type UnknownPeersError struct {
}

type NameCollisionError struct {
	Name PeerName
}

type PacketDecodingError struct {
	Fatal bool
	Desc  string
}

type LocalAddress struct {
	ip      net.IP
	network *net.IPNet
}

type ForwardedFrame struct {
	srcPeer *Peer
	dstPeer *Peer
	frame   []byte
}

type Interaction struct {
	code       int
	resultChan chan<- interface{}
}

// UDPSender interface and implementations

type UDPSender interface {
	Send([]byte) error
	Shutdown() error
}

type SimpleUDPSender struct {
	conn    *LocalConnection
	udpConn *net.UDPConn
}

type RawUDPSender struct {
	ipBuf     gopacket.SerializeBuffer
	opts      gopacket.SerializeOptions
	udpHeader *layers.UDP
	socket    *net.IPConn
	conn      *LocalConnection
}

// Packet capture/inject interfaces

type PacketSource interface {
	ReadPacket() ([]byte, error)
}

type PacketSink interface {
	WritePacket([]byte) error
}

type PacketSourceSink interface {
	PacketSource
	PacketSink
}
