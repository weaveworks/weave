package ring

/* Exported variables for monitoring and management */

import (
	"expvar"
	"strconv"
)

var (
	expRingSize    = expvar.NewMap("ipam.ringSize")
	expRingEntries = expvar.NewMap("ipam.ringEntries")
)

type _int int

func (i _int) String() string {
	return strconv.Itoa(int(i))
}

type _uint32 uint32

func (i _uint32) String() string {
	return strconv.FormatUint(uint64(i), 10)
}

func (r *Ring) updateExportedVariables() {
	ringName := r.Start.String()
	expRingSize.Set(ringName, _uint32(r.End-r.Start))
	expRingEntries.Set(ringName, _int(len(r.Entries)))
}
