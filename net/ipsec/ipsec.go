// package IPsec provides primitives for establishing IPsec in the fastdp mode.
package ipsec

// TODO(mp) install iptables rule (raw OUTPUT) which drops marked non-ESP traffic.
// TODO(mp) with non-default port!

import (
	"crypto/rand"
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
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/crypto/hkdf"

	"github.com/weaveworks/mesh"
)

type SPI uint32

const (
	protoVsn = 1

	keySize   = 36 // AES-GCM key 32 bytes + 4 bytes salt
	nonceSize = 32 // HKDF nonce size

	mark    = uint32(0x1) << 17
	markStr = "0x20000/0x20000"

	tableMangle = "mangle"
	tableFilter = "filter"
	markChain   = "WEAVE-IPSEC-MARK"
	mainChain   = "WEAVE-IPSEC"

	mask   = (SPI(1) << (mesh.PeerShortIDBits)) - 1
	spiMSB = SPI(1) << 31
)

// IPSec

type IPSec struct {
	sync.RWMutex
	ipt *iptables.IPTables
	rc  *connRefCount
	// TODO(mp) type for [16]byte
	spiByKey map[[16]byte]SPI
	spis     map[SPI]struct{}
	rekey    map[SPI]func() error
}

func New() (*IPSec, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, errors.Wrap(err, "iptables new")
	}

	ipsec := &IPSec{
		ipt:      ipt,
		rc:       newConnRefCount(),
		spiByKey: make(map[[16]byte]SPI),
		spis:     make(map[SPI]struct{}),
		rekey:    make(map[SPI]func() error),
	}

	return ipsec, nil
}

func (ipsec *IPSec) Monitor() error {
	// TODO(mp) close chan
	ch := make(chan netlink.XfrmMsg)
	errorCh := make(chan error)
	if err := netlink.XfrmMonitor(ch, nil, errorCh, nl.XFRM_MSG_EXPIRE); err != nil {
		return errors.Wrap(err, "xfrm monitor")
	}

	for {
		select {
		case err := <-errorCh:
			return err
		case msg := <-ch:
			if exp, ok := msg.(*netlink.XfrmMsgExpire); ok {
				if exp.Hard {
					ipsec.Lock()
					delete(ipsec.spis, SPI(exp.XfrmState.Spi))
					ipsec.Unlock()
				} else {
					ipsec.Lock()

					if doRekey, ok := ipsec.rekey[SPI(exp.XfrmState.Spi)]; ok {
						if err := doRekey(); err != nil {
							ipsec.Unlock()
							return errors.Wrap(err, "rekey")
						}
					}

					ipsec.Unlock()
				}
			}

		}
	}
}

// SAremote->local
func (ipsec *IPSec) ProtectInit(localPeer, remotePeer mesh.PeerName, localIP, remoteIP net.IP, dstPort int, sessionKey *[32]byte, rekey bool, send func([]byte) error) error {
	ipsec.Lock()
	defer ipsec.Unlock()

	if rekey {
		fmt.Println("ProtectInit: rekey")
	}

	if !rekey && ipsec.rc.get(localPeer, remotePeer) > 1 {
		// IPSec has been already set up between the given peers
		return nil
	}

	spiKey := connRefKey(remotePeer, localPeer)
	if rekey {
		if _, ok := ipsec.spiByKey[spiKey]; !ok {
			return fmt.Errorf("cannot find SPI by %x", spiKey)
		}
	}

	// TODO(mp) create a chain + the following:
	// iptables -t filter -A INPUT -p udp --dport ${dstPort} -s ${remoteIP} -d ${localIP} -j DROP
	// iptables -t filter -A OUTPUT -p udp --dport ${dstPort} -s ${localIP} -d ${remoteIP} -j DROP

	nonce, err := genNonce()
	if err != nil {
		return errors.Wrap(err, "generate nonce")
	}
	key, err := deriveKey(sessionKey[:], nonce, localPeer)
	if err != nil {
		return errors.Wrap(err, "derive key")
	}

	sa, err := netlink.XfrmStateAllocSpi(xfrmAllocSpiState(remoteIP, localIP))
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("ip xfrm state allocspi (in, %s, %s)", remoteIP, localIP))
	}

	spi := SPI(sa.Spi) // TODO(mp) get rid of SPI
	if sa, err := xfrmState(remoteIP, localIP, spi, false, key); err == nil {
		if err := netlink.XfrmStateUpdate(sa); err != nil {
			return errors.Wrap(err, fmt.Sprintf("xfrm state update (in, %s, %s, 0x%x)", sa.Src, sa.Dst, sa.Spi))
		}
	} else {
		return errors.Wrap(err, "new xfrm state (in)")
	}

	if err := send(composeCreateSA(nonce, spi)); err != nil {
		return errors.Wrap(err, "send CREATE_SA")
	}

	ipsec.spiByKey[spiKey] = spi
	ipsec.spis[spi] = struct{}{}
	fmt.Printf("ProtectInit: %x -> %x\n", spiKey, spi)

	return nil
}

