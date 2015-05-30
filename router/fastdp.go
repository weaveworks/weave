package router

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/weaveworks/go-odp/odp"
)

type thunk func() FlowOp

// A bridgePortID is either an ODP vport or the router.  We express it
// like this to use it as a map key.
type bridgePortID struct {
	vport  odp.VportID
	router bool
}

var routerPortID = bridgePortID{router: true}

type FastDatapath struct {
	vxlanPort int

	// The lock guards rthe FastDatapath state, and also
	// synchronises use of the dpif
	lock              sync.Mutex
	dpif              *odp.Dpif
	dp                odp.DatapathHandle
	consumingMisses   bool
	missHandlers      map[odp.VportID]func(odp.FlowKeys) thunk
	vxlanVportID      odp.VportID
	localPeer         *Peer
	peers             *Peers
	interHostConsumer InterHostConsumer

	// Bridge state: A bridge port is represented as a function
	// that says how to send out a packet on that port.
	bridgePorts map[bridgePortID]func(PacketKey) thunk
	bridgeMACs  map[MAC]func(PacketKey) thunk

	// Only accessed from Miss, so not locked
	dec *EthernetDecoder
}

func NewFastDatapath(dpname string, vxlanPort int) (*FastDatapath, error) {
	dpif, err := odp.NewDpif()
	if err != nil {
		return nil, err
	}

	success := false
	defer func() {
		if !success {
			dpif.Close()
		}
	}()

	dp, err := dpif.LookupDatapath(dpname)
	if err != nil {
		return nil, err
	}

	fastdp := &FastDatapath{
		vxlanPort:    vxlanPort,
		dpif:         dpif,
		dp:           dp,
		missHandlers: make(map[odp.VportID]func(odp.FlowKeys) thunk),
		bridgePorts:  make(map[bridgePortID]func(PacketKey) thunk),
		bridgeMACs:   make(map[MAC]func(PacketKey) thunk),
		dec:          NewEthernetDecoder(),
	}

	if err := fastdp.deleteVxlanVports(); err != nil {
		return nil, err
	}

	if err := fastdp.clearFlows(); err != nil {
		return nil, err
	}

	fastdp.vxlanVportID, err = fastdp.dp.CreateVport(
		odp.NewVxlanVportSpec("vxlan", uint16(vxlanPort)))
	if err != nil {
		return nil, err
	}

	success = true
	return fastdp, nil
}

func (fastdp *FastDatapath) consumeMisses() error {
	if !fastdp.consumingMisses {
		if err := fastdp.dp.ConsumeMisses(fastdp); err != nil {
			return err
		}

		fastdp.consumingMisses = true
	}

	return nil
}

func (fastdp *FastDatapath) Close() error {
	err := fastdp.dpif.Close()
	fastdp.dpif = nil
	return err
}

func (fastdp *FastDatapath) clearFlows() error {
	flows, err := fastdp.dp.EnumerateFlows()
	if err != nil {
		return err
	}

	for _, flow := range flows {
		err = fastdp.dp.DeleteFlow(flow.FlowSpec)
		if err != nil && !odp.IsNoSuchFlowError(err) {
			return err
		}
	}

	return nil
}

func (fastdp *FastDatapath) deleteVxlanVports() error {
	vports, err := fastdp.dp.EnumerateVports()
	if err != nil {
		return err
	}

	for _, vport := range vports {
		if vport.Spec.TypeName() != "vxlan" {
			continue
		}

		err = fastdp.dp.DeleteVport(vport.ID)
		if err != nil && !odp.IsNoSuchVportError(err) {
			return err
		}
	}

	return nil
}

func (fastdp *FastDatapath) Error(err error, stopped bool) {
	// XXX fatal if stopped
	log.Println("Error while listening on datapath:", err)
}

func (fastdp *FastDatapath) Miss(packet []byte, flowKeys odp.FlowKeys) error {
	thunk := fastdp.handleMiss(flowKeys)
	if thunk != nil {
		fastdp.send(thunk(), packet)
	}

	return nil
}

func (fastdp *FastDatapath) handleMiss(fks odp.FlowKeys) thunk {
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()

	ingress := fks[odp.OVS_KEY_ATTR_IN_PORT].(odp.InPortFlowKey).VportID()

	log.Println("Got miss", fks, "on port", ingress)

	handler := fastdp.missHandlers[ingress]
	if handler == nil {
		handler = fastdp.makeMissHandler(ingress)
		if handler == nil {
			return simpleThunk(vetoFlowCreation)
		}

		fastdp.missHandlers[ingress] = handler
	}

	return handler(fks)
}

