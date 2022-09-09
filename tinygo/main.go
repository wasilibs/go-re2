package main

import (
	"reflect"
	"regexp"
	"unsafe"
)

var bufs = make(map[unsafe.Pointer][]byte)

//export tg_malloc
func libc_malloc(size uintptr) unsafe.Pointer {
	buf := make([]byte, size)
	ptr := unsafe.Pointer(&buf[0])
	bufs[ptr] = buf
	return ptr
}

//export tg_free
func libc_free(ptr unsafe.Pointer) {
	delete(bufs, ptr)
}

var allocs = map[unsafe.Pointer]*regexp.Regexp{}

// main is required for TinyGo to compile to Wasm.
func main() {}

//export tg_new
func tg_new(sPtr uint32, sLen uint32, options uint32) unsafe.Pointer {
	s := ptrToString(sPtr, sLen)
	re, err := regexp.Compile(s)
	if err != nil {
		panic(err)
	}
	rePtr := unsafe.Pointer(re)
	allocs[rePtr] = re
	return rePtr
}

//export tg_delete
func tg_delete(rePtr unsafe.Pointer) {
	delete(allocs, rePtr)
}

//export tg_match
func tg_match(rePtr unsafe.Pointer, sPtr uint32, sLen uint32, start uint32, end uint32, anchor uint32, match uint32, nmatch uint32) int32 {
	re := allocs[rePtr]
	s := ptrToString(sPtr, sLen)
	if re.MatchString(s) {
		return 1
	}
	return 0
}

// ptrToString returns a string from WebAssembly compatible numeric types
// representing its pointer and length.
func ptrToString(ptr uint32, size uint32) string {
	// Get a slice view of the underlying bytes in the stream. We use SliceHeader, not StringHeader
	// as it allows us to fix the capacity to what was allocated.
	return *(*string)(unsafe.Pointer(&reflect.SliceHeader{
		Data: uintptr(ptr),
		Len:  uintptr(size), // Tinygo requires these as uintptrs even if they are int fields.
		Cap:  uintptr(size), // ^^ See https://github.com/tinygo-org/tinygo/issues/1284
	}))
}
