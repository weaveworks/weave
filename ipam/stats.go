package ipam

type Stats struct {
	IPs       uint32
	Nickname  string
	Reachable bool
}

func PeerStats(status *Status) Stats {
	peerStats := make(map[string]*Stats)
	for _, entry := range status.Entries {
		s, found := peerStats[entry.Peer]
		if !found {
			s = &Stats{Nickname: entry.Nickname, Reachable: entry.IsKnownPeer}
			peerStats[entry.Peer] = s
		}
		s.IPs += entry.Size
	}
	return peerStats
}
