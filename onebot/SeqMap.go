package onebot

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type SeqSyncMap struct {
	mu     sync.Mutex
	read   atomic.Value // readOnly
	dirty  map[uint64]*entrySeqSyncMap
	misses int
}

type readOnlySeqSyncMap struct {
	m       map[uint64]*entrySeqSyncMap
	amended bool // true if the dirty map contains some key not in m.
}

var expungedSeqSyncMap = unsafe.Pointer(new(chan<- APIResponse))

// An entry is a slot in the map corresponding to a particular key.
type entrySeqSyncMap struct {
	p unsafe.Pointer // *interface{}
}

func newEntrySeqSyncMap(i chan<- APIResponse) *entrySeqSyncMap {
	return &entrySeqSyncMap{p: unsafe.Pointer(&i)}
}

func (m *SeqSyncMap) Load(key uint64) (value chan<- APIResponse, ok bool) {
	read, _ := m.read.Load().(readOnlySeqSyncMap)
	e, ok := read.m[key]
	if !ok && read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlySeqSyncMap)
		e, ok = read.m[key]
		if !ok && read.amended {
			e, ok = m.dirty[key]
			m.missLocked()
		}
		m.mu.Unlock()
	}
	if !ok {
		return value, false
	}
	return e.load()
}

func (e *entrySeqSyncMap) load() (value chan<- APIResponse, ok bool) {
	p := atomic.LoadPointer(&e.p)
	if p == nil || p == expungedSeqSyncMap {
		return value, false
	}
	return *(*chan<- APIResponse)(p), true
}

// Store sets the value for a key.
func (m *SeqSyncMap) Store(key uint64, value chan<- APIResponse) {
	read, _ := m.read.Load().(readOnlySeqSyncMap)
	if e, ok := read.m[key]; ok && e.tryStore(&value) {
		return
	}

	m.mu.Lock()
	read, _ = m.read.Load().(readOnlySeqSyncMap)
	if e, ok := read.m[key]; ok {
		if e.unexpungeLocked() {
			m.dirty[key] = e
		}
		e.storeLocked(&value)
	} else if e, ok := m.dirty[key]; ok {
		e.storeLocked(&value)
	} else {
		if !read.amended {
			m.dirtyLocked()
			m.read.Store(readOnlySeqSyncMap{m: read.m, amended: true})
		}
		m.dirty[key] = newEntrySeqSyncMap(value)
	}
	m.mu.Unlock()
}

func (e *entrySeqSyncMap) tryStore(i *chan<- APIResponse) bool {
	for {
		p := atomic.LoadPointer(&e.p)
		if p == expungedSeqSyncMap {
			return false
		}
		if atomic.CompareAndSwapPointer(&e.p, p, unsafe.Pointer(i)) {
			return true
		}
	}
}

func (e *entrySeqSyncMap) unexpungeLocked() (wasExpunged bool) {
	return atomic.CompareAndSwapPointer(&e.p, expungedSeqSyncMap, nil)
}

func (e *entrySeqSyncMap) storeLocked(i *chan<- APIResponse) {
	atomic.StorePointer(&e.p, unsafe.Pointer(i))
}

func (m *SeqSyncMap) LoadOrStore(key uint64, value chan<- APIResponse) (actual chan<- APIResponse, loaded bool) {
	// Avoid locking if it's a clean hit.
	read, _ := m.read.Load().(readOnlySeqSyncMap)
	if e, ok := read.m[key]; ok {
		actual, loaded, ok := e.tryLoadOrStore(value)
		if ok {
			return actual, loaded
		}
	}

	m.mu.Lock()
	read, _ = m.read.Load().(readOnlySeqSyncMap)
	if e, ok := read.m[key]; ok {
		if e.unexpungeLocked() {
			m.dirty[key] = e
		}
		actual, loaded, _ = e.tryLoadOrStore(value)
	} else if e, ok := m.dirty[key]; ok {
		actual, loaded, _ = e.tryLoadOrStore(value)
		m.missLocked()
	} else {
		if !read.amended {
			// We're adding the first new key to the dirty map.
			// Make sure it is allocated and mark the read-only map as incomplete.
			m.dirtyLocked()
			m.read.Store(readOnlySeqSyncMap{m: read.m, amended: true})
		}
		m.dirty[key] = newEntrySeqSyncMap(value)
		actual, loaded = value, false
	}
	m.mu.Unlock()

	return actual, loaded
}