func (fastdp *FastDatapath) makeMissHandler(
	ingress odp.VportID) func(odp.FlowKeys) thunk {
	// Auto-add netdev and internal vports to the bridge (i.e. not
	// vxlan vports)
	vport, err := fastdp.dp.LookupVport(ingress)
	if err != nil {
		log.Println(err)
		return nil
	}

	typ := vport.Spec.TypeName()
	if typ != "netdev" && typ != "internal" {
		return nil
	}

	// Construct a bridge port for the netdev, which simply
	// outputs packets
	bridgePort := func(key PacketKey) thunk {
		return simpleThunk(fastdp.odpActions(
			odp.NewOutputAction(ingress)))
	}
	fastdp.bridgePorts[bridgePortID{vport: ingress}] = bridgePort

	// Clear flows, in order to recalculate flows for broadcasts
	// on the bridge
	checkWarn(fastdp.clearFlows())

	// Packets from the netdev are processed by the bridge
	return func(fks odp.FlowKeys) thunk {
		return multiThunk(fastdp.bridge(bridgePortID{vport: ingress},
			flowKeysToPacketKey(fks)))
	}
}

func flowKeysToPacketKey(fks odp.FlowKeys) PacketKey {
	eth := fks[odp.OVS_KEY_ATTR_ETHERNET].(odp.EthernetFlowKey).Key()
	return PacketKey{SrcMAC: eth.EthSrc, DstMAC: eth.EthDst}
}

// Send a packet, creating a corresponding ODP flow rule if possible
func (fastdp *FastDatapath) send(fops FlowOp, frame []byte) {
	// Gather the actions from actionFlowOps, execute any others
	var dec *EthernetDecoder
	flow := odp.NewFlowSpec()
	createFlow := true

	for _, xfop := range FlattenFlowOp(fops) {
		switch fop := xfop.(type) {
		case interface {
			updateFlowSpec(*odp.FlowSpec)
		}:
			fop.updateFlowSpec(&flow)
		case vetoFlowCreationFlowOp:
			createFlow = false
		default:
			// A foreign flow op, so send the packet the
			// normal way, decoding the packet lazily.
			if dec == nil {
				dec = fastdp.dec
				dec.DecodeLayers(frame)
				createFlow = false
			}

			if len(dec.decoded) != 0 {
				fop.Send(frame, dec, false)
			}
		}
	}

	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()

	if len(flow.Actions) != 0 {
		checkWarn(fastdp.dp.Execute(frame, nil, flow.Actions))
	}

	if createFlow {
		log.Println("Creating flow", flow)
		checkWarn(fastdp.dp.CreateFlow(flow))
	}
}

type odpActionsFlowOp struct {
	fastdp  *FastDatapath
	actions []odp.Action
}

func (fastdp *FastDatapath) odpActions(actions ...odp.Action) FlowOp {
	return odpActionsFlowOp{
		fastdp:  fastdp,
		actions: actions,
	}
}

func (fop odpActionsFlowOp) updateFlowSpec(flow *odp.FlowSpec) {
	flow.AddActions(fop.actions)
}

func (fop odpActionsFlowOp) Send(frame []byte, dec *EthernetDecoder, bc bool) {
	fastdp := fop.fastdp
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()
	checkWarn(fastdp.dp.Execute(frame, nil, fop.actions))
}

type nopFlowOp struct{}

func (nopFlowOp) Send([]byte, *EthernetDecoder, bool) {
	// A nopFlowOp just provides a hint about flow creation, it
	// doesn't send anything
}

type vetoFlowCreationFlowOp struct {
	nopFlowOp
}

var vetoFlowCreation = vetoFlowCreationFlowOp{}

type odpFlowKeyFlowOp struct {
	key odp.FlowKey
	nopFlowOp
}

func odpFlowKey(key odp.FlowKey) FlowOp {
	return odpFlowKeyFlowOp{key: key}
}

func (fop odpFlowKeyFlowOp) updateFlowSpec(flow *odp.FlowSpec) {
	flow.AddKey(fop.key)
}

func odpEthernetFlowKey(key PacketKey) FlowOp {
	fk := odp.NewEthernetFlowKey()
	fk.SetEthSrc(key.SrcMAC)
	fk.SetEthDst(key.DstMAC)
	return odpFlowKeyFlowOp{key: fk}
}

