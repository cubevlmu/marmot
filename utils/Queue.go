package utils

import (
	"errors"
	"sync"
)

var (
	ErrEmpty  = errors.New("queue empty")
	ErrFull   = errors.New("queue full")
	ErrClosed = errors.New("queue closed")
)

type RingQueue[T any] struct {
	buf      []T
	capacity int
	head     int
	tail     int
	size     int

	mu       sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond
	closed   bool
}

func NewRingQueue[T any](cap int) *RingQueue[T] {
	if cap <= 0 {
		panic("capacity must be > 0")
	}

	q := &RingQueue[T]{
		buf:      make([]T, cap),
		capacity: cap,
	}
	q.notEmpty = sync.NewCond(&q.mu)
	q.notFull = sync.NewCond(&q.mu)
	return q
}

// Enqueue 非阻塞入队
func (q *RingQueue[T]) Enqueue(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrClosed
	}
	if q.size == q.capacity {
		return ErrFull
	}

	q.buf[q.tail] = v
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	q.notEmpty.Signal()
	return nil
}

func (q *RingQueue[T]) WaitEnqueue(v T) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.size == q.capacity && !q.closed {
		q.notFull.Wait()
	}
	if q.closed {
		return false
	}

	q.buf[q.tail] = v
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	q.notEmpty.Signal()
	return true
}

func (q *RingQueue[T]) Dequeue() (T, error) {
	var zero T

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == 0 {
		return zero, ErrEmpty
	}

	v := q.buf[q.head]

	var zeroT T
	q.buf[q.head] = zeroT

	q.head = (q.head + 1) % q.capacity
	q.size--
	q.notFull.Signal()
	return v, nil
}

func (q *RingQueue[T]) WaitDequeue() (T, bool) {
	var zero T

	q.mu.Lock()
	defer q.mu.Unlock()

	for q.size == 0 && !q.closed {
		q.notEmpty.Wait()
	}
	if q.size == 0 && q.closed {
		return zero, false
	}

	v := q.buf[q.head]
	var zeroT T
	q.buf[q.head] = zeroT

	q.head = (q.head + 1) % q.capacity
	q.size--
	q.notFull.Signal()
	return v, true
}

func (q *RingQueue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	q.notEmpty.Broadcast()
	q.notFull.Broadcast()
}

func (q *RingQueue[T]) IsClosed() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

func (q *RingQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

func (q *RingQueue[T]) Cap() int {
	return q.capacity
}
