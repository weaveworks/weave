package odp

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	DATAPATH     = iota
	VPORT        = iota
	FLOW         = iota
	PACKET       = iota
	FAMILY_COUNT = iota
)

var familyNames = [FAMILY_COUNT]string{
	"ovs_datapath",
	"ovs_vport",
	"ovs_flow",
	"ovs_packet",
}

type Dpif struct {
	sock     *NetlinkSocket
	families [FAMILY_COUNT]GenlFamily
}

type familyUnavailableError struct {
	family string
}

func (fue familyUnavailableError) Error() string {
	return fmt.Sprintf("Generic netlink family '%s' unavailable; the Open vSwitch kernel module is probably not loaded, try 'modprobe openvswitch'", fue.family)
}

func IsKernelLacksODPError(err error) bool {
	_, ok := err.(familyUnavailableError)
	return ok
}

func lookupFamily(sock *NetlinkSocket, name string) (GenlFamily, error) {
	family, err := sock.LookupGenlFamily(name)
	if err == nil {
		return family, nil
	}

	if err == NetlinkError(syscall.ENOENT) {
		loadOpenvswitchModule()

		// The module might be loaded now, so try again
		family, err = sock.LookupGenlFamily(name)
		if err == nil {
			return family, nil
		}

		if err == NetlinkError(syscall.ENOENT) {
			err = familyUnavailableError{name}
		}
	}

	return GenlFamily{}, err
}

var triedLoadOpenvswitchModule bool

// This tries to provoke the kernel into loading the openvswitch
// module.  Yes, netdev ioctls can be used to load arbitrary modules,
// if you have CAP_SYS_MODULE.
func loadOpenvswitchModule() {
	if triedLoadOpenvswitchModule {
		return
	}

	// netdev ioctls don't seem to work on netlink sockets, so we
	// need a new socket for this purpose.
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		triedLoadOpenvswitchModule = true
		return
	}

	defer syscall.Close(s)

	var req ifreqIfindex
	copy(req.name[:], []byte("openvswitch"))
	syscall.Syscall(syscall.SYS_IOCTL, uintptr(s),
		syscall.SIOCGIFINDEX, uintptr(unsafe.Pointer(&req)))
	triedLoadOpenvswitchModule = true
}

func NewDpif() (*Dpif, error) {
	sock, err := OpenNetlinkSocket(syscall.NETLINK_GENERIC)
	if err != nil {
		return nil, err
	}

	dpif := &Dpif{sock: sock}

	for i := 0; i < FAMILY_COUNT; i++ {
		dpif.families[i], err = lookupFamily(sock, familyNames[i])
		if err != nil {
			sock.Close()
			return nil, err
		}
	}

	return dpif, nil
}

// Open a dpif with a new socket, but reuing the family info
func (dpif *Dpif) Reopen() (*Dpif, error) {
	sock, err := OpenNetlinkSocket(syscall.NETLINK_GENERIC)
	if err != nil {
		return nil, err
	}

	return &Dpif{sock: sock, families: dpif.families}, nil
}

func (dpif *Dpif) getMCGroup(family int, name string) (uint32, error) {
	mcGroup, ok := dpif.families[family].mcGroups[name]
	if !ok {
		return 0, fmt.Errorf("No genl MC group %s in family %s", name, familyNames[family])
	}

	return mcGroup, nil
}

func (dpif *Dpif) Close() error {
	return dpif.sock.Close()
}

func (nlmsg *NlMsgBuilder) putOvsHeader(ifindex DatapathID) {
	pos := nlmsg.AlignGrow(syscall.NLMSG_ALIGNTO, SizeofOvsHeader)
	h := ovsHeaderAt(nlmsg.buf, pos)
	h.DpIfIndex = int32(ifindex)
}

func (nlmsg *NlMsgParser) takeOvsHeader() (*OvsHeader, error) {
	pos, err := nlmsg.AlignAdvance(syscall.NLMSG_ALIGNTO, SizeofOvsHeader)
	if err != nil {
		return nil, err
	}

	return ovsHeaderAt(nlmsg.data, pos), nil
}

func (ovshdr OvsHeader) datapathID() DatapathID {
	return DatapathID(ovshdr.DpIfIndex)
}

func (dpif *Dpif) checkNlMsgHeaders(msg *NlMsgParser, family int, cmd int) (*GenlMsghdr, *OvsHeader, error) {
	if _, err := msg.ExpectNlMsghdr(dpif.families[family].id); err != nil {
		return nil, nil, err
	}

	genlhdr, err := msg.CheckGenlMsghdr(cmd)
	if err != nil {
		return nil, nil, err
	}

	ovshdr, err := msg.takeOvsHeader()
	if err != nil {
		return nil, nil, err
	}

	return genlhdr, ovshdr, nil
}

type Cancelable interface {
	Cancel() error
}

type cancelableDpif struct {
	*Dpif
}

func (dpif cancelableDpif) Cancel() error {
	return dpif.Close()
}
