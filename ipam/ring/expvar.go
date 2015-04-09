package ring

/* Exported variables for monitoring and management */

import (
	"expvar"
	"strconv"

	"github.com/weaveworks/weave/ipam/utils"
)

var (
	expRingSize       = expvar.NewMap("ipam.ringSize")
	expRingEntries    = expvar.NewMap("ipam.ringEntries")
	expRingTombstones = expvar.NewMap("ipam.ringTombstones")
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
	ringName := utils.IntIP4(r.Start).String()
	expRingSize.Set(ringName, _uint32(r.End-r.Start))
	expRingEntries.Set(ringName, _int(len(r.Entries)))
	expRingTombstones.Set(ringName, _int(len(r.Entries)-len(r.Entries.filteredEntries())))
}
