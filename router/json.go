package router

import (
	"encoding/json"
)

func (peers *Peers) MarshalJSON() ([]byte, error) {
	ps := make([]*Peer, 0)
	peers.ForEach(func(_ PeerName, peer *Peer) { ps = append(ps, peer) })
	return json.Marshal(ps)
}

func (peer *Peer) MarshalJSON() ([]byte, error) {
	conns := make([]Connection, 0)
	peer.ForEachConnection(func(remoteName PeerName, conn Connection) {
		conns = append(conns, conn)
	})
	return json.Marshal(struct {
		Name        string
		Connections []Connection
	}{peer.Name.String(), conns})
}

func (conn *RemoteConnection) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RemoteName, TcpAddr string
		Established         bool
	}{conn.Remote().Name.String(), conn.RemoteTCPAddr(), conn.Established()})
}
