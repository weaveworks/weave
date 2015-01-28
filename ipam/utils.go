package ipam

import (
	"bytes"
	"encoding/gob"
)

// We shouldn't ever get any errors on *encoding*, but if we do, this will make sure we get to hear about them.
func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}

// Merge with the one in router/utils.go
func GobEncode(items ...interface{}) []byte {
	buf := new(bytes.Buffer)
	enc := gob.NewEncoder(buf)
	for _, i := range items {
		if spaceSet, ok := i.(SpaceSet); ok {
			panicOnError(spaceSet.Encode(enc))
		} else {
			panicOnError(enc.Encode(i))
		}
	}
	return buf.Bytes()
}
