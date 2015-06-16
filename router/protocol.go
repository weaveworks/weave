package router

import (
	"bytes"
	"fmt"
	"io"
)

const (
	Protocol                = "weave"
	ProtocolVersion    byte = 1
	ProtocolMinVersion byte = ProtocolVersion
)

var (
	ProtocolBytes     = []byte(Protocol)
	ProtocolLen       = len(ProtocolBytes)
	ProtocolHeader    = append(ProtocolBytes, ProtocolMinVersion, ProtocolVersion)
	ProtocolHeaderLen = len(ProtocolHeader)
)

func exchangeProtocolHeader(w io.Writer, r io.Reader) (byte, error) {
	if n, err := w.Write(ProtocolHeader); err != nil {
		return 0, err
	} else if n != ProtocolHeaderLen {
		return 0, fmt.Errorf("failed to send complete protocol header")
	}
	header := make([]byte, ProtocolHeaderLen)
	if n, err := io.ReadFull(r, header); err != nil && n == 0 {
		return 0, fmt.Errorf("failed to receive remote protocol header: %s", err)
	} else if err != nil {
		return 0, fmt.Errorf("received incomplete remote protocol header (%d octets instead of %d): %v; error: %s",
			n, ProtocolHeaderLen, header[:n], err)
	}
	if !bytes.Equal(ProtocolBytes, header[:ProtocolLen]) {
		return 0, fmt.Errorf("remote protocol header not recognised: %v", header[:ProtocolHeaderLen])
	}
	var (
		remoteMinVersion = header[ProtocolLen]
		remoteVersion    = header[ProtocolLen+1]
	)
	minVersion := ProtocolMinVersion
	if remoteMinVersion > minVersion {
		minVersion = remoteMinVersion
	}
	maxVersion := ProtocolVersion
	if remoteVersion < maxVersion {
		maxVersion = remoteVersion
	}
	if minVersion > maxVersion {
		return 0, fmt.Errorf("remote version [%d,%d] is incompatible with our version [%d,%d]",
			remoteMinVersion, remoteVersion, ProtocolMinVersion, ProtocolVersion)
	}
	return maxVersion, nil
}

type ProtocolTag byte

const (
	ProtocolHeartbeat ProtocolTag = iota
	ProtocolConnectionEstablished
	ProtocolFragmentationReceived
	ProtocolPMTUVerified
	ProtocolGossip
	ProtocolGossipUnicast
	ProtocolGossipBroadcast
)

type ProtocolMsg struct {
	tag ProtocolTag
	msg []byte
}

type ProtocolSender interface {
	SendProtocolMsg(m ProtocolMsg)
}