func simpleThunk(fop FlowOp) thunk {
	return func() FlowOp { return fop }
}

func callThunks(thunks []thunk) FlowOp {
	if len(thunks) == 1 {
		return thunks[0]()
	}

	mfop := NewMultiFlowOp(false)
	for _, thunk := range thunks {
		mfop.Add(thunk())
	}

	return mfop
}

func multiThunk(thunks []thunk) thunk {
	return func() FlowOp { return callThunks(thunks) }
}

// IntraHost

func (fastdp *FastDatapath) ConsumeIntraHostPackets(
	consumer IntraHostConsumer) error {
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()

	if fastdp.bridgePorts[routerPortID] != nil {
		panic("FastDatapath already has an IntraHostConsumer")
	}

	fastdp.bridgePorts[routerPortID] = func(key PacketKey) thunk {
		return func() FlowOp {
			return consumer.CapturedPacket(key)
		}
	}

	return fastdp.consumeMisses()
}

func (fastdp *FastDatapath) InjectPacket(key PacketKey) FlowOp {
	return callThunks(func() []thunk {
		fastdp.lock.Lock()
		defer fastdp.lock.Unlock()
		return fastdp.bridge(routerPortID, key)
	}())
}

// A trivial bridge implementation
func (fastdp *FastDatapath) bridge(ingress bridgePortID,
	key PacketKey) []thunk {
	if fastdp.bridgeMACs[key.SrcMAC] == nil {
		// Learn the source MAC
		fastdp.bridgeMACs[key.SrcMAC] = fastdp.bridgePorts[ingress]
	}

	// If we know about the destination MAC, deliver it to the
	// associated port.
	dst := fastdp.bridgeMACs[key.DstMAC]
	if dst != nil {
		return []thunk{simpleThunk(odpEthernetFlowKey(key)), dst(key)}
	}

	// Otherwise, it might be a real broadcast, or it might just
	// be for a MAC we don't know about yet.  Either way, we'll
	// broadcast it.
	var fop FlowOp
	thunks := make([]thunk, 1, len(fastdp.bridgePorts)+2)

	if (key.DstMAC[0] & 1) == 0 {
		// Not a real broadcast, so doon't create a flow rule.
		// If we did, we'd need to clear the flows every time
		// we learned a new MAC address, or have a more
		// complicated selective invalidation scheme.
		fop = vetoFlowCreation
	} else {
		// A real broadcast
		fop = odpEthernetFlowKey(key)

		if !ingress.router {
			// The rule depends on the ingress vport.
			thunks = thunks[:2]
			thunks[1] = simpleThunk(odpFlowKey(odp.NewInPortFlowKey(
				ingress.vport)))
		}
	}

	thunks[0] = simpleThunk(fop)

	// Send to all ports except the one it came in on.
	for id, dst := range fastdp.bridgePorts {
		if id != ingress {
			thunks = append(thunks, dst(key))
		}
	}

	return thunks
}

// InterHost

func (fastdp *FastDatapath) ConsumeInterHostPackets(
	localPeer *Peer, peers *Peers, consumer InterHostConsumer) error {
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()

	if fastdp.missHandlers[fastdp.vxlanVportID] != nil {
		panic("FastDatapath already has an InterHostConsumer")
	}

	fastdp.localPeer = localPeer
	fastdp.peers = peers

	fastdp.missHandlers[fastdp.vxlanVportID] = func(fks odp.FlowKeys) thunk {
		tunnel := fks[odp.OVS_KEY_ATTR_TUNNEL].(odp.TunnelFlowKey).Key()
		srcPeer, dstPeer := fastdp.extractPeers(tunnel.TunnelId)
		if srcPeer == nil || dstPeer == nil {
			return simpleThunk(vetoFlowCreation)
		}

		key := ForwardPacketKey{
			SrcPeer:   srcPeer,
			DstPeer:   dstPeer,
			PacketKey: flowKeysToPacketKey(fks),
		}

		// The resulting flow rule should be restricted to
		// packets with the same tunnelID
		var tunnelFlowKey odp.TunnelFlowKey
		tunnelFlowKey.SetTunnelId(tunnel.TunnelId)
		tunnelFlowKey.SetIpv4Src(tunnel.Ipv4Src)
		tunnelFlowKey.SetIpv4Dst(tunnel.Ipv4Dst)

		return func() FlowOp {
			fop := NewMultiFlowOp(false)
			fop.Add(odpFlowKey(tunnelFlowKey))
			fop.Add(consumer(key))
			return fop
		}
	}

	return fastdp.consumeMisses()
}

