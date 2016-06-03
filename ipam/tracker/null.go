package tracker

import (
	"github.com/weaveworks/weave/net/address"
)

type NullTracker struct{}

func NewNullTracker() *NullTracker {
	return &NullTracker{}
}

func (t *NullTracker) HandleUpdate(prevRanges, currRanges []address.Range) error {
	return nil
}

func (t *NullTracker) String() string {
	return "null"
}
