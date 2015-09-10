package router

import (
	"bytes"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"net"
	"os"

	"github.com/weaveworks/weave/common"
)

var log = common.Log

var void = struct{}{}

func checkFatal(e error) {
	if e != nil {
		log.Fatal(e)
	}
}

func checkWarn(e error) {
	if e != nil {
		log.Warnln(e)
	}
}

// Look inside an error produced by the net package to get to the
// syscall.Errno at the root of the problem.
func PosixError(err error) error {
	if operr, ok := err.(*net.OpError); ok {
		err = operr.Err
	}

	// go1.5 wraps an Errno inside a SyscallError inside an OpError
	if scerr, ok := err.(*os.SyscallError); ok {
		err = scerr.Err
	}

	return err
}

func (upe UnknownPeerError) Error() string {
	return fmt.Sprint("Reference to unknown peer ", upe.Name)
}

func (nce NameCollisionError) Error() string {
	return fmt.Sprint("Multiple peers found with same name: ", nce.Name)
}

func (pde PacketDecodingError) Error() string {
	return fmt.Sprint("Failed to decode packet: ", pde.Desc)
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

func GobEncode(items ...interface{}) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, i := range items {
		checkFatal(enc.Encode(i))
	}
	return buf.Bytes()
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
	return lop[i].Name < lop[j].Name
}
