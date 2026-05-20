package memory

import (
	"sync/atomic"
	"unsafe"
)

func atomicStoreSliceLen(b *[]byte, n int) {
	// The mmap keeps data and capacity stable; publish only the length change.
	lenp := (*uintptr)(unsafe.Add(unsafe.Pointer(b), unsafe.Sizeof(uintptr(0))))
	atomic.StoreUintptr(lenp, uintptr(n))
}
