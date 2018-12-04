package tracker

import (
	"github.com/weaveworks/weave/net/address"
)

// Merge merges adjacent range entries.
// The given slice has to be sorted in increasing order.
func Merge(r []address.Range) []address.Range {
	var merged []address.Range

	for i := range r {
		if prev := len(merged) - 1; prev >= 0 && merged[prev].End == r[i].Start {
			merged[prev].End = r[i].End
		} else {
			merged = append(merged, r[i])
		}
	}

	return merged
}

// RemoveCommon filters out CIDR ranges which are contained in both a and b slices.
// Both slices have to be sorted in increasing order.
func RemoveCommon(a, b []address.CIDR) (newA, newB []address.CIDR) {
	i, j := 0, 0

	for i < len(a) && j < len(b) {
		switch {
		case a[i].Start() < b[j].Start() || a[i].End() < b[j].End():
			newA = append(newA, a[i])
			i++
		case a[i].Start() > b[j].Start() || a[i].End() > b[j].End():
			newB = append(newB, b[j])
			j++
		default:
			i++
			j++
		}

	}
	newA = append(newA, a[i:]...)
	newB = append(newB, b[j:]...)

	return
}
