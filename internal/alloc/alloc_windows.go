//go:build windows

package alloc

import (
	"fmt"
	"math"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental"
	"golang.org/x/sys/windows"
)

func Allocator() experimental.MemoryAllocator {
	return experimental.MemoryAllocatorFunc(virtualAllocator)
}

func virtualAllocator(cap, max uint64) experimental.LinearMemory {
	// Round up to the page size.
	rnd := uint64(windows.Getpagesize() - 1)
	max = (max + rnd) &^ rnd
	cap = (cap + rnd) &^ rnd

	if max > math.MaxInt {
		// This ensures uintptr(max) overflows to a large value,
		// and windows.VirtualAlloc returns an error.
		max = math.MaxUint64
	}

	// Reserve, but don't commit, max bytes of address space, to ensure we won't need to move it.
	// This does not commit memory.
	r, err := windows.VirtualAlloc(0, uintptr(max), windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if err != nil {
		panic(fmt.Errorf("alloc_windows: failed to reserve memory: %w", err))
	}

	// Commit the initial cap bytes of memory.
	r, err = windows.VirtualAlloc(r, uintptr(cap), windows.MEM_COMMIT, windows.PAGE_READWRITE)
	if err != nil {
		_ = windows.VirtualFree(r, 0, windows.MEM_RELEASE)
		panic(fmt.Errorf("alloc_windows: failed to commit initial memory: %w", err))
	}

	buf := unsafe.Slice((*byte)(unsafe.Pointer(r)), int(max))
	return &virtualMemory{buf: buf[:cap], addr: r}
}

// The slice covers the entire allocated memory:
//   - len(buf) is the already committed memory,
//   - cap(buf) is the reserved address space.
type virtualMemory struct {
	buf  []byte
	addr uintptr
}

func (m *virtualMemory) Reallocate(size uint64) []byte {
	com := uint64(len(m.buf))
	res := uint64(cap(m.buf))
	if com < size && size < res {
		// Round up to the page size.
		rnd := uint64(windows.Getpagesize() - 1)
		new := (size + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		_, err := windows.VirtualAlloc(m.addr, uintptr(new), windows.MEM_COMMIT, windows.PAGE_READWRITE)
		if err != nil {
			panic(fmt.Errorf("alloc_windows: failed to commit memory: %w", err))
		}

		// Limit returned capacity because bytes beyond
		// len(m.buf) have not yet been committed.
		m.buf = m.buf[:new]
	}
	return m.buf[:size:len(m.buf)]
}

func (m *virtualMemory) Free() {
	err := windows.VirtualFree(m.addr, 0, windows.MEM_RELEASE)
	if err != nil {
		panic(fmt.Errorf("alloc_windows: failed to release memory: %w", err))
	}
	m.addr = 0
	m.buf = nil
}
