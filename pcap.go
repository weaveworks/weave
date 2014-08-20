package weave

import (
	"code.google.com/p/gopacket/pcap"
)

type PcapIO struct {
	handle *pcap.Handle
}

func NewPcapIO(ifName string, bufSz int) (pio PacketSourceSink, err error) {
	pio, err = newPcapIO(ifName, true, 65535, bufSz)
	return
}

func NewPcapO(ifName string) (po PacketSink, err error) {
	po, err = newPcapIO(ifName, false, 0, 0)
	return
}

func newPcapIO(ifName string, promisc bool, snaplen int, bufSz int) (handle *PcapIO, err error) {
	inactive, err := pcap.NewInactiveHandle(ifName)
	if err != nil {
		return
	}
	defer inactive.CleanUp()
	if inactive.SetPromisc(promisc) != nil {
		return
	}
	if inactive.SetSnapLen(snaplen) != nil {
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
	data, _, err = pi.handle.ZeroCopyReadPacketData()
	return
}

func (po *PcapIO) WritePacket(data []byte) error {
	return po.handle.WritePacketData(data)
}
