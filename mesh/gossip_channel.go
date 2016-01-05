package mesh

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
)

type GossipChannel struct {
	sync.Mutex
	name     string
	ourself  *LocalPeer
	routes   *Routes
	gossiper Gossiper
	senders  connectionSenders
}

type connectionSenders map[Connection]*GossipSender

func NewGossipChannel(channelName string, ourself *LocalPeer, routes *Routes, g Gossiper) *GossipChannel {
	return &GossipChannel{
		name:     channelName,
		ourself:  ourself,
		routes:   routes,
		gossiper: g,
		senders:  make(connectionSenders),
	}
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
	if c.ourself.Name == destName {
		var payload []byte
		if err := dec.Decode(&payload); err != nil {
			return err
		}
		return c.gossiper.OnGossipUnicast(srcName, payload)
	}
	if err := c.relayUnicast(destName, origPayload); err != nil {
		c.log(err)
	}
	return nil
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
	if err := c.relayBroadcast(srcName, data); err != nil {
		c.log(err)
	}
	return nil
}

func (c *GossipChannel) deliver(srcName PeerName, _ []byte, dec *gob.Decoder) error {
	var payload []byte
	if err := dec.Decode(&payload); err != nil {
		return err
	}
	update, err := c.gossiper.OnGossip(payload)
	if err != nil || update == nil {
		return err
	}
	c.relay(srcName, update)
	return nil
}

func (c *GossipChannel) GossipUnicast(dstPeerName PeerName, msg []byte) error {
	return c.relayUnicast(dstPeerName, GobEncode(c.name, c.ourself.Name, dstPeerName, msg))
}

func (c *GossipChannel) GossipBroadcast(update GossipData) error {
	return c.relayBroadcast(c.ourself.Name, update)
}

func (c *GossipChannel) Send(data GossipData) {
	c.relay(c.ourself.Name, data)
}

func (c *GossipChannel) SendDown(conn Connection, data GossipData) {
	for _, sender := range c.sendersFor([]Connection{conn}) {
		sender.Send(data)
	}
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
	c.routes.EnsureRecalculated()
	for _, sender := range c.sendersFor(c.ourself.ConnectionsTo(c.routes.BroadcastAll(srcName))) {
		sender.Broadcast(srcName, update)
	}
	return nil
}

func (c *GossipChannel) relay(srcName PeerName, data GossipData) {
	c.routes.EnsureRecalculated()
	for _, sender := range c.sendersFor(c.ourself.ConnectionsTo(c.routes.RandomNeighbours(srcName))) {
		sender.Send(data)
	}
}

func (c *GossipChannel) sendersFor(conns []Connection) []*GossipSender {
	if len(conns) == 0 {
		return nil
	}
	ourConnections := c.ourself.Connections()
	c.Lock()
	defer c.Unlock()
	// GC - randomly (courtesy of go's map iterator) pick some
	// existing senders and stop&remove them if the associated
	// connection is no longer active.  We stop as soon as we
	// encounter a valid entry; the idea being that when there is
	// little or no garbage then this executes close to O(1)[1],
	// whereas when there is lots of garbage we remove it quickly.
	//
	// [1] TODO Unfortunately, due to the desire to avoid nested
	// locks, instead of simply invoking LocalPeer.ConnectionTo(name),
	// we operate on LocalPeer.Connections(). That is
	// O(n_our_connections) at best.
	for conn, sender := range c.senders {
		if _, found := ourConnections[conn]; !found {
			delete(c.senders, conn)
			sender.Stop()
		} else {
			break
		}
	}
	// start senders, if necessary
	senders := make([]*GossipSender, len(conns), len(conns))
	for i, conn := range conns {
		sender, found := c.senders[conn]
		if !found {
			sender = NewGossipSender(c.makeMsg, c.makeBroadcastMsg, conn.(ProtocolSender))
			c.senders[conn] = sender
		}
		senders[i] = sender
	}
	return senders
}

func (c *GossipChannel) makeMsg(msg []byte) ProtocolMsg {
	return ProtocolMsg{ProtocolGossip, GobEncode(c.name, c.ourself.Name, msg)}
}

func (c *GossipChannel) makeBroadcastMsg(srcName PeerName, msg []byte) ProtocolMsg {
	return ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.name, srcName, msg)}
}

func (c *GossipChannel) log(args ...interface{}) {
	log.Println(append(append([]interface{}{}, "[gossip "+c.name+"]:"), args...)...)
}

func GobEncode(items ...interface{}) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, i := range items {
		checkFatal(enc.Encode(i))
	}
	return buf.Bytes()
}
