package router

import (
	"bytes"
	"code.google.com/p/go-bit/bit"
	"code.google.com/p/gopacket"
	"code.google.com/p/gopacket/layers"
	"encoding/gob"
	"net"
	"sync"
	"time"
)

type Router struct {
	Ourself            *Peer
	Iface              *net.Interface
	PeersSubscribeChan chan<- chan<- map[string]Peer
	Macs               *MacCache
	Peers              *PeerCache
	UDPListener        *net.UDPConn
	Topology           *Topology
	ConnectionMaker    *ConnectionMaker
	Password           *[]byte
	ConnLimit          int
	BufSz              int
	LogFrame           func(string, []byte, *layers.Ethernet)
}

type Peer struct {
	sync.RWMutex
	Name          PeerName
	NameByte      []byte
	connections   map[PeerName]Connection
	version       uint64
	UID           uint64
	Router        *Router
	localRefCount uint64
	queryChan     chan<- *PeerInteraction
}

type PeerInteraction struct {
	Interaction
	payload interface{}
}

type Connection interface {
	Local() *Peer
	Remote() *Peer
	RemoteTCPAddr() string
	Established() bool
	Shutdown()
}

type RemoteConnection struct {
	local         *Peer
	remote        *Peer
	remoteTCPAddr string
}

type LocalConnection struct {
	sync.RWMutex
	RemoteConnection
	TCPConn       *net.TCPConn
	tcpSender     TCPSender
	remoteUDPAddr *net.UDPAddr
	established   bool
	stackFrag     bool
	effectivePMTU int
	SessionKey    *[32]byte
	heartbeatStop chan<- interface{}
	forwardChan   chan<- *ForwardedFrame
	forwardChanDF chan<- *ForwardedFrame
	stopForward   chan<- interface{}
	stopForwardDF chan<- interface{}
	verifyPMTU    chan<- int
	Decryptor     Decryptor
	Router        *Router
	UID           uint64
	shutdown      bool
	queryChan     chan<- *ConnectionInteraction
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
	Desc string
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

type Topology struct {
	sync.RWMutex
	queryChan chan<- *Interaction
	unicast   map[PeerName]PeerName
	broadcast map[PeerName][]PeerName
	router    *Router
}

type ConnectionMakerInteraction struct {
	Interaction
	address string
}

type ConnectionMaker struct {
	router         *Router
	queryChan      chan<- *ConnectionMakerInteraction
	targets        map[string]*Target
	cmdLineAddress map[string]bool
}

type ConnectionState int

// Information about an address where we may find a peer
type Target struct {
	attempting   bool          // are we currently attempting to connect there?
	tryAfter     time.Time     // next time to try this address
	tryInterval  time.Duration // backoff time on next failure
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

// TCPSender interface and implementations

type TCPSender interface {
	Send([]byte) error
}

type SimpleTCPSender struct {
	encoder *gob.Encoder
}

type EncryptedTCPSender struct {
	sync.RWMutex
	outerEncoder *gob.Encoder
	innerEncoder *gob.Encoder
	buffer       *bytes.Buffer
	conn         *LocalConnection
	msgCount     int
}

type EncryptedTCPMessage struct {
	Number int
	Body   []byte
}

// TCPReceiver interface and implementations

type TCPReceiver interface {
	Decode([]byte) ([]byte, error)
}

type SimpleTCPReceiver struct {
}

type EncryptedTCPReceiver struct {
	conn     *LocalConnection
	decoder  *gob.Decoder
	buffer   *bytes.Buffer
	msgCount int
}

// Encryptor interface and implementations

type Encryptor interface {
	FrameOverhead() int
	PacketOverhead() int
	IsEmpty() bool
	Bytes() []byte
	AppendFrame(*ForwardedFrame)
	TotalLen() int
}

type NonEncryptor struct {
	buf       []byte
	bufTail   []byte
	buffered  int
	prefixLen int
}

type NaClEncryptor struct {
	NonEncryptor
	buf       []byte
	offset    uint16
	nonce     *[24]byte
	nonceChan chan *[24]byte
	flags     uint16
	prefixLen int
	conn      *LocalConnection
	df        bool
}

// Decryptor interface and implementations

type FrameConsumer func(*LocalConnection, *net.UDPAddr, []byte, []byte, uint16, []byte) error

type Decryptor interface {
	IterateFrames(FrameConsumer, *UDPPacket) error
	ReceiveNonce([]byte)
	Shutdown()
}

type NonDecryptor struct {
	conn *LocalConnection
}

type NaClDecryptor struct {
	NonDecryptor
	instance   *NaClDecryptorInstance
	instanceDF *NaClDecryptorInstance
}

type NaClDecryptorInstance struct {
	nonce               *[24]byte
	previousNonce       *[24]byte
	usedOffsets         *bit.Set
	previousUsedOffsets *bit.Set
	highestOffsetSeen   uint16
	nonceChan           chan *[24]byte
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
