package queue

import "sync/atomic"

// RingBuffer is a lock-free single ring buffer for pointer-like payloads.
type RingBuffer[T comparable] struct {
	entries []T
	size    uint64
	write   uint64
	read    uint64
	zero    T
}

// NewRingBuffer creates a new ring buffer with fixed size.
func NewRingBuffer[T comparable](size uint64) *RingBuffer[T] {
	return &RingBuffer[T]{
		entries: make([]T, size),
		size:    size,
	}
}

// Push attempts to enqueue one element.
func (r *RingBuffer[T]) Push(v T) bool {
	if r == nil {
		return false
	}
	for {
		writePos := atomic.LoadUint64(&r.write)
		readPos := atomic.LoadUint64(&r.read)
		nextWrite := (writePos + 1) % r.size
		if nextWrite == readPos {
			return false
		}
		if atomic.CompareAndSwapUint64(&r.write, writePos, nextWrite) {
			r.entries[writePos] = v
			return true
		}
	}
}

// DrainBatch drains up to max elements into dst and returns updated dst.
func (r *RingBuffer[T]) DrainBatch(dst []T, max int) ([]T, bool) {
	if r == nil || max <= 0 {
		return dst, false
	}
	readPos := atomic.LoadUint64(&r.read)
	writePos := atomic.LoadUint64(&r.write)
	if readPos == writePos {
		return dst, false
	}

	readCount := 0
	startPos := readPos
	for readCount < max && readPos != writePos {
		v := r.entries[readPos]
		if v != r.zero {
			dst = append(dst, v)
			r.entries[readPos] = r.zero
			readCount++
		}
		readPos = (readPos + 1) % r.size
	}
	if readCount > 0 || readPos != startPos {
		atomic.StoreUint64(&r.read, readPos)
	}
	return dst, readCount > 0
}

// IsEmpty returns true when there are no readable entries.
func (r *RingBuffer[T]) IsEmpty() bool {
	if r == nil {
		return true
	}
	return atomic.LoadUint64(&r.read) == atomic.LoadUint64(&r.write)
}
