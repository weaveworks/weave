// package IPsec provides primitives for establishing IPsec in the fastdp mode.
package ipsec

// TODO(mp) install iptables rule (raw OUTPUT) which drops marked non-ESP traffic.

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
	"golang.org/x/crypto/hkdf"

	"github.com/weaveworks/mesh"
)

type SPI uint32

const (
	keySize = 36 // AES-GCM key 32 bytes + 4 bytes salt

	mark    = uint32(0x1) << 17
	markStr = "0x20000/0x20000"

	table     = "mangle"
	markChain = "WEAVE-IPSEC-MARK"
	mainChain = "WEAVE-IPSEC"

	mask   = (SPI(1) << (mesh.PeerShortIDBits)) - 1
	spiMSB = SPI(1) << 31
)

// IPSec

type IPSec struct {
	sync.RWMutex
	ipt *iptables.IPTables
	rc  *connRefCount
}

func New() (*IPSec, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, errors.Wrap(err, "iptables new")
	}

	ipsec := &IPSec{
		ipt: ipt,
		rc:  newConnRefCount(),
	}

	return ipsec, nil
}

// Protect establishes IPsec between given peers.
func (ipsec *IPSec) Protect(srcPeer, dstPeer mesh.PeerShortID, srcIP, dstIP net.IP, dstPort int, masterKey *[32]byte) (SPI, error) {
	outSPI, err := newSPI(srcPeer, dstPeer)
	if err != nil {
		return 0,
			errors.Wrap(err, fmt.Sprintf("derive SPI (%x, %x)", srcPeer, dstPeer))
	}

	if ipsec.rc.get(srcIP, dstIP, outSPI) > 1 {
		// IPSec has been already set up between the given peers
		return outSPI, nil
	}

	inSPI, err := newSPI(dstPeer, srcPeer)
	if err != nil {
		return 0,
			errors.Wrap(err, fmt.Sprintf("derive SPI (%x, %x)", dstPeer, srcPeer))
	}

	localKey, remoteKey, err := deriveKeys(masterKey[:])
	if err != nil {
		return 0, errors.Wrap(err, "derive keys")
	}
	if srcPeer > dstPeer {
		localKey, remoteKey = remoteKey, localKey
	}

	ipsec.Lock()
	defer ipsec.Unlock()

	if inSA, err := xfrmState(dstIP, srcIP, inSPI, remoteKey); err == nil {
		if err := netlink.XfrmStateAdd(inSA); err != nil {
			return 0,
				errors.Wrap(err, fmt.Sprintf("xfrm state add (in, %s, %s, 0x%x)", inSA.Src, inSA.Dst, inSA.Spi))
		}
	} else {
		return 0, errors.Wrap(err, "new xfrm state (in)")
	}

	if outSA, err := xfrmState(srcIP, dstIP, outSPI, localKey); err == nil {
		if err := netlink.XfrmStateAdd(outSA); err != nil {
			return 0,
				errors.Wrap(err, fmt.Sprintf("xfrm state add (out, %s, %s, 0x%x)", outSA.Src, outSA.Dst, outSA.Spi))
		}
	} else {
		return 0, errors.Wrap(err, "new xfrm state (out)")
	}

	outPolicy := xfrmPolicy(srcIP, dstIP, outSPI)
	if err := netlink.XfrmPolicyAdd(outPolicy); err != nil {
		return 0,
			errors.Wrap(err, fmt.Sprintf("xfrm policy add (%s, %s, 0x%x)", srcIP, dstIP, outSPI))
	}

	if err := ipsec.installMarkRule(srcIP, dstIP, dstPort); err != nil {
		return 0,
			errors.Wrap(err, fmt.Sprintf("install mark rule (%s, %s, 0x%x)", srcIP, dstIP, dstPort))
	}

	return outSPI, nil
}