// SAlocal->remote
// TODO(mp) rekeying
func (ipsec *IPSec) ProtectFinish(createSAMsg []byte, localPeer, remotePeer mesh.PeerName, localIP, remoteIP net.IP, dstPort int, sessionKey *[32]byte, rekey func() error) error {
	ipsec.Lock()
	defer ipsec.Unlock()

	if size := len(createSAMsg); size != createSASize {
		return fmt.Errorf("invalid CREATE_SA msg size: %d", size)
	}
	vsn, nonce, spi := parseCreateSA(createSAMsg)
	if vsn != protoVsn {
		return fmt.Errorf("unsupported vsn: %d", vsn)
	}

	spiKey := connRefKey(localPeer, remotePeer)
	oldSPI, isRekey := ipsec.spiByKey[spiKey]

	if isRekey {
		fmt.Printf("ProtectFinish: rekey: %x\n", oldSPI)
	}

	key, err := deriveKey(sessionKey[:], nonce, remotePeer)
	if err != nil {
		return errors.Wrap(err, "derive key")
	}

	if sa, err := xfrmState(localIP, remoteIP, spi, true, key); err == nil {
		if err := netlink.XfrmStateAdd(sa); err != nil {
			return errors.Wrap(err, fmt.Sprintf("xfrm state update (out, %s, %s, 0x%x)", sa.Src, sa.Dst, sa.Spi))
		}
	} else {
		return errors.Wrap(err, "new xfrm state (out)")
	}

	sp := xfrmPolicy(localIP, remoteIP, spi)
	if isRekey {
		if err := netlink.XfrmPolicyUpdate(sp); err != nil {
			return errors.Wrap(err, fmt.Sprintf("xfrm policy update (%s, %s, 0x%x)", localIP, remoteIP, spi))
		}
	} else {
		if err := netlink.XfrmPolicyAdd(sp); err != nil {
			return errors.Wrap(err, fmt.Sprintf("xfrm policy add (%s, %s, 0x%x)", localIP, remoteIP, spi))
		}
	}

	if err := ipsec.installMarkRule(localIP, remoteIP, dstPort); err != nil {
		return errors.Wrap(err, fmt.Sprintf("install mark rule (%s, %s, 0x%x)", localIP, remoteIP, dstPort))
	}

	ipsec.spiByKey[spiKey] = spi
	ipsec.spis[spi] = struct{}{}
	ipsec.rekey[spi] = rekey
	fmt.Printf("ProtectFinish: %x -> %x\n", spiKey, spi)

	// TODO(mp) delete:
	// iptables -t filter -A OUTPUT -p udp --dport ${dstPort} -s ${localIP} -d ${remoteIP} -j DROP

	return nil
}

