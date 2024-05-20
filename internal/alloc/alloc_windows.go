//go:build windows

package alloc

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"

	"github.com/tetratelabs/wazero/experimental"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procVirtualAlloc = kernel32.NewProc("VirtualAlloc")
	procVirtualFree  = kernel32.NewProc("VirtualFree")
)

const (
	windows_MEM_COMMIT     uintptr = 0x00001000
	windows_MEM_RESERVE    uintptr = 0x00002000
	windows_MEM_RELEASE    uintptr = 0x00008000
	windows_PAGE_READWRITE uintptr = 0x00000004

	// https://cs.opensource.google/go/x/sys/+/refs/tags/v0.20.0:windows/syscall_windows.go;l=131
	windows_PAGE_SIZE uint64 = 4096
)

func Allocator() experimental.MemoryAllocator {
	return experimental.MemoryAllocatorFunc(virtualAllocator)
}

func virtualAllocator(cap, max uint64) experimental.LinearMemory {
	// Round up to the page size.
	rnd := windows_PAGE_SIZE - 1
	max = (max + rnd) &^ rnd
	cap = (cap + rnd) &^ rnd

	if max > math.MaxInt {
		// This ensures uintptr(max) overflows to a large value,
		// and windows.VirtualAlloc returns an error.
		max = math.MaxUint64
	}

	// Reserve, but don't commit, max bytes of address space, to ensure we won't need to move it.
	// This does not commit memory.
	r, _, err := procVirtualAlloc.Call(0, uintptr(max), windows_MEM_RESERVE, windows_PAGE_READWRITE)
	if r == 0 {
		panic(fmt.Errorf("alloc_windows: failed to reserve memory: %w", err))
	}

	// Commit the initial cap bytes of memory.
	r, _, err = procVirtualAlloc.Call(r, uintptr(cap), windows_MEM_COMMIT, windows_PAGE_READWRITE)
	if r == 0 {
		_, _, _ = procVirtualFree.Call(r, 0, windows_MEM_RELEASE)
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
		rnd := windows_PAGE_SIZE - 1
		new := (size + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		r, _, err := procVirtualAlloc.Call(m.addr, uintptr(new), windows_MEM_COMMIT, windows_PAGE_READWRITE)
		if r == 0 {
			panic(fmt.Errorf("alloc_windows: failed to commit memory: %w", err))
		}

		// Limit returned capacity because bytes beyond
		// len(m.buf) have not yet been committed.
		m.buf = m.buf[:new]
	}
	return m.buf[:size:len(m.buf)]
}

func (m *virtualMemory) Free() {
	r, _, err := procVirtualFree.Call(m.addr, 0, windows_MEM_RELEASE)
	if r == 0 {
		panic(fmt.Errorf("alloc_windows: failed to release memory: %w", err))
	}
	m.addr = 0
	m.buf = nil
}