// Destroy tears down the previously established IPsec between two peers.
func (ipsec *IPSec) Destroy(srcIP, dstIP net.IP, dstPort int, outSPI SPI) error {
	var err error

	ipsec.Lock()
	defer ipsec.Unlock()

	count := ipsec.rc.put(srcIP, dstIP, outSPI)
	switch {
	case count > 0:
		return nil
	case count < 0:
		return fmt.Errorf("IPSec invalid state")
	}

	if err = netlink.XfrmPolicyDel(xfrmPolicy(srcIP, dstIP, outSPI)); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("xfrm policy del (%s, %s, 0x%x)", srcIP, dstIP, outSPI))
	}

	inSA := &netlink.XfrmState{
		Src:   srcIP,
		Dst:   dstIP,
		Proto: netlink.XFRM_PROTO_ESP,
		Spi:   int(outSPI),
	}
	outSA := &netlink.XfrmState{
		Src:   dstIP,
		Dst:   srcIP,
		Proto: netlink.XFRM_PROTO_ESP,
		Spi:   int(reverseSPI(outSPI)),
	}
	if err = netlink.XfrmStateDel(inSA); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("xfrm state del (in, %s, %s, 0x%x)", inSA.Src, inSA.Dst, inSA.Spi))
	}
	if err = netlink.XfrmStateDel(outSA); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("xfrm state del (out, %s, %s, 0x%x)", outSA.Src, outSA.Dst, outSA.Spi))
	}

	if err = ipsec.removeMarkRule(srcIP, dstIP, dstPort); err != nil {
		return errors.Wrap(err,
			fmt.Sprintf("remove mark rule (%s, %s, %d)", srcIP, dstIP, dstPort))
	}

	return nil
}

// Flush removes all policies/SAs established by us. Also, it removes chains and
// rules of iptables used for the marking. If destroy is true, the chains and
// the marking rule won't be re-created.
func (ipsec *IPSec) Flush(destroy bool) error {
	ipsec.Lock()
	defer ipsec.Unlock()

	spis := make(map[SPI]struct{})

	policies, err := netlink.XfrmPolicyList(syscall.AF_INET)
	if err != nil {
		return errors.Wrap(err, "xfrm policy list")
	}
	for _, p := range policies {
		if p.Mark != nil && p.Mark.Value == mark && len(p.Tmpls) != 0 {
			spi := SPI(p.Tmpls[0].Spi)
			spis[spi] = struct{}{}
			spis[reverseSPI(spi)] = struct{}{}

			if err := netlink.XfrmPolicyDel(&p); err != nil {
				return errors.Wrap(err, fmt.Sprintf("xfrm policy del (%s, %s, 0x%x)", p.Src, p.Dst, spi))
			}
		}
	}

	states, err := netlink.XfrmStateList(syscall.AF_INET)
	if err != nil {
		return errors.Wrap(err, "xfrm state list")
	}
	for _, s := range states {
		if _, ok := spis[SPI(s.Spi)]; ok {
			if err := netlink.XfrmStateDel(&s); err != nil {
				return errors.Wrap(err, fmt.Sprintf("xfrm state list (%s, %s, 0x%x)", s.Src, s.Dst, s.Spi))
			}
		}
	}

	if err := ipsec.resetIPTables(destroy); err != nil {
		return errors.Wrap(err, "reset ip tables")
	}

	return nil
}

// connRefCount

// Reference counting for IPsec establishments.
//
// Mesh might simultaneously create two connections for the same peer pair which
// could result in establishing IPsec multiple times.
type connRefCount struct {
	ref map[[12]byte]int
}

func newConnRefCount() *connRefCount {
	return &connRefCount{ref: make(map[[12]byte]int)}
}

func (rc *connRefCount) get(srcIP, dstIP net.IP, spi SPI) int {
	key := connRefKey(srcIP, dstIP, spi)
	rc.ref[key]++

	return rc.ref[key]
}

func (rc *connRefCount) put(srcIP, dstIP net.IP, spi SPI) int {
	key := connRefKey(srcIP, dstIP, spi)
	rc.ref[key]--

	return rc.ref[key]
}

// iptables

func (ipsec *IPSec) installMarkRule(srcIP, dstIP net.IP, dstPort int) error {
	rulespec := markRulespec(srcIP, dstIP, dstPort)
	if err := ipsec.ipt.AppendUnique(table, mainChain, rulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s, %s)", table, mainChain, rulespec))
	}

	return nil
}

func (ipsec *IPSec) removeMarkRule(srcIP, dstIP net.IP, dstPort int) error {
	rulespec := markRulespec(srcIP, dstIP, dstPort)
	if err := ipsec.ipt.Delete(table, mainChain, rulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s, %s)", table, mainChain, rulespec))
	}

	return nil
}

