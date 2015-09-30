package router

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
)

type GossipChannel struct {
	sync.Mutex
	name         string
	ourself      *LocalPeer
	routes       *Routes
	gossiper     Gossiper
	senders      connectionSenders
	broadcasters peerSenders
}

type connectionSenders map[Connection]*GossipSender
type peerSenders map[PeerName]*GossipSender

func NewGossipChannel(channelName string, ourself *LocalPeer, routes *Routes, g Gossiper) *GossipChannel {
	return &GossipChannel{
		name:         channelName,
		ourself:      ourself,
		routes:       routes,
		gossiper:     g,
		senders:      make(connectionSenders),
		broadcasters: make(peerSenders)}
}

func (router *Router) handleGossip(tag ProtocolTag, payload []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(payload))
	var channelName string
	if err := decoder.Decode(&channelName); err != nil {
		return err
	}
	channel := router.gossipChannel(channelName)
	var srcName PeerName
	if err := decoder.Decode(&srcName); err != nil {
		return err
	}
	switch tag {
	case ProtocolGossipUnicast:
		return channel.deliverUnicast(srcName, payload, decoder)
	case ProtocolGossipBroadcast:
		return channel.deliverBroadcast(srcName, payload, decoder)
	case ProtocolGossip:
		return channel.deliver(srcName, payload, decoder)
	}
	return nil
}

func (c *GossipChannel) deliverUnicast(srcName PeerName, origPayload []byte, dec *gob.Decoder) error {
	var destName PeerName
	if err := dec.Decode(&destName); err != nil {
		return err
	}
	if c.ourself.Name != destName {
		if err := c.relayUnicast(destName, origPayload); err != nil {
			// just log errors from relayUnicast; a problem between us and destination
			// is not enough reason to break the connection from the source
			c.log(err)
		}
		return nil
	}
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	return c.gossiper.OnGossipUnicast(srcName, payload)
}

func (c *GossipChannel) deliverBroadcast(srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	data, err := c.gossiper.OnGossipBroadcast(srcName, payload)
	if err != nil || data == nil {
		return err
	}
	return c.relayBroadcast(srcName, data)
}

func (c *GossipChannel) deliver(srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	if data, err := c.gossiper.OnGossip(payload); err != nil {
		return err
	} else if data != nil {
		c.Send(srcName, data)
	}
	return nil
}

func (c *GossipChannel) Send(srcName PeerName, data GossipData) {
	// do this outside the lock below so we avoid lock nesting
	c.routes.EnsureRecalculated()
	selectedConnections := make(ConnectionSet)
	for name := range c.routes.RandomNeighbours(srcName) {
		if conn, found := c.ourself.ConnectionTo(name); found {
			selectedConnections[conn] = void
		}
	}
	if len(selectedConnections) == 0 {
		return
	}
	connections := c.ourself.Connections()
	c.Lock()
	defer c.Unlock()
	c.gcSenders(connections)
	for conn := range selectedConnections {
		c.sendDown(conn, data)
	}
}

func (c *GossipChannel) SendDown(conn Connection, data GossipData) {
	connections := c.ourself.Connections()
	c.Lock()
	defer c.Unlock()
	c.gcSenders(connections)
	c.sendDown(conn, data)
}

// GC - randomly (courtesy of go's map iterator) pick some existing
// senders and stop&remove them if the associated connection is no
// longer active.  We stop as soon as we encounter a valid entry; the
// idea being that when there is little or no garbage then this
// executes close to O(1)[1], whereas when there is lots of garbage we
// remove it quickly.
//
// [1] TODO Unfortunately, due to the desire to avoid nested locks,
// instead of simply invoking LocalPeer.ConnectionTo(name), we pass in
// the result of LocalPeer.Connections(). That is O(n_our_connections)
// at best.
func (c *GossipChannel) gcSenders(connections ConnectionSet) {
	for conn, sender := range c.senders {
		if _, found := connections[conn]; !found {
			delete(c.senders, conn)
			sender.Stop()
		} else {
			break
		}
	}
}