func (ipsec *IPSec) Destroy(localPeer, remotePeer mesh.PeerName, localIP, remoteIP net.IP, remotePort int) error {
	ipsec.Lock()
	defer ipsec.Unlock()

	count := ipsec.rc.put(localPeer, remotePeer)
	switch {
	case count > 0:
		return nil
	case count < 0:
		return fmt.Errorf("IPSec invalid state")
	}

	// TODO(mp) delete if exists:
	// iptables -t filter -A OUTPUT -p udp --dport ${dstPort} -s ${localIP} -d ${remoteIP} -j DROP

	inSPIKey := connRefKey(remotePeer, localPeer)
	if inSPI, ok := ipsec.spiByKey[inSPIKey]; ok {
		inSA := &netlink.XfrmState{
			Src:   remoteIP,
			Dst:   localIP,
			Proto: netlink.XFRM_PROTO_ESP,
			Spi:   int(inSPI),
		}
		if err := netlink.XfrmStateDel(inSA); err != nil {
			return errors.Wrap(err,
				fmt.Sprintf("xfrm state del (in, %s, %s, 0x%x)", inSA.Src, inSA.Dst, inSA.Spi))
		}
		delete(ipsec.spiByKey, inSPIKey)
		delete(ipsec.spis, inSPI)
	}

	outSPIKey := connRefKey(localPeer, remotePeer)
	if outSPI, ok := ipsec.spiByKey[outSPIKey]; ok {
		if err := netlink.XfrmPolicyDel(xfrmPolicy(localIP, remoteIP, outSPI)); err != nil {
			return errors.Wrap(err,
				fmt.Sprintf("xfrm policy del (%s, %s, 0x%x)", localIP, remoteIP, outSPI))
		}

		outSA := &netlink.XfrmState{
			Src:   localIP,
			Dst:   remoteIP,
			Proto: netlink.XFRM_PROTO_ESP,
			Spi:   int(outSPI),
		}
		if err := netlink.XfrmStateDel(outSA); err != nil {
			return errors.Wrap(err,
				fmt.Sprintf("xfrm state del (out, %s, %s, 0x%x)", outSA.Src, outSA.Dst, outSA.Spi))
		}

		if err := ipsec.removeMarkRule(localIP, remoteIP, remotePort); err != nil {
			return errors.Wrap(err,
				fmt.Sprintf("remove mark rule (%s, %s, %d)", localIP, remoteIP, remotePort))
		}

		delete(ipsec.spiByKey, outSPIKey)
		delete(ipsec.spis, outSPI)
	}

	return nil

}

// Flush removes all policies/SAs established by us. Also, it removes chains and
// rules of iptables used for the marking. If destroy is true, the chains and
// the marking rule won't be re-created.
func (ipsec *IPSec) Flush(destroy bool) error {
	ipsec.Lock()
	defer ipsec.Unlock()

	policies, err := netlink.XfrmPolicyList(syscall.AF_INET)
	if err != nil {
		return errors.Wrap(err, "xfrm policy list")
	}
	for _, p := range policies {
		if p.Mark != nil && p.Mark.Value == mark && len(p.Tmpls) != 0 {
			spi := SPI(p.Tmpls[0].Spi)
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
		if _, ok := ipsec.spis[SPI(s.Spi)]; ok {
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
	ref map[[16]byte]int
}

func newConnRefCount() *connRefCount {
	return &connRefCount{ref: make(map[[16]byte]int)}
}

func (rc *connRefCount) get(srcPeer, dstPeer mesh.PeerName) int {
	key := connRefKey(srcPeer, dstPeer)
	rc.ref[key]++

	return rc.ref[key]
}

func (rc *connRefCount) put(srcPeer, dstPeer mesh.PeerName) int {
	key := connRefKey(srcPeer, dstPeer)
	rc.ref[key]--

	return rc.ref[key]
}

func connRefKey(srcPeer, dstPeer mesh.PeerName) (key [16]byte) {
	binary.BigEndian.PutUint64(key[:], uint64(srcPeer))
	binary.BigEndian.PutUint64(key[8:], uint64(dstPeer))
	return
}

// iptables

// TODO(mp) add inbound
var dropMatchingNonESPRulespec = []string{
	"!", "-p", "esp",
	"-m", "policy", "--dir", "out", "--pol", "none",
	"-m", "mark", "--mark", markStr,
	"-j", "DROP",
}

func (ipsec *IPSec) installMarkRule(srcIP, dstIP net.IP, dstPort int) error {
	rulespec := markRulespec(srcIP, dstIP, dstPort)
	if err := ipsec.ipt.AppendUnique(tableMangle, mainChain, rulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s, %s)", tableMangle, mainChain, rulespec))
	}

	return nil
}

func (ipsec *IPSec) removeMarkRule(srcIP, dstIP net.IP, dstPort int) error {
	rulespec := markRulespec(srcIP, dstIP, dstPort)
	if err := ipsec.ipt.Delete(tableMangle, mainChain, rulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s, %s)", tableMangle, mainChain, rulespec))
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
	if err := ipsec.ipt.ClearChain(tableMangle, mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables clear (%s, %s)", tableMangle, mainChain))
	}

	if err := ipsec.ipt.ClearChain(tableMangle, markChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables clear (%s, %s)", tableMangle, markChain))
	}

	if err := ipsec.ipt.AppendUnique(tableMangle, "OUTPUT", "-j", mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s)", tableMangle, "OUTPUT"))
	}

	// drop marked traffic which does not match any XFRM policy to prevent from
	// sending unencrypted packets
	if err := ipsec.ipt.AppendUnique(tableFilter, "OUTPUT", dropMatchingNonESPRulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s)", tableFilter, "OUTPUT"))
	}

	if !destroy {
		rulespec := []string{"-j", "MARK", "--set-xmark", markStr}
		if err := ipsec.ipt.Append(tableMangle, markChain, rulespec...); err != nil {
			return errors.Wrap(err, fmt.Sprintf("iptables append (%s, %s, %s)", tableMangle, markChain, rulespec))
		}

		return nil
	}

	if err := ipsec.ipt.Delete(tableFilter, "OUTPUT", dropMatchingNonESPRulespec...); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", tableFilter, "OUTPUT"))
	}

	if err := ipsec.ipt.Delete(tableMangle, "OUTPUT", "-j", mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", tableMangle, "OUTPUT"))
	}

	if err := ipsec.ipt.DeleteChain(tableMangle, mainChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", tableMangle, mainChain))
	}

	if err := ipsec.ipt.DeleteChain(tableMangle, markChain); err != nil {
		return errors.Wrap(err, fmt.Sprintf("iptables delete (%s, %s)", tableMangle, mainChain))
	}

	return nil
}

