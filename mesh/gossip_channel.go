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
		gossiper: g}
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
	err := c.gossiper.OnGossipBroadcast(srcName, payload)
	if err != nil {
		return err
	}
	if err := c.relayBroadcast(srcName, payload); err != nil {
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

func (c *GossipChannel) GossipBroadcast(update []byte) error {
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

func (c *GossipChannel) relayBroadcast(srcName PeerName, update []byte) error {
	c.routes.EnsureRecalculated()
	destinations := c.routes.BroadcastAll(srcName)
	if len(destinations) == 0 {
		return nil
	}
	msg := ProtocolMsg{ProtocolGossipBroadcast, GobEncode(c.name, srcName, update)}
	for _, conn := range c.ourself.ConnectionsTo(destinations) {
		conn.(ProtocolSender).GossipSender(c.name, c.gossip).SendGossip(msg)
	}
	return nil
}

func (c *GossipChannel) relay(srcName PeerName, update []byte) {
	c.routes.EnsureRecalculated()
	destinations := c.routes.RandomNeighbours(srcName)
	if len(destinations) == 0 {
		return
	}
	msg := ProtocolMsg{ProtocolGossip, GobEncode(c.name, c.ourself.Name, update)}
	for _, conn := range c.ourself.ConnectionsTo(destinations) {
		conn.(ProtocolSender).GossipSender(c.name, c.gossip).SendGossip(msg)
	}
}

func (c *GossipChannel) Send() {
	c.routes.EnsureRecalculated()
	destinations := c.routes.RandomNeighbours(c.ourself.Name)
	if len(destinations) == 0 {
		return
	}
	for _, conn := range c.ourself.ConnectionsTo(destinations) {
		c.SendDown(conn)
	}
}

func (c *GossipChannel) SendDown(conn Connection) {
	conn.(ProtocolSender).GossipSender(c.name, c.gossip).SendAllGossip()
}

func (c *GossipChannel) gossip() *ProtocolMsg {
	if gossip := c.gossiper.Gossip(); gossip != nil {
		return &ProtocolMsg{ProtocolGossip, GobEncode(c.name, c.ourself.Name, gossip)}
	}
	return nil
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
