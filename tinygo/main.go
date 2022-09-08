package main

import (
	"reflect"
	"regexp"
	"unsafe"
)

var allocs = map[uint32]*regexp.Regexp{}

// main is required for TinyGo to compile to Wasm.
func main() {}

//export cre2_new
func cre2_new(sPtr uint32, sLen uint32, options uint32) uint32 {
	s := ptrToString(sPtr, sLen)
	re, err := regexp.Compile(s)
	if err != nil {
		panic(err)
	}
	rePtr := uint32(uintptr(unsafe.Pointer(re)))
	allocs[rePtr] = re
	return rePtr
}

//export cre2_delete
func cre2_delete(rePtr uint32) {
	delete(allocs, rePtr)
}

//export cre2_match
func cre2_match(rePtr uint32, sPtr uint32, sLen uint32, start uint32, end uint32, anchor uint32, match uint32, nmatch uint32) uint32 {
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
