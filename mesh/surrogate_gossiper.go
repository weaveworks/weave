package mesh

type SurrogateGossipData struct {
	messages [][]byte
}

func NewSurrogateGossipData(msg []byte) *SurrogateGossipData {
	return &SurrogateGossipData{messages: [][]byte{msg}}
}

func (d *SurrogateGossipData) Encode() [][]byte {
	return d.messages
}

func (d *SurrogateGossipData) Merge(other GossipData) GossipData {
	o := other.(*SurrogateGossipData)
	messages := make([][]byte, 0, len(d.messages)+len(o.messages))
	messages = append(messages, d.messages...)
	messages = append(messages, o.messages...)
	return &SurrogateGossipData{messages: messages}
}

// SurrogateGossiper ignores unicasts and relays broadcasts and gossips.

type SurrogateGossiper struct{}

func (*SurrogateGossiper) OnGossipUnicast(sender PeerName, msg []byte) error {
	return nil
}

func (*SurrogateGossiper) OnGossipBroadcast(_ PeerName, update []byte) (GossipData, error) {
	return NewSurrogateGossipData(update), nil
}

func (*SurrogateGossiper) Gossip() GossipData {
	return nil
}

func (*SurrogateGossiper) OnGossip(update []byte) (GossipData, error) {
	return NewSurrogateGossipData(update), nil
}

var (
	surrogateGossiper SurrogateGossiper
)
