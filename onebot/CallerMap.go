package onebot

import (
	"sync"
	"sync/atomic"
	"unsafe"
)

type callerMap struct {
	mu     sync.Mutex
	read   atomic.Value // readOnly
	dirty  map[int64]*entryCallerMap
	misses int
}

type readOnlyCallerMap struct {
	m       map[int64]*entryCallerMap
	amended bool // true if the dirty map contains some key not in m.
}

var expungedCallerMap = unsafe.Pointer(new(APICaller))

type entryCallerMap struct {
	p unsafe.Pointer // *interface{}
}

func newEntryCallerMap(i APICaller) *entryCallerMap {
	return &entryCallerMap{p: unsafe.Pointer(&i)}
}

func (m *callerMap) Load(key int64) (value APICaller, ok bool) {
	read, _ := m.read.Load().(readOnlyCallerMap)
	e, ok := read.m[key]
	if !ok && read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlyCallerMap)
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

func (e *entryCallerMap) load() (value APICaller, ok bool) {
	p := atomic.LoadPointer(&e.p)
	if p == nil || p == expungedCallerMap {
		return value, false
	}
	return *(*APICaller)(p), true
}

// Store sets the value for a key.
func (m *callerMap) Store(key int64, value APICaller) {
	read, _ := m.read.Load().(readOnlyCallerMap)
	if e, ok := read.m[key]; ok && e.tryStore(&value) {
		return
	}

	m.mu.Lock()
	read, _ = m.read.Load().(readOnlyCallerMap)
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
			m.read.Store(readOnlyCallerMap{m: read.m, amended: true})
		}
		m.dirty[key] = newEntryCallerMap(value)
	}
	m.mu.Unlock()
}

func (e *entryCallerMap) tryStore(i *APICaller) bool {
	for {
		p := atomic.LoadPointer(&e.p)
		if p == expungedCallerMap {
			return false
		}
		if atomic.CompareAndSwapPointer(&e.p, p, unsafe.Pointer(i)) {
			return true
		}
	}
}

func (e *entryCallerMap) unexpungeLocked() (wasExpunged bool) {
	return atomic.CompareAndSwapPointer(&e.p, expungedCallerMap, nil)
}

func (e *entryCallerMap) storeLocked(i *APICaller) {
	atomic.StorePointer(&e.p, unsafe.Pointer(i))
}

func (m *callerMap) LoadOrStore(key int64, value APICaller) (actual APICaller, loaded bool) {
	// Avoid locking if it's a clean hit.
	read, _ := m.read.Load().(readOnlyCallerMap)
	if e, ok := read.m[key]; ok {
		actual, loaded, ok := e.tryLoadOrStore(value)
		if ok {
			return actual, loaded
		}
	}

	m.mu.Lock()
	read, _ = m.read.Load().(readOnlyCallerMap)
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
			m.dirtyLocked()
			m.read.Store(readOnlyCallerMap{m: read.m, amended: true})
		}
		m.dirty[key] = newEntryCallerMap(value)
		actual, loaded = value, false
	}
	m.mu.Unlock()

	return actual, loaded
}

func (e *entryCallerMap) tryLoadOrStore(i APICaller) (actual APICaller, loaded, ok bool) {
	p := atomic.LoadPointer(&e.p)
	if p == expungedCallerMap {
		return actual, false, false
	}
	if p != nil {
		return *(*APICaller)(p), true, true
	}

	ic := i
	for {
		if atomic.CompareAndSwapPointer(&e.p, nil, unsafe.Pointer(&ic)) {
			return i, false, true
		}
		p = atomic.LoadPointer(&e.p)
		if p == expungedCallerMap {
			return actual, false, false
		}
		if p != nil {
			return *(*APICaller)(p), true, true
		}
	}
}

func (m *callerMap) LoadAndDelete(key int64) (value APICaller, loaded bool) {
	read, _ := m.read.Load().(readOnlyCallerMap)
	e, ok := read.m[key]
	if !ok && read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlyCallerMap)
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

func (m *callerMap) Delete(key int64) {
	m.LoadAndDelete(key)
}

func (e *entryCallerMap) delete() (value APICaller, ok bool) {
	for {
		p := atomic.LoadPointer(&e.p)
		if p == nil || p == expungedCallerMap {
			return value, false
		}
		if atomic.CompareAndSwapPointer(&e.p, p, nil) {
			return *(*APICaller)(p), true
		}
	}
}

func (m *callerMap) Range(f func(key int64, value APICaller) bool) {
	read, _ := m.read.Load().(readOnlyCallerMap)
	if read.amended {
		m.mu.Lock()
		read, _ = m.read.Load().(readOnlyCallerMap)
		if read.amended {
			read = readOnlyCallerMap{m: m.dirty}
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

func (m *callerMap) missLocked() {
	m.misses++
	if m.misses < len(m.dirty) {
		return
	}
	m.read.Store(readOnlyCallerMap{m: m.dirty})
	m.dirty = nil
	m.misses = 0
}

func (m *callerMap) dirtyLocked() {
	if m.dirty != nil {
		return
	}

	read, _ := m.read.Load().(readOnlyCallerMap)
	m.dirty = make(map[int64]*entryCallerMap, len(read.m))
	for k, e := range read.m {
		if !e.tryExpungeLocked() {
			m.dirty[k] = e
		}
	}
}

func (e *entryCallerMap) tryExpungeLocked() (isExpunged bool) {
	p := atomic.LoadPointer(&e.p)
	for p == nil {
		if atomic.CompareAndSwapPointer(&e.p, nil, expungedCallerMap) {
			return true
		}
		p = atomic.LoadPointer(&e.p)
	}
	return p == expungedCallerMap
}