// We have seen a couple of failures which suggest a >128GB slice was encountered.
// 100MB should be enough for anyone.
const maxFeasibleMessageLen = 100 * 1024 * 1024

func (c *GossipChannel) sendDown(conn Connection, data GossipData) {
	sender, found := c.senders[conn]
	if !found {
		sender = NewGossipSender(func(pending GossipData) {
			for _, msg := range pending.Encode() {
				if len(msg) > maxFeasibleMessageLen {
					panic(fmt.Sprintf("Gossip message too large: len=%d bytes; on channel '%s' from %+v", len(msg), c.name, pending))
				}
				protocolMsg := ProtocolMsg{ProtocolGossip, GobEncode(c.name, c.ourself.Name, msg)}
				conn.(ProtocolSender).SendProtocolMsg(protocolMsg)
			}
		})
		c.senders[conn] = sender
	}
	sender.Send(data)
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, msg []byte) error {
	return c.relayUnicast(dstPeerName, GobEncode(c.name, c.ourself.Name, dstPeerName, msg))
}

func (c *GossipChannel) GossipBroadcast(update GossipData) error {
	return c.relayBroadcast(c.ourself.Name, update)
}

func (c *GossipChannel) relayUnicast(dstPeerName PeerName, buf []byte) (err error) {
	if relayPeerName, found := c.routes.UnicastAll(dstPeerName); !found {
		err = fmt.Errorf("unknown relay destination: %s", dstPeerName)
	} else if conn, found := c.ourself.ConnectionTo(relayPeerName); !found {
		err = fmt.Errorf("unable to find connection to relay peer %s", relayPeerName)
	} else {
		conn.(ProtocolSender).SendProtocolMsg(ProtocolMsg{ProtocolGossipUnicast, buf})
	}
	return err
}

func (c *GossipChannel) relayBroadcast(srcName PeerName, update GossipData) error {
	names := c.routes.PeerNames() // do this outside the lock so they don't nest
	c.Lock()
	defer c.Unlock()
	// GC - randomly (courtesy of go's map iterator) pick some
	// existing broadcasters and stop&remove them if their source peer
	// is unknown. We stop as soon as we encounter a valid entry; the
	// idea being that when there is little or no garbage then this
	// executes close to O(1)[1], whereas when there is lots of
	// garbage we remove it quickly.
	//
	// [1] TODO Unfortunately, due to the desire to avoid nested
	// locks, instead of simply invoking Peers.Fetch(name) below, we
	// have that Peers.Names() invocation above. That is O(n_peers) at
	// best.
	for name, broadcaster := range c.broadcasters {
		if _, found := names[name]; !found {
			delete(c.broadcasters, name)
			broadcaster.Stop()
		} else {
			break
		}
	}
	broadcaster, found := c.broadcasters[srcName]
	if !found {
		broadcaster = NewGossipSender(func(pending GossipData) { c.sendBroadcast(srcName, pending) })
		c.broadcasters[srcName] = broadcaster
	}
	broadcaster.Send(update)
	return nil
}

func (c *GossipChannel) sendBroadcast(srcName PeerName, update GossipData) {
	c.routes.EnsureRecalculated()
	nextHops := c.routes.BroadcastAll(srcName)
	if len(nextHops) == 0 {
		return
	}
	connections := c.ourself.ConnectionsTo(nextHops)
	for _, msg := range update.Encode() {
		protocolMsg := ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.name, srcName, msg)}
		// FIXME a single blocked connection can stall us
		for _, conn := range connections {
			conn.(ProtocolSender).SendProtocolMsg(protocolMsg)
		}
	}
}

func (c *GossipChannel) log(args ...interface{}) {
	log.Println(append(append([]interface{}{}, "[gossip "+c.name+"]:"), args...)...)
}
