//go:build tinygo.wasm

package re2

import (
	"reflect"
	"unsafe"
)

//export cre2_new
func cre2New(patternPtr uint32, patternLen uint32, opts unsafe.Pointer) uint32

//export cre2_delete
func cre2Delete(rePtr unsafe.Pointer)

//export cre2_opt_new
func cre2OptNew() unsafe.Pointer

//export cre2_opt_delete
func cre2OptDelete(ptr unsafe.Pointer)

//export cre2_opt_set_max_mem
func cre2OptSetMaxMem(ptr unsafe.Pointer, maxMem uint64)

//export cre2_opt_set_longest_match
func cre2OptSetLongestMatch(ptr unsafe.Pointer, flag bool)

//export cre2_opt_set_posix_syntax
func cre2OptSetPosixSyntax(ptr unsafe.Pointer, flag bool)

//export cre2_match
func cre2Match(rePtr uint32, textPtr uint32, textLen uint32, startPos uint32, endPos uint32,
	anchor uint32, matchArrPtr uint32, nmatch uint32) bool

//export cre2_find_and_consume_re
func cre2FindAndConsumeRE(rePtr uint32, textRE2String uint32, match uint32, nMatch uint32) bool

//export cre2_num_capturing_groups
func cre2NumCapturingGroups(rePtr uint32) uint32

//export cre2_error_code
func cre2ErrorCode(rePtr uint32) uint32

//export cre2_named_groups_iter_new
func cre2NamedGroupsIterNew(rePtr uint32) uint32

//export cre2_named_groups_iter_next
func cre2NamedGroupsIterNext(iterPtr uint32, namePtrPtr *uint32, indexPtr *uint32) uint32

//export cre2_named_groups_iter_delete
func cre2NamedGroupsIterDelete(iterPtr uint32)

//export cre2_global_replace_re
func cre2GlobalReplaceRE(rePtr uint32, textAndTargetPtr uint32, rewritePtr uint32) int32

//export malloc
func malloc(size uint32) uint32

//export free
func free(ptr uint32)

func newRE(pattern cString, longest bool) uint32 {
	opts := cre2OptNew()
	defer cre2OptDelete(opts)
	if longest {
		cre2OptSetLongestMatch(opts, true)
	}
	return cre2New(pattern.ptr, pattern.length, opts)
}

func reError(rePtr uint32) uint32 {
	return cre2ErrorCode(rePtr)
}

func numCapturingGroups(rePtr uint32) int {
	return int(cre2NumCapturingGroups(rePtr))
}

func match(rePtr uint32, s cString, matchesPtr uint32, nMatches uint32) bool {
	return cre2Match(rePtr, s.ptr, s.length, 0, s.length, 0, matchesPtr, nMatches)
}

func findAndConsume(re *Regexp, csPtr pointer, matchPtr uint32, nMatch uint32) bool {
	cs := (*cString)(unsafe.Pointer(uintptr(csPtr.ptr)))

	sPtrOrig := cs.ptr

	res := cre2FindAndConsumeRE(re.parensPtr, csPtr.ptr, matchPtr, nMatch)

	// If the regex matched an empty string, consumption will not advance the input, so we must do it ourselves.
	if cs.ptr == sPtrOrig && cs.length > 0 {
		cs.ptr += 1
		cs.length -= 1
	}

	return res
}

type cString struct {
	ptr    uint32
	length uint32
}

func (s cString) release() {
	// no-op
}

func newCString(s string) cString {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    uint32(sh.Data),
		length: uint32(sh.Len),
	}
}

func newCStringFromBytes(s []byte) cString {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    uint32(sh.Data),
		length: uint32(sh.Len),
	}
}

func newCStringPtr(cs cString) pointer {
	return pointer{ptr: uint32(uintptr(unsafe.Pointer(&cs)))}
}

type pointer struct {
	ptr uint32
}

func (p pointer) release() {
}

func namedGroupsIter(rePtr uint32) uint32 {
	return cre2NamedGroupsIterNew(rePtr)
}

func namedGroupsIterNext(iterPtr uint32) (string, int, bool) {
	var namePtr uint32
	var index uint32
	if cre2NamedGroupsIterNext(iterPtr, &namePtr, &index) == 0 {
		return "", 0, false
	}

	// C-string, find NULL
	nameLen := 0
	for *(*byte)(unsafe.Pointer(uintptr(namePtr + uint32(nameLen)))) != 0 {
		nameLen++
	}

	// Convert to Go string. The results are aliases into strings stored in the regexp,
	// so it is safe to alias them without copying.
	name := *(*string)(unsafe.Pointer(&reflect.StringHeader{
		Data: uintptr(namePtr),
		Len:  uintptr(nameLen),
	}))

	return name, int(index), true
}

func namedGroupsIterDelete(iterPtr uint32) {
	cre2NamedGroupsIterDelete(iterPtr)
}

func globalReplace(rePtr uint32, textAndTargetPtr uint32, rewritePtr uint32) ([]byte, bool) {
	res := cre2GlobalReplaceRE(rePtr, textAndTargetPtr, rewritePtr)
	if res == -1 {
		panic("out of memory")
	}
	if res == 0 {
		// No replacements
		return nil, false
	}

	textAndTarget := (*cString)(unsafe.Pointer(uintptr(textAndTargetPtr)))
	// This was malloc'd by cre2, so free it
	defer free(textAndTarget.ptr)

	var buf []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh.Data = uintptr(textAndTarget.ptr)
	sh.Len = uintptr(textAndTarget.length)
	sh.Cap = uintptr(textAndTarget.length)

	// content of buf will be free'd, so copy it
	return append([]byte{}, buf...), true
}

func readMatch(cs cString, matchPtr uint32, dstCap []int) []int {
	match := (*cString)(unsafe.Pointer(uintptr(matchPtr)))
	subStrPtr := match.ptr
	if subStrPtr == 0 && cs.ptr != 0 {
		return append(dstCap, -1, -1)
	}
	sIdx := subStrPtr - cs.ptr
	return append(dstCap, int(sIdx), int(sIdx+match.length))
}
