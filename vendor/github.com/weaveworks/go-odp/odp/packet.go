package odp

import (
	"sync"
)

type MissConsumer interface {
	Miss(packet []byte, flowKeys FlowKeys) error
	Error(err error, stopped bool)
}

func (origDP DatapathHandle) ConsumeMisses(consumer MissConsumer) (Cancelable, error) {
	// We end up needing 3 netlink sockets: one to consume
	// misses, one to consume vport events, and one for general
	// use.
	dp, err := origDP.Reopen()
	if err != nil {
		return nil, err
	}

	success := false
	defer func() {
		if !success {
			dp.dpif.Close()
		}
	}()

	missDP, err := origDP.Reopen()
	if err != nil {
		return nil, err
	}

	defer func() {
		if !success {
			missDP.dpif.Close()
		}
	}()

	// We need to set the upcall port ID on all vports.  That
	// includes vports that get added while we are listening, so
	// we need to listen for them too.
	vportConsumer := &missVportConsumer{
		dp:           dp,
		upcallPortId: missDP.dpif.sock.PortId(),
		missConsumer: consumer,
		vportsDone:   make(map[VportID]struct{}),
	}

	vportCancel, err := origDP.ConsumeVportEvents(vportConsumer)
	if err != nil {
		return nil, err
	}

	defer func() {
		if !success {
			vportCancel.Cancel()
		}
	}()

	vports, err := origDP.EnumerateVports()
	if err != nil {
		return nil, err
	}

	for _, vport := range vports {
		err = vportConsumer.setVportUpcallPortId(vport.ID)
		if err != nil {
			return nil, err
		}
	}

	success = true
	vportConsumer.cancel = vportCancel
	go missDP.consumeMisses(consumer, vportConsumer)
	return cancelableDpif{missDP.dpif}, nil
}

type missVportConsumer struct {
	dp           DatapathHandle
	upcallPortId uint32
	missConsumer MissConsumer
	cancel       Cancelable

	lock       sync.Mutex
	vportsDone map[VportID]struct{}
}

// Set a vport's upcall port ID.  This generates a OVS_VPORT_CMD_NEW
// (not a OVS_VPORT_CMD_SET), leading to a call of the New method
// below.  So we need to record which vports we already processed in
// order to avoid a vicious circle.
func (c *missVportConsumer) setVportUpcallPortId(vport VportID) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	if _, doneAlready := c.vportsDone[vport]; doneAlready {
		return nil
	}

	if err := c.dp.setVportUpcallPortId(vport, c.upcallPortId); err != nil {
		return err
	}

	c.vportsDone[vport] = struct{}{}
	return nil
}

func (c *missVportConsumer) VportCreated(dpid DatapathID, vport Vport) error {
	return c.setVportUpcallPortId(vport.ID)
}

func (c *missVportConsumer) VportDeleted(dpid DatapathID, vport Vport) error {
	c.lock.Lock()
	delete(c.vportsDone, vport.ID)
	c.lock.Unlock()
	return nil
}

func (c *missVportConsumer) Error(err error, stopped bool) {
	c.missConsumer.Error(err, stopped)
}

func (dp DatapathHandle) consumeMisses(consumer MissConsumer, vportConsumer *missVportConsumer) {
	dp.dpif.sock.consume(consumer, func(msg *NlMsgParser) error {
		if err := dp.checkNlMsgHeaders(msg, PACKET, OVS_PACKET_CMD_MISS); err != nil {
			return err
		}

		attrs, err := msg.TakeAttrs()
		if err != nil {
			return err
		}

		fkattrs, err := attrs.GetNestedAttrs(OVS_PACKET_ATTR_KEY, false)
		if err != nil {
			return err
		}

		fks, err := ParseFlowKeys(fkattrs, nil)
		if err != nil {
			return err
		}

		return consumer.Miss(attrs[OVS_PACKET_ATTR_PACKET], fks)
	})

	vportConsumer.cancel.Cancel()
	vportConsumer.dp.dpif.Close()
}

func (dp DatapathHandle) Execute(packet []byte, keys FlowKeys, actions []Action) error {
	dpif := dp.dpif

	req := NewNlMsgBuilder(RequestFlags, dpif.families[PACKET].id)
	req.PutGenlMsghdr(OVS_PACKET_CMD_EXECUTE, OVS_PACKET_VERSION)
	req.putOvsHeader(dp.ifindex)
	req.PutSliceAttr(OVS_PACKET_ATTR_PACKET, packet)

	req.PutNestedAttrs(OVS_PACKET_ATTR_KEY, func() {
		for _, k := range keys {
			k.putKeyNlAttr(req)
		}
	})

	req.PutNestedAttrs(OVS_PACKET_ATTR_ACTIONS, func() {
		for _, a := range actions {
			a.toNlAttr(req)
		}
	})

	_, err := dpif.sock.send(req)
	return err
}