// xfrm

func xfrmAllocSpiState(srcIP, dstIP net.IP) *netlink.XfrmState {
	return &netlink.XfrmState{
		Src:          srcIP,
		Dst:          dstIP,
		Proto:        netlink.XFRM_PROTO_ESP,
		Mode:         netlink.XFRM_MODE_TRANSPORT,
		ReplayWindow: 32,
	}
}

func xfrmState(srcIP, dstIP net.IP, spi SPI, isOut bool, key []byte) (*netlink.XfrmState, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("key should be %d bytes long", keySize)
	}

	state := xfrmAllocSpiState(srcIP, dstIP)

	state.Spi = int(spi)
	state.Aead = &netlink.XfrmStateAlgo{
		Name:   "rfc4106(gcm(aes))",
		Key:    key,
		ICVLen: 128,
	}

	state.Limits = netlink.XfrmStateLimits{PacketHard: 30}
	if isOut {
		state.Limits.PacketSoft = 10
	}

	return state, nil
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
		// TODO(mp) limits
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

func genNonce() ([]byte, error) {
	buf := make([]byte, nonceSize)
	n, err := rand.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("crypto rand failed: %s", err)
	}
	if n != nonceSize {
		return nil, fmt.Errorf("not enough random data: %d", n)
	}
	return buf, nil
}

func deriveKey(sessionKey []byte, nonce []byte, peerName mesh.PeerName) ([]byte, error) {
	info := make([]byte, 8)
	binary.BigEndian.PutUint64(info, uint64(peerName))

	key := make([]byte, keySize)

	hkdf := hkdf.New(sha256.New, sessionKey, nonce, info)

	n, err := io.ReadFull(hkdf, key)
	if err != nil {
		return nil, err
	}
	if n != keySize {
		return nil, fmt.Errorf("derived too short key: %d", n)
	}

	return key, nil
}

// Protocol Messages

const createSASize = 1 + nonceSize + 32

// | 1: VSN | 32: Nonce | 32: SPI |
func composeCreateSA(nonce []byte, spi SPI) []byte {
	msg := make([]byte, createSASize)

	msg[0] = protoVsn
	copy(msg[1:(1+nonceSize)], nonce)
	binary.BigEndian.PutUint32(msg[1+nonceSize:], uint32(spi))

	return msg
}

func parseCreateSA(msg []byte) (uint8, []byte, SPI) {
	nonce := make([]byte, nonceSize)
	copy(nonce, msg[1:(1+nonceSize)])
	spi := SPI(binary.BigEndian.Uint32(msg[1+nonceSize:]))

	return msg[0], nonce, spi
}
