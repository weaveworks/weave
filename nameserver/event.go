package nameserver

type eventType int

const (
	eventStarted eventType = iota
	eventDestroyed
	eventDied
)

type event struct {
	eType eventType
	ident string
}

func (n *Nameserver) ContainerStarted(ident string) {}

func (n *Nameserver) ContainerDestroyed(ident string) {
	n.Lock()
	if !n.ready {
		n.pendingEvents = append(n.pendingEvents, event{eventDestroyed, ident})
		n.Unlock()
		return
	}
	entries := n.containerDestroyed(ident)
	n.Unlock()
	n.broadcastEntries(entries...)
}

func (n *Nameserver) ContainerDied(ident string) {
	n.Lock()
	if !n.ready {
		n.pendingEvents = append(n.pendingEvents, event{eventDied, ident})
		n.Unlock()
		return
	}
	entries := n.containerDied(ident)
	n.Unlock()
	n.broadcastEntries(entries...)
}

func (n *Nameserver) containerDestroyed(ident string) Entries {
	entries := n.entries.forceTombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s destroyed; tombstoning entry %s", ident, e.String())
			// Unset the flag to allow the entry be removed later on;
			// Because the flag isn't exposed to other peers, we don't
			// need to bump the version number if the entry has been previously
			// tombstoned.
			e.stopped = false
			return true
		}
		return false
	})
	n.snapshot()
	return entries
}

func (n *Nameserver) containerDied(ident string) Entries {
	entries := n.entries.tombstone(n.ourName, func(e *Entry) bool {
		if e.ContainerID == ident {
			n.infof("container %s died; tombstoning entry %s", ident, e.String())
			// We might want to restore the entry if the container comes back
			e.stopped = true
			return true
		}
		return false
	})
	n.snapshot()
	return entries
}

// TODO(mp) replayEvents could merge all entries and do a single broadcast
func (n *Nameserver) replayEvents() {
	var entries Entries
	for _, e := range n.pendingEvents {
		switch e.eType {
		case eventDestroyed:
			entries = n.containerDestroyed(e.ident)
		case eventDied:
			entries = n.containerDied(e.ident)
		}
		// TODO(mp) maybe move broadcastEntries from contented path
		n.broadcastEntries(entries...)
	}
	n.pendingEvents = nil
}
