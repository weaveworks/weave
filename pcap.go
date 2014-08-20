package weave

import (
	"code.google.com/p/gopacket/pcap"
)

type PcapIO struct {
	handle *pcap.Handle
}

func NewPcapIO(ifName string, bufSz int) (handle *PcapIO, err error) {
	inactive, err := pcap.NewInactiveHandle(ifName)
	if err != nil {
		return
	}
	defer inactive.CleanUp()
	if inactive.SetPromisc(true) != nil {
		return
	}
	if inactive.SetSnapLen(65535) != nil {
		return
	}
	if inactive.SetTimeout(-1) != nil {
		return
	}
	if inactive.SetImmediateMode(true) != nil {
		return
	}
	if inactive.SetBufferSize(bufSz) != nil {
		return
	}
	active, err := inactive.Activate()
	if err != nil {
		return
	}
	if active.SetDirection(pcap.DirectionIn) != nil {
		return
	}
	return &PcapIO{handle: active}, nil
}

func (pi *PcapIO) ReadPacket() (data []byte, err error) {
	data, _, err = pi.handle.ReadPacketData()
	return
}

func (po *PcapIO) WritePacket(data []byte) error {
	return po.handle.WritePacketData(data)
}
