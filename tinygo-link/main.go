package main

import "unsafe"

//export cre2_new
func re2_cre2_new(sPtr uint32, sLen uint32, options uint32) uint32

//export cre2_delete
func re2_cre2_delete(rePtr uint32)

//export cre2_match
func re2_cre2_match(rePtr uint32, sPtr uint32, sLen uint32, start uint32, end uint32, anchor uint32, match uint32, nmatch uint32) uint32

//export malloc
func libc_malloc(size uintptr) unsafe.Pointer

//export free
func libc_free(ptr unsafe.Pointer)

//export calloc
func libc_calloc(nmemb, size uintptr) unsafe.Pointer

//export __libc_calloc
func __libc_calloc(nmemb, size uintptr) unsafe.Pointer {
	return libc_calloc(nmemb, size)
}

//export __libc_malloc
func __libc_malloc(size uintptr) unsafe.Pointer {
	return libc_malloc(size)
}

//export __libc_free
func __libc_free(ptr unsafe.Pointer) {
	libc_free(ptr)
}

//export posix_memalign
func posix_memalign(memptr *unsafe.Pointer, alignment, size uintptr) int {
	// Ignore alignment for now
	*memptr = libc_malloc(size)
	return 0
}

// main is required for TinyGo to compile to Wasm.
func main() {}

//export tg_cre2_new
func cre2_new(sPtr uint32, sLen uint32, options uint32) uint32 {
	return re2_cre2_new(sPtr, sLen, options)
}

//export tg_cre2_delete
func cre2_delete(rePtr uint32) {
	re2_cre2_delete(rePtr)
}

//export tg_cre2_match
func cre2_match(rePtr uint32, sPtr uint32, sLen uint32, start uint32, end uint32, anchor uint32, match uint32, nmatch uint32) uint32 {
	return re2_cre2_match(rePtr, sPtr, sLen, start, end, anchor, match, nmatch)
}
