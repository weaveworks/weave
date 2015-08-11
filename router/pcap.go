package router

import (
	"fmt"

	"github.com/google/gopacket/pcap"
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
		// If gopacket is compiled against an older pcap.h that
		// doesn't have pcap_set_immediate_mode, it supplies a dummy
		// definition that always returns PCAP_ERROR.  That becomes
		// "Generic error", which is not very helpful.  The real
		// pcap_set_immediate_mode never returns PCAP_ERROR, so this
		// turns it into a more informative message.
		if fmt.Sprint(err) == "Generic error" {
			err = fmt.Errorf("compiled against an old version of libpcap; please compile against libpcap-1.5.0 or later")
		}

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

func (pio *PcapIO) ReadPacket() (data []byte, err error) {
	for {
		data, _, err = pio.handle.ZeroCopyReadPacketData()
		if err == nil || err != pcap.NextErrorTimeoutExpired {
			break
		}
	}
	return
}

func (pio *PcapIO) WritePacket(data []byte) error {
	return pio.handle.WritePacketData(data)
}

func (pio *PcapIO) Stats() map[string]int {
	stats, err := pio.handle.Stats()
	if err != nil {
		return nil
	}
	res := make(map[string]int)
	res["PacketsReceived"] = stats.PacketsReceived
	res["PacketsDropped"] = stats.PacketsDropped
	res["PacketsIfDropped"] = stats.PacketsIfDropped
	return res
}
