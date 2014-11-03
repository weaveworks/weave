package router

import (
	"crypto/rand"
	"fmt"
	"log"
	"net"
)

func checkFatal(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func checkWarn(e error) {
	if e != nil {
		log.Println(e)
	}
}

func PosixError(err error) error {
	if err == nil {
		return nil
	}
	operr, ok := err.(*net.OpError)
	if !ok {
		return nil
	}
	return operr.Err
}

func (mtbe MsgTooBigError) Error() string {
	return fmt.Sprint("Msg too big error. PMTU is ", mtbe.PMTU)
}

func (ftbe FrameTooBigError) Error() string {
	return fmt.Sprint("Frame too big error. Effective PMTU is ", ftbe.EPMTU)
}

func (upe UnknownPeersError) Error() string {
	return fmt.Sprint("Reference to unknown peers")
}

func (nce NameCollisionError) Error() string {
	return fmt.Sprint("Multiple peers found with same name: ", nce.Name)
}

func (pde PacketDecodingError) Error() string {
	return fmt.Sprint("Failed to decode packet: ", pde.Desc)
}

func (packet UDPPacket) String() string {
	return fmt.Sprintf("UDP Packet\n name: %s\n sender: %v\n payload: % X", packet.Name, packet.Sender, packet.Packet)
}

func Concat(elems ...[]byte) []byte {
	res := []byte{}
	for _, e := range elems {
		res = append(res, e...)
	}
	return res
}

func randUint64() (r uint64) {
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	checkFatal(err)
	for _, v := range buf {
		r <<= 8
		r |= uint64(v)
	}
	return
}

func macint(mac net.HardwareAddr) (r uint64) {
	for _, b := range mac {
		r <<= 8
		r |= uint64(b)
	}
	return
}

func intmac(key uint64) (r net.HardwareAddr) {
	r = make([]byte, 6)
	for i := 5; i >= 0; i-- {
		r[i] = byte(key)
		key >>= 8
	}
	return
}

type ListOfPeers []*Peer

func (lop ListOfPeers) Len() int {
	return len(lop)
}
func (lop ListOfPeers) Swap(i, j int) {
	lop[i], lop[j] = lop[j], lop[i]
}
func (lop ListOfPeers) Less(i, j int) bool {
	return lop[i].Name < lop[i].Name
}

// given an address like '1.2.3.4:567', return the address if it has a port,
// otherwise return the address with weave's standard port number
func NormalisePeerAddr(peerAddr string) string {
	_, _, err := net.SplitHostPort(peerAddr)
	if err == nil {
		return peerAddr
	} else {
		return fmt.Sprintf("%s:%d", peerAddr, Port)
	}
}
