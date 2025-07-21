package utils

import (
	"errors"
	"sync"
)

var (
	ErrEmpty = errors.New("queue empty")
	ErrFull  = errors.New("queue full")
)

// RingQueue Thread safe RingBuffer
type RingQueue[T any] struct {
	buf      []T
	capacity int
	head     int // next pop
	tail     int // next push
	size     int // length
	mu       sync.Mutex
	notEmpty *sync.Cond // block queue until not empty
	notFull  *sync.Cond // block queue until not full
}

func NewRingQueue[T any](cap int) *RingQueue[T] {
	if cap < 1 {
		panic("capacity must be > 0")
	}
	rq := &RingQueue[T]{
		buf:      make([]T, cap),
		capacity: cap,
	}
	rq.notEmpty = sync.NewCond(&rq.mu)
	rq.notFull = sync.NewCond(&rq.mu)
	return rq
}

func (q *RingQueue[T]) Enqueue(v T) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == q.capacity {
		return ErrFull
	}

	q.buf[q.tail] = v
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	q.notEmpty.Signal()
	return nil
}

func (q *RingQueue[T]) WaitEnqueue(v T) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.size == q.capacity {
		q.notFull.Wait()
	}

	q.buf[q.tail] = v
	q.tail = (q.tail + 1) % q.capacity
	q.size++
	q.notEmpty.Signal()
}

func (q *RingQueue[T]) Dequeue() (T, error) {
	var zero T

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.size == 0 {
		return zero, ErrEmpty
	}

	v := q.buf[q.head]
	q.head = (q.head + 1) % q.capacity
	q.size--
	q.notFull.Signal()
	return v, nil
}

func (q *RingQueue[T]) WaitDequeue() T {
	q.mu.Lock()
	defer q.mu.Unlock()

	for q.size == 0 {
		q.notEmpty.Wait()
	}

	v := q.buf[q.head]
	q.head = (q.head + 1) % q.capacity
	q.size--
	q.notFull.Signal()
	return v
}

func (q *RingQueue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
}

func (q *RingQueue[T]) Cap() int {
	return q.capacity
}
