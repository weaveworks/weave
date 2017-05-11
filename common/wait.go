package common

import (
	"sync"
	"sync/atomic"
)

type WaitGroup struct {
	wg    sync.WaitGroup
	count int32
}

// Increase the count of things waiting and return a closure which calls Done
func (w *WaitGroup) Add() func() {
	atomic.AddInt32(&w.count, 1)
	w.wg.Add(1)
	return func() {
		w.wg.Done()
		atomic.AddInt32(&w.count, -1)
	}
}

func (w *WaitGroup) IsDone() bool {
	return atomic.LoadInt32(&w.count) == 0
}
