package internal

import (
	"encoding/binary"
	"errors"
	"runtime"
	"sync/atomic"
)

const errorBufferLength = 64

type Set struct {
	ptr      wasmPtr
	abi      *libre2ABI
	opts     CompileOptions
	released uint32
}

func CompileSet(exprs []string, opts CompileOptions) (*Set, error) {
	abi := newABI()
	setPtr := newSet(abi, opts)
	set := &Set{
		ptr:  setPtr,
		abi:  abi,
		opts: opts,
	}
	var errorBufferLen int
	var estimatedMemorySize int
	for _, expr := range exprs {
		errorBufferLen = max(errorBufferLen, len(expr))
		estimatedMemorySize += len(expr) + 2 + 8
	}

	alloc := abi.startOperation(estimatedMemorySize + errorBufferLen + errorBufferLength)
	defer abi.endOperation(alloc)

	errorBuffer := alloc.newCStringArray((errorBufferLen + errorBufferLength) / 16)
	defer errorBuffer.free()

	for _, expr := range exprs {
		errorsLen := len(expr) + errorBufferLength
		cs := alloc.newCString(expr)
		res := setAdd(set, cs, errorBuffer.ptr, errorsLen)
		if res == -1 {
			return nil, errors.New(readErr(errorBuffer.ptr, errorsLen))
		}
	}
	setCompile(set)
	// Use func(interface{}) form for nottinygc compatibility.
	runtime.SetFinalizer(set, func(obj interface{}) {
		obj.(*Set).release()
	})
	return set, nil
}

func NewSet(opts CompileOptions) (*Set, error) {
	abi := newABI()
	setPtr := newSet(abi, opts)
	set := &Set{
		ptr:  setPtr,
		abi:  abi,
		opts: opts,
	}
	// Use func(interface{}) form for nottinygc compatibility.
	runtime.SetFinalizer(set, func(obj interface{}) {
		obj.(*Set).release()
	})
	return set, nil
}

func (set *Set) release() {
	if !atomic.CompareAndSwapUint32(&set.released, 0, 1) {
		return
	}
	deleteSet(set.abi, set.ptr)
}

func (set *Set) Compile(expr string) {
	setCompile(set)
	runtime.KeepAlive(set)
}

//func (set *Set) SetAdd(expr string) error {
//	alloc := set.abi.startOperation(len(expr) + 2 + 8)
//	defer set.abi.endOperation(alloc)
//
//	cs := alloc.newCString(expr)
//	errorBuffer := alloc.newCStringArray(1)
//	defer errorBuffer.free()
//
//	if res := setAdd(set, cs, errorBuffer.ptr, errorBufferLength); res == -1 {
//		return errors.New(readErrorMessage(&alloc, errorBuffer.ptr, errorBufferLength))
//	}
//
//	runtime.KeepAlive(set)
//	return nil
//}

func (set *Set) SetAddSimple(expr string) {
	alloc := set.abi.startOperation(len(expr) + 2 + 8)
	defer set.abi.endOperation(alloc)

	cs := alloc.newCString(expr)
	setAddSimple(set, cs)

	runtime.KeepAlive(set)
}

func (set *Set) Match(b []byte, n int) []int {
	alloc := set.abi.startOperation(len(b) + 8 + n*8)
	defer set.abi.endOperation(alloc)

	matchArr := alloc.newCStringArray(n)
	defer matchArr.free()

	cs := alloc.newCStringFromBytes(b)
	matchedCount := setMatch(set, cs, matchArr.ptr, n)
	matchedIDs := make([]int, min(matchedCount, n))
	matches := alloc.read(matchArr.ptr, n*4)
	for i := 0; i < len(matchedIDs); i++ {
		matchedIDs[i] = int(binary.LittleEndian.Uint32(matches[i*4:]))
	}

	runtime.KeepAlive(b)
	runtime.KeepAlive(matchArr)
	runtime.KeepAlive(set) // don't allow finalizer to run during method
	return matchedIDs
}
