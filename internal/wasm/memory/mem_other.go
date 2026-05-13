//go:build !unix && !windows

package memory

import "sync"

// Memory implements LinearMemory for platforms without mmap support.
// It uses a pre-allocated buffer to ensure memory never moves,
// which is critical for shared wasm memory.
type Memory struct {
	backing []byte // Full preallocated capacity
	Buf     []byte // Current logical view
	Max     int64
	mu      sync.Mutex
}

func (m *Memory) Slice() *[]byte {
	return &m.Buf
}

func (m *Memory) Grow(delta, _ int64) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	sz := len(m.Buf)
	old := int64(sz >> 16)
	if delta == 0 {
		return old
	}

	new := old + delta
	newBytes := int(new) << 16
	if new > m.Max || newBytes < 0 {
		return -1
	}

	// If this is the first call, allocate the full backing buffer
	if m.backing == nil {
		maxBytes := int(m.Max) << 16
		if maxBytes < 0 {
			return -1
		}
		m.backing = make([]byte, maxBytes)
		m.Buf = m.backing[:0]
	}

	// Grow by slicing the pre-allocated backing buffer (memory doesn't move)
	if newBytes > len(m.backing) {
		return -1
	}
	m.Buf = m.backing[:newBytes]
	return old
}

func (m *Memory) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.backing = nil
	m.Buf = nil
	return nil
}
