package mesh

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

type GossipChannel struct {
	name     string
	ourself  *LocalPeer
	routes   *Routes
	gossiper Gossiper
}

func NewGossipChannel(channelName string, ourself *LocalPeer, routes *Routes, g Gossiper) *GossipChannel {
	return &GossipChannel{
		name:     channelName,
		ourself:  ourself,
		routes:   routes,
		gossiper: g,
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
	c.senderFor(conn).Send(data)
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
	for _, conn := range c.ourself.ConnectionsTo(c.routes.BroadcastAll(srcName)) {
		c.senderFor(conn).Broadcast(srcName, update)
	}
	return nil
}

func (c *GossipChannel) relay(srcName PeerName, data GossipData) {
	c.routes.EnsureRecalculated()
	for _, conn := range c.ourself.ConnectionsTo(c.routes.RandomNeighbours(srcName)) {
		c.senderFor(conn).Send(data)
	}
}

func (c *GossipChannel) senderFor(conn Connection) *GossipSender {
	return conn.(GossipConnection).GossipSenders().Sender(c.name, c.makeGossipSender)
}

func (c *GossipChannel) makeGossipSender(sender ProtocolSender, stop <-chan struct{}) *GossipSender {
	return NewGossipSender(c.makeMsg, c.makeBroadcastMsg, sender, stop)
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
