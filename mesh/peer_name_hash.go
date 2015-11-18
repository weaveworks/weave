// +build peer_name_hash

// Let peer names be sha256 hashes...of anything (as long as it's
// unique).

package mesh

import (
	"crypto/sha256"
	"encoding/hex"
)

type PeerName string

const (
	PeerNameFlavour = "hash"
	NameSize        = sha256.Size >> 1
	UnknownPeerName = PeerName("")
)

func PeerNameFromUserInput(userInput string) (PeerName, error) {
	// fixed-length identity
	nameByteAry := sha256.Sum256([]byte(userInput))
	return PeerNameFromBin(nameByteAry[:NameSize]), nil
}

func PeerNameFromString(nameStr string) (PeerName, error) {
	if _, err := hex.DecodeString(nameStr); err != nil {
		return UnknownPeerName, err
	}
	return PeerName(nameStr), nil
}

func PeerNameFromBin(nameByte []byte) PeerName {
	return PeerName(hex.EncodeToString(nameByte))
}

func (name PeerName) Bin() []byte {
	res, err := hex.DecodeString(string(name))
	checkFatal(err)
	return res
}

func (name PeerName) String() string {
	return string(name)
}