func (e *entrySeqSyncMap) tryLoadOrStore(i chan<- APIResponse) (actual chan<- APIResponse, loaded, ok bool) {
	p := atomic.LoadPointer(&e.p)
	if p == expungedSeqSyncMap {
		return actual, false, false
	}
	if p != nil {
		return *(*chan<- APIResponse)(p), true, true
	}

	ic := i
	for {
		if atomic.CompareAndSwapPointer(&e.p, nil, unsafe.Pointer(&ic)) {
			return i, false, true
		}
		p = atomic.LoadPointer(&e.p)
		if p == expungedSeqSyncMap {
			return actual, false, false
		}
		if p != nil {
			return *(*chan<- APIResponse)(p), true, true
		}
	}
}

func (m *SeqSyncMap) LoadAndDelete(key uint64) (value chan<- APIResponse, loaded bool) {
	read, _ := m.read.Load().(readOnlySeqSyncMap)
	e, ok := read.m[key]
	if !ok && read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlySeqSyncMap)
		e, ok = read.m[key]
		if !ok && read.amended {
			e, ok = m.dirty[key]
			delete(m.dirty, key)
			m.missLocked()
		}
		m.mu.Unlock()
	}
	if ok {
		return e.delete()
	}
	return value, false
}

// Delete deletes the value for a key.
func (m *SeqSyncMap) Delete(key uint64) {
	m.LoadAndDelete(key)
}

func (e *entrySeqSyncMap) delete() (value chan<- APIResponse, ok bool) {
	for {
		p := atomic.LoadPointer(&e.p)
		if p == nil || p == expungedSeqSyncMap {
			return value, false
		}
		if atomic.CompareAndSwapPointer(&e.p, p, nil) {
			return *(*chan<- APIResponse)(p), true
		}
	}
}

func (m *SeqSyncMap) Range(f func(key uint64, value chan<- APIResponse) bool) {
	read, _ := m.read.Load().(readOnlySeqSyncMap)
	if read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlySeqSyncMap)
		if read.amended {
			read = readOnlySeqSyncMap{m: m.dirty}
			m.read.Store(read)
			m.dirty = nil
			m.misses = 0
		}
		m.mu.Unlock()
	}

	for k, e := range read.m {
		v, ok := e.load()
		if !ok {
			continue
		}
		if !f(k, v) {
			break
		}
	}
}

func (m *SeqSyncMap) missLocked() {
	m.misses++
	if m.misses < len(m.dirty) {
		return
	}
	m.read.Store(readOnlySeqSyncMap{m: m.dirty})
	m.dirty = nil
	m.misses = 0
}

func (m *SeqSyncMap) dirtyLocked() {
	if m.dirty != nil {
		return
	}

	read, _ := m.read.Load().(readOnlySeqSyncMap)
	m.dirty = make(map[uint64]*entrySeqSyncMap, len(read.m))
	for k, e := range read.m {
		if !e.tryExpungeLocked() {
			m.dirty[k] = e
		}
	}
}

func (e *entrySeqSyncMap) tryExpungeLocked() (isExpunged bool) {
	p := atomic.LoadPointer(&e.p)
	for p == nil {
		if atomic.CompareAndSwapPointer(&e.p, nil, expungedSeqSyncMap) {
			return true
		}
		p = atomic.LoadPointer(&e.p)
	}
	return p == expungedSeqSyncMap
}
