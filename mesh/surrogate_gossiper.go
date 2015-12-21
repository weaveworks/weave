package mesh

// SurrogateGossiper ignores unicasts and relays broadcasts and gossips.

type SurrogateGossiper struct{}

func (*SurrogateGossiper) OnGossipUnicast(sender PeerName, msg []byte) error {
	return nil
}

func (*SurrogateGossiper) OnGossipBroadcast(_ PeerName, update []byte) error {
	return nil
}

func (*SurrogateGossiper) Gossip() []byte {
	return nil
}

func (*SurrogateGossiper) OnGossip(update []byte) ([]byte, error) {
	return update, nil
}

var (
	surrogateGossiper SurrogateGossiper
)
