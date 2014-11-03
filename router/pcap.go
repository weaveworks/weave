package router

import (
	"code.google.com/p/gopacket/pcap"
)

type PcapIO struct {
	handle *pcap.Handle
}

func NewPcapIO(ifName string, bufSz int) (PacketSourceSink, error) {
	pio, err := newPcapIO(ifName, true, 65535, bufSz)
	if err != nil {
		return pio, err
	}

	// Under Linux, libpcap implements the SetDirection filtering
	// in userspace.  So set a BPF filter to discard outbound
	// packets inside the kernel.  We do this here rather than in
	// newPcapIO because libpcap doesn't like this in combination
	// with a 0 snaplen.
	err = pio.handle.SetBPFFilter("inbound")
	return pio, err
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
	if err = inactive.SetPromisc(promisc); err != nil {
		return
	}
	if err = inactive.SetSnapLen(snaplen); err != nil {
		return
	}
	if err = inactive.SetTimeout(MaxDuration); err != nil {
		return
	}
	if err = inactive.SetImmediateMode(true); err != nil {
		return
	}
	if err = inactive.SetBufferSize(bufSz); err != nil {
		return
	}
	active, err := inactive.Activate()
	if err != nil {
		return
	}
	if err = active.SetDirection(pcap.DirectionIn); err != nil {
		return
	}
	return &PcapIO{handle: active}, nil
}

func (pi *PcapIO) ReadPacket() (data []byte, err error) {
	for {
		data, _, err = pi.handle.ZeroCopyReadPacketData()
		if err == nil || err != pcap.NextErrorTimeoutExpired {
			break
		}
	}
	return
}

func (po *PcapIO) WritePacket(data []byte) error {
	return po.handle.WritePacketData(data)
}
