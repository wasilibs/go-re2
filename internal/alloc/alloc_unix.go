//go:build unix

// Mostly copied from https://github.com/ncruces/go-sqlite3/blob/main/internal/util/alloc.go#L12

// MIT License
//
// Copyright (c) 2023 Nuno Cruces
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package alloc

import (
	"math"
	"syscall"

	"github.com/tetratelabs/wazero/experimental"
)

func Allocator() experimental.MemoryAllocator {
	return experimental.MemoryAllocatorFunc(mmappedAllocator)
}

func mmappedAllocator(cap, max uint64) experimental.LinearMemory {
	// Round up to the page size.
	rnd := uint64(syscall.Getpagesize() - 1)
	max = (max + rnd) &^ rnd
	cap = (cap + rnd) &^ rnd

	if max > math.MaxInt {
		// This ensures int(max) overflows to a negative value,
		// and syscall.Mmap returns EINVAL.
		max = math.MaxUint64
	}
	// Reserve max bytes of address space, to ensure we won't need to move it.
	// A protected, private, anonymous mapping should not commit memory.
	b, err := syscall.Mmap(-1, 0, int(max), syscall.PROT_NONE, syscall.MAP_PRIVATE|syscall.MAP_ANON)
	if err != nil {
		panic(err)
	}
	// Commit the initial cap bytes of memory.
	err = syscall.Mprotect(b[:cap], syscall.PROT_READ|syscall.PROT_WRITE)
	if err != nil {
		_ = syscall.Munmap(b)
		panic(err)
	}
	return &mmappedMemory{buf: b[:cap]}
}

// The slice covers the entire mmapped memory:
//   - len(buf) is the already committed memory,
//   - cap(buf) is the reserved address space.
type mmappedMemory struct {
	buf []byte
}

func (m *mmappedMemory) Reallocate(size uint64) []byte {
	if com := uint64(len(m.buf)); com < size {
		// Round up to the page size.
		rnd := uint64(syscall.Getpagesize() - 1)
		new := (size + rnd) &^ rnd

		// Commit additional memory up to new bytes.
		err := syscall.Mprotect(m.buf[com:new], syscall.PROT_READ|syscall.PROT_WRITE)
		if err != nil {
			panic(err)
		}

		// Update committed memory.
		m.buf = m.buf[:new]
	}
	return m.buf[:size]
}

func (m *mmappedMemory) Free() {
	err := syscall.Munmap(m.buf[:cap(m.buf)])
	if err != nil {
		panic(err)
	}
	m.buf = nil
}
