package onebot

import (
	"sync"
	"sync/atomic"
	"time"
)

type eventRingItem struct {
	response []byte
	caller   APICaller
}

type eventRing struct {
	ring []*atomic.Pointer[eventRingItem]
	size uintptr
	head uintptr // atomic
}

func newring(ringLen uint) *eventRing {
	ring := make([]*atomic.Pointer[eventRingItem], ringLen)
	for i := range ring {
		ring[i] = &atomic.Pointer[eventRingItem]{}
	}
	return &eventRing{
		ring: ring,
		size: uintptr(ringLen),
	}
}

var itemPool = sync.Pool{
	New: func() any {
		return &eventRingItem{}
	},
}

// non-blocking write
func (evr *eventRing) processEvent(response []byte, caller APICaller) {
	idx := atomic.AddUintptr(&evr.head, 1) - 1
	slot := evr.ring[idx%evr.size]

	item := itemPool.Get().(*eventRingItem)
	item.response = response
	item.caller = caller

	slot.Store(item)
}

func (evr *eventRing) loop(interval time.Duration, maxwait time.Duration, process func([]byte, APICaller, time.Duration)) {
	go func() {
		var tail uintptr
		timer := time.NewTicker(interval)
		defer timer.Stop()

		for range timer.C {
			idx := tail % evr.size
			slot := evr.ring[idx]
			item := slot.Swap(nil)
			if item != nil {
				process(item.response, item.caller, maxwait)
				// dispose
				item.response = nil
				item.caller = nil
				itemPool.Put(item)
			}
			tail++
		}
	}()
}