func markRulespec(srcIP, dstIP net.IP, dstPort int) []string {
	return []string{
		"-s", srcIP.String(), "-d", dstIP.String(),
		"-p", "udp", "--dport", strconv.FormatUint(uint64(dstPort), 10),
		"-j", markChain,
	}

}

func (ipsec *IPSec) resetIPTables(destroy bool) error {
	if err := ipsec.ipt.ClearChain(table, mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables clear (%s, %s)", table, mainChain))
	}

	if err := ipsec.ipt.ClearChain(table, markChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables clear (%s, %s)", table, markChain))
	}

	if err := ipsec.ipt.AppendUnique(table, "OUTPUT", "-j", mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s)", table, "OUTPUT"))
	}

	if !destroy {
		rulespec := []string{"-j", "MARK", "--set-xmark", markStr}
		if err := ipsec.ipt.Append(table, markChain, rulespec...); err != nil {
			return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s, %s)", table, markChain, rulespec))
		}

		return nil
	}

	if err := ipsec.ipt.Delete(table, "OUTPUT", "-j", mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", table, "OUTPUT"))
	}

	if err := ipsec.ipt.DeleteChain(table, mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", table, mainChain))
	}

	if err := ipsec.ipt.DeleteChain(table, markChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", table, mainChain))
	}

	return nil
}

// xfrm

func xfrmState(srcIP, dstIP net.IP, spi SPI, key []byte) (*netlink.XfrmState, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("key should be %d bytes long", keySize)
	}

	return &netlink.XfrmState{
		Src:   srcIP,
		Dst:   dstIP,
		Proto: netlink.XFRM_PROTO_ESP,
		Mode:  netlink.XFRM_MODE_TRANSPORT,
		Spi:   int(spi),
		Aead: &netlink.XfrmStateAlgo{
			Name:   "rfc4106(gcm(aes))",
			Key:    key,
			ICVLen: 128,
		},
	}, nil
}

func xfrmPolicy(srcIP, dstIP net.IP, spi SPI) *netlink.XfrmPolicy {
	ipMask := []byte{0xff, 0xff, 0xff, 0xff} // /32

	return &netlink.XfrmPolicy{
		Src:   &net.IPNet{IP: srcIP, Mask: ipMask},
		Dst:   &net.IPNet{IP: dstIP, Mask: ipMask},
		Proto: syscall.IPPROTO_UDP,
		Dir:   netlink.XFRM_DIR_OUT,
		Mark: &netlink.XfrmMark{
			Value: mark,
			Mask:  mark,
		},
		Tmpls: []netlink.XfrmPolicyTmpl{
			{
				Src:   srcIP,
				Dst:   dstIP,
				Proto: netlink.XFRM_PROTO_ESP,
				Mode:  netlink.XFRM_MODE_TRANSPORT,
				Spi:   int(spi),
			},
		},
	}
}

// Helpers

func newSPI(srcPeer, dstPeer mesh.PeerShortID) (SPI, error) {
	if mesh.PeerShortIDBits > 15 { // should not happen
		return 0, fmt.Errorf("PeerShortID too long")
	}

	spi := spiMSB | SPI(uint32(srcPeer)<<mesh.PeerShortIDBits|uint32(dstPeer))

	return spi, nil
}

func reverseSPI(spi SPI) SPI {
	spi ^= spiMSB

	return spiMSB | SPI(uint32(spi)>>mesh.PeerShortIDBits|uint32(spi&mask)<<mesh.PeerShortIDBits)
}

func connRefKey(srcIP, dstIP net.IP, spi SPI) (key [12]byte) {
	copy(key[:], srcIP.To4())
	copy(key[4:], dstIP.To4())
	binary.BigEndian.PutUint32(key[8:], uint32(spi))

	return
}

func deriveKeys(masterKey []byte) ([]byte, []byte, error) {
	keys := make([]byte, 2*keySize)
	hkdf := hkdf.New(sha256.New, masterKey, nil, nil)

	n, err := io.ReadFull(hkdf, keys)
	if err != nil {
		return nil, nil, err
	}
	if n != 2*keySize {
		return nil, nil, fmt.Errorf("derived too short key: %d", n)
	}

	return keys[:keySize], keys[keySize:], nil
}
