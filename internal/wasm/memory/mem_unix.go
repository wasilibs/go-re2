//go:build unix

package memory

import (
	"fmt"
	"math"
	"sync"

	"golang.org/x/sys/unix"
)

type Memory struct {
	Buf []byte
	Max int64
	com int
	mu  sync.Mutex
}

func (m *Memory) Slice() *[]byte {
	return &m.Buf
}

func (m *Memory) Grow(delta, _ int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Buf == nil {
		m.allocate(uint64(m.Max) << 16)
	}

	sz := len(m.Buf)
	old := int64(sz >> 16)
	if delta == 0 {
		return old
	}
	newPages := old + delta
	if newPages > m.Max {
		return -1
	}
	m.reallocate(uint64(newPages) << 16)
	return old
}

func (m *Memory) allocate(maxSz uint64) {
	// Round up to the page size.
	rnd := uint64(unix.Getpagesize() - 1)
	res := (maxSz + rnd) &^ rnd

	if res > math.MaxInt {
		// This ensures int(res) overflows to a negative value,
		// and unix.Mmap returns EINVAL.
		res = math.MaxUint64
	}

	// Reserve res bytes of address space, to ensure we won't need to move it.
	// A protected, private, anonymous mapping should not commit memory.
	b, err := unix.Mmap(-1, 0, int(res), unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		panic(err)
	}
	m.Buf = b[:0]
}

func (m *Memory) reallocate(size uint64) {
	com := uint64(m.com)
	res := uint64(cap(m.Buf))
	if com < size && size <= res {
		// Grow geometrically, round up to the page size.
		rnd := uint64(unix.Getpagesize() - 1)
		newSz := com + com>>3
		newSz = min(max(size, newSz), res)
		newSz = (newSz + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		err := unix.Mprotect(m.Buf[m.com:newSz], unix.PROT_READ|unix.PROT_WRITE)
		if err != nil {
			panic(err)
		}
		m.com = int(newSz)
	}
	buf := m.Buf[:size]
	atomicStoreSliceLen(&m.Buf, len(buf))
}

func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := unix.Munmap(m.Buf[:cap(m.Buf)])
	m.Buf = nil
	m.com = 0
	return fmt.Errorf("memory: unmap failed: %w", err)
}
