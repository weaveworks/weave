package router

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"log"
	"net"
	"syscall"
)

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

type MsgTooBigError struct {
	PMTU int // actual pmtu, i.e. what the kernel told us
}

func NewSimpleUDPSender(conn *LocalConnection) *SimpleUDPSender {
	return &SimpleUDPSender{udpConn: conn.Router.UDPListener, conn: conn}
}

func (sender *SimpleUDPSender) Send(msg []byte) error {
	_, err := sender.udpConn.WriteToUDP(msg, sender.conn.RemoteUDPAddr())
	return err
}

func (sender *SimpleUDPSender) Shutdown() error {
	return nil
}

func NewRawUDPSender(conn *LocalConnection) (*RawUDPSender, error) {
	ipSocket, err := dialIP(conn)
	if err != nil {
		return nil, err
	}
	udpHeader := &layers.UDP{SrcPort: layers.UDPPort(conn.Router.Port)}
	ipBuf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths: true,
		// UDP header is calculated with a phantom IP
		// header. Yes, it's totally nuts. Thankfully, for UDP
		// over IPv4, the checksum is optional. It's not
		// optional for IPv6, but we'll ignore that for
		// now. TODO
		ComputeChecksums: false}

	return &RawUDPSender{
		ipBuf:     ipBuf,
		opts:      opts,
		udpHeader: udpHeader,
		socket:    ipSocket,
		conn:      conn}, nil
}

func (sender *RawUDPSender) Send(msg []byte) error {
	payload := gopacket.Payload(msg)
	sender.udpHeader.DstPort = layers.UDPPort(sender.conn.RemoteUDPAddr().Port)

	err := gopacket.SerializeLayers(sender.ipBuf, sender.opts, sender.udpHeader, &payload)
	if err != nil {
		return err
	}
	packet := sender.ipBuf.Bytes()
	_, err = sender.socket.Write(packet)
	if err == nil || PosixError(err) != syscall.EMSGSIZE {
		return err
	}
	f, err := sender.socket.File()
	if err != nil {
		return err
	}
	defer f.Close()
	fd := int(f.Fd())
	log.Println("EMSGSIZE on send, expecting PMTU update (IP packet was",
		len(packet), "bytes, payload was", len(msg), "bytes)")
	pmtu, err := syscall.GetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MTU)
	if err != nil {
		return err
	}
	return MsgTooBigError{PMTU: pmtu}
}

func (sender *RawUDPSender) Shutdown() error {
	defer func() { sender.socket = nil }()
	return sender.socket.Close()
}

func dialIP(conn *LocalConnection) (*net.IPConn, error) {
	ipLocalAddr, err := ipAddr(conn.TCPConn.LocalAddr())
	if err != nil {
		return nil, err
	}
	ipRemoteAddr, err := ipAddr(conn.TCPConn.RemoteAddr())
	if err != nil {
		return nil, err
	}
	ipSocket, err := net.DialIP("ip4:UDP", ipLocalAddr, ipRemoteAddr)
	if err != nil {
		return nil, err
	}
	f, err := ipSocket.File()
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fd := int(f.Fd())
	// This Makes sure all packets we send out have DF set on them.
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_MTU_DISCOVER, syscall.IP_PMTUDISC_DO)
	if err != nil {
		return nil, err
	}
	return ipSocket, nil
}

func ipAddr(addr net.Addr) (*net.IPAddr, error) {
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, err
	}
	return &net.IPAddr{
		IP:   net.ParseIP(host),
		Zone: ""}, nil
}