func (fastdp *FastDatapath) InvalidateRoutes() {
	fmt.Println("InvalidateRoutes")
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()
	checkWarn(fastdp.clearFlows())
}

func (fastdp *FastDatapath) InvalidateShortIDs() {
	fmt.Println("InvalidateShortIDs")
	fastdp.lock.Lock()
	defer fastdp.lock.Unlock()
	checkWarn(fastdp.clearFlows())
}

func (fastdp *FastDatapath) extractPeers(tunnelID [8]byte) (*Peer, *Peer) {
	if fastdp.peers == nil {
		return nil, nil
	}

	vni := binary.BigEndian.Uint64(tunnelID[:])
	srcPeer := fastdp.peers.FetchByShortID(PeerShortID(vni & 0xfff))
	dstPeer := fastdp.peers.FetchByShortID(PeerShortID((vni >> 12) & 0xfff))
	return srcPeer, dstPeer
}

type FastDatapathForwarder struct {
	fastdp         *FastDatapath
	remotePeer     *Peer
	localIP        [4]byte
	sendControlMsg func([]byte) error

	lock        sync.Mutex
	listener    InterHostForwarderListener
	remoteIP    [4]byte
	established bool
}

func (fastdp *FastDatapath) MakeForwarder(remotePeer *Peer, localIP net.IP,
	remote *net.UDPAddr, connUID uint64, crypto InterHostCrypto,
	sendControlMsg func([]byte) error) (InterHostForwarder, error) {
	if len(localIP) != 4 {
		return nil, fmt.Errorf("local IP address %s is not IPv4", localIP)
	}

	fwd := &FastDatapathForwarder{
		fastdp:         fastdp,
		remotePeer:     remotePeer,
		sendControlMsg: sendControlMsg,
	}
	copy(fwd.localIP[:], localIP)
	return fwd, nil
}

func (fwd *FastDatapathForwarder) SetListener(
	listener InterHostForwarderListener) {
	fwd.lock.Lock()
	defer fwd.lock.Unlock()

	if listener == nil {
		panic("nil listener")
	}

	fwd.listener = listener
	if fwd.established {
		listener.Established()
	}

	fwd.sendControlMsg(fwd.localIP[:])
}

func (fwd *FastDatapathForwarder) ControlMessage(msg []byte) {
	fwd.lock.Lock()
	defer fwd.lock.Unlock()

	if len(msg) != 4 {
		if fwd.listener != nil {
			fwd.listener.Error(fmt.Errorf("FastDatapath control message wrong length %d", len(msg)))
			fwd.listener = nil
		}

		return
	}

	if !fwd.established {
		copy(fwd.remoteIP[:], msg)
		fwd.established = true
		if fwd.listener != nil {
			fwd.listener.Established()
		}
	}
}

func (fwd *FastDatapathForwarder) Forward(key ForwardPacketKey) FlowOp {
	fwd.lock.Lock()
	defer fwd.lock.Unlock()
	if !fwd.established {
		// Ideally we could just return nil.  But then we
		// would have to invalidate the resulting flows when
		// we learn the remote IP.  So for now, just prevent
		// flows.
		return vetoFlowCreation
	}

	var sta odp.SetTunnelAction
	sta.SetTunnelId(tunnelIDFor(key))
	sta.SetIpv4Src(fwd.localIP)
	sta.SetIpv4Dst(fwd.remoteIP)
	sta.SetTos(0)
	sta.SetTtl(64)
	sta.SetDf(true)
	sta.SetCsum(false)

	return fwd.fastdp.odpActions(sta,
		odp.NewOutputAction(fwd.fastdp.vxlanVportID))
}

func tunnelIDFor(key ForwardPacketKey) (tunnelID [8]byte) {
	src := uint64(key.SrcPeer.ShortID)
	dst := uint64(key.DstPeer.ShortID)
	binary.BigEndian.PutUint64(tunnelID[:], src|dst<<12)
	return
}

func (fwd *FastDatapathForwarder) Close() {
	// Ideally we would delete all the relevant flows here.  But
	// until we do that, it's probably not worth clearing all
	// flows.
}
