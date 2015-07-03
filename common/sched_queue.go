package common

import (
	"container/heap"
	"math"
	"time"

	"sync/atomic"

	"github.com/benbjohnson/clock"
)

// A callable is a function that is called and return an (optional) next scheduled time
type callable func() time.Time

type schedCall struct {
	t time.Time
	c callable
}

func (sc *schedCall) FromNow(now time.Time) time.Duration {
	if sc.t.After(now) {
		return time.Duration(sc.t.Sub(now).Nanoseconds())
	}
	return time.Duration(0)
}

////////////////////////////////////////////////////////////////////////////////

// A min-heap of scheduled callables
type schedCallsHeap []*schedCall

func (ch *schedCallsHeap) Len() int           { return len(*ch) }
func (ch *schedCallsHeap) Less(i, j int) bool { return (*ch)[i].t.Before((*ch)[j].t) }
func (ch *schedCallsHeap) Swap(i, j int) {
	(*ch)[i], (*ch)[j] = (*ch)[j], (*ch)[i]
}

func (ch *schedCallsHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	entry := x.(*schedCall)
	*ch = append(*ch, entry)
}

func (ch *schedCallsHeap) Pop() interface{} {
	old := *ch
	n := len(old)
	item := old[n-1]
	*ch = old[0 : n-1]
	return item
}

////////////////////////////////////////////////////////////////////////////////

// SchedQueue is queue of scheduled callables
type SchedQueue struct {
	clock      clock.Clock
	callablesH schedCallsHeap
	schedChan  chan *schedCall
	closeChan  chan bool
	counter    uint64 // number of calls invoked so far (used for stats). Note: it will wrap.
}

// NewSchedQueue creates a new scheduled queue
func NewSchedQueue(clock clock.Clock) *SchedQueue {
	cq := SchedQueue{
		clock:     clock,
		schedChan: make(chan *schedCall),
		closeChan: make(chan bool),
	}
	heap.Init(&cq.callablesH)
	return &cq
}

// Start starts the scheduled queue
func (cq *SchedQueue) Start() {
	go func() {
		defer func() { Log.Debugf("[sched-q] Quitting (%d calls pending)", len(cq.callablesH)) }()

		var now time.Time
		timer := cq.clock.Timer(time.Duration(math.MaxInt64))

		for {
			// Wait until an insertion, until the next callable or until we are Stop()ed
			select {
			case ce := <-cq.schedChan:
				heap.Push(&cq.callablesH, ce)
				now = cq.clock.Now()
			case <-cq.closeChan:
				return
			case now = <-timer.C:
			}

			timer.Stop()
			durationUntilNext := cq.durationUntilNext(now)
			for durationUntilNext == 0 {
				sched := heap.Pop(&cq.callablesH).(*schedCall)

				atomic.AddUint64(&cq.counter, 1)
				schedNextTime := sched.c()

				if !schedNextTime.IsZero() {
					sched.t = schedNextTime
					heap.Push(&cq.callablesH, sched) // TODO: use a Fix() instead of Pop()&Push()
				}

				now = cq.clock.Now()
				durationUntilNext = cq.durationUntilNext(now)
			}

			timer = cq.clock.Timer(durationUntilNext)
		}
	}()
}

// Stop stops the scheduled queue
func (cq *SchedQueue) Stop() {
	Log.Debugf("[sched-q] Stopping...")
	cq.closeChan <- true
}

// Add schedules a call.
// The callable should not modify the scheduled queue in any way.
func (cq *SchedQueue) Add(c callable, t time.Time) {
	Log.Debugf("[sched-q] Adding call at %s", t)
	ce := schedCall{c: c, t: t}
	cq.schedChan <- &ce
}

// Counter returns the number of calls invoked in the queued
// Note: the result will wrap over time.
func (cq *SchedQueue) Count() uint64 {
	return atomic.LoadUint64(&cq.counter)
}

func (cq *SchedQueue) durationUntilNext(now time.Time) time.Duration {
	if len(cq.callablesH) > 0 {
		firstSched := cq.callablesH[0]
		return firstSched.FromNow(now)
	}
	return time.Duration(math.MaxInt64)
}

// ensure that the background goroutine has executed any pending callbacks
func (cq *SchedQueue) Flush() {
	ch := make(chan struct{})
	flushFunc := func() time.Time {
		atomic.AddUint64(&cq.counter, ^uint64(0)) // decrement so nobody sees this
		close(ch)
		return time.Time{}
	}
	cq.schedChan <- &schedCall{c: flushFunc, t: time.Time{}}
	<-ch
}
