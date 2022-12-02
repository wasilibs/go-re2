//go:build tinygo.wasm || re2_cgo

package re2

import (
	"reflect"
	"unsafe"

	"github.com/anuraaga/re2-go/cre2"
)

func malloc(_ *libre2ABI, size uint32) uintptr {
	return uintptr(cre2.Malloc(int(size)))
}

func free(_ *libre2ABI, ptr uintptr) {
	cre2.Free(unsafe.Pointer(uintptr(ptr)))
}

type libre2ABI struct{}

func newABI() *libre2ABI {
	return &libre2ABI{}
}

func (abi *libre2ABI) startOperation(memorySize int) {
}

func (abi *libre2ABI) endOperation() {
}

func newRE(abi *libre2ABI, pattern cString, longest bool) uintptr {
	opt := cre2.NewOpt()
	defer cre2.DeleteOpt(opt)
	if longest {
		cre2.OptSetLongestMatch(opt, true)
	}
	return uintptr(cre2.New(unsafe.Pointer(uintptr(pattern.ptr)), int(pattern.length), opt))
}

func reError(abi *libre2ABI, rePtr uintptr) int {
	return cre2.ErrorCode(unsafe.Pointer(rePtr))
}

func numCapturingGroups(abi *libre2ABI, rePtr uintptr) int {
	return cre2.NumCapturingGroups(unsafe.Pointer(rePtr))
}

func deleteRE(_ *libre2ABI, rePtr uintptr) {
	cre2.Delete(unsafe.Pointer(rePtr))
}

func release(re *Regexp) {
	deleteRE(re.abi, re.ptr)
	deleteRE(re.abi, re.parensPtr)
}

func match(re *Regexp, s cString, matchesPtr uintptr, nMatches uint32) bool {
	return cre2.Match(unsafe.Pointer(re.ptr), unsafe.Pointer(s.ptr),
		int(s.length), 0, int(s.length), 0, unsafe.Pointer(matchesPtr), int(nMatches))
}

func findAndConsume(re *Regexp, csPtr pointer, matchPtr uintptr, nMatch uint32) bool {
	cs := (*cString)(unsafe.Pointer(csPtr.ptr))

	sPtrOrig := cs.ptr

	res := cre2.FindAndConsume(unsafe.Pointer(re.parensPtr), unsafe.Pointer(csPtr.ptr), unsafe.Pointer(matchPtr), int(nMatch))

	// If the regex matched an empty string, consumption will not advance the input, so we must do it ourselves.
	if cs.ptr == sPtrOrig && cs.length > 0 {
		cs.ptr += 1
		cs.length -= 1
	}

	return res
}

type cString struct {
	ptr    uintptr
	length int
}

func newCString(_ *libre2ABI, s string) cString {
	if len(s) == 0 {
		// TinyGo uses a null pointer to represent an empty string, but this
		// prevents us from distinguishing a match on the empty string vs no
		// match for subexpressions. So we replace with an empty-length slice
		// to a string that isn't null.
		s = "a"[0:0]
	}
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    sh.Data,
		length: int(sh.Len),
	}
}

func newCStringFromBytes(_ *libre2ABI, s []byte) cString {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    sh.Data,
		length: int(sh.Len),
	}
}

func newCStringPtr(_ *libre2ABI, cs cString) pointer {
	return pointer{ptr: uintptr(unsafe.Pointer(&cs))}
}

type cStringArray struct {
	// Reference to keep the array alive.
	arr []cString
	ptr uintptr
}

func newCStringArray(abi *libre2ABI, n int) cStringArray {
	arr := make([]cString, n)
	ptr := uintptr(unsafe.Pointer(&arr[0]))
	return cStringArray{arr: arr, ptr: ptr}
}

type pointer struct {
	ptr uintptr
}

func namedGroupsIter(_ *libre2ABI, rePtr uintptr) uintptr {
	return uintptr(cre2.NamedGroupsIterNew(unsafe.Pointer(rePtr)))
}

func namedGroupsIterNext(_ *libre2ABI, iterPtr uintptr) (string, int, bool) {
	var namePtr unsafe.Pointer
	var index int
	if !cre2.NamedGroupsIterNext(unsafe.Pointer(iterPtr), &namePtr, &index) {
		return "", 0, false
	}

	name := cre2.CopyCString(namePtr)
	return name, index, true
}

func namedGroupsIterDelete(_ *libre2ABI, iterPtr uintptr) {
	cre2.NamedGroupsIterDelete(unsafe.Pointer(uintptr(iterPtr)))
}

func globalReplace(re *Regexp, textAndTargetPtr uintptr, rewritePtr uintptr) ([]byte, bool) {
	if !cre2.GlobalReplace(unsafe.Pointer(re.ptr), unsafe.Pointer(textAndTargetPtr), unsafe.Pointer(rewritePtr)) {
		// No replacements
		return nil, false
	}

	textAndTarget := (*cString)(unsafe.Pointer(textAndTargetPtr))
	// This was malloc'd by cre2, so free it
	defer cre2.Free(unsafe.Pointer(textAndTarget.ptr))

	// content of buf will be free'd, so copy it
	return cre2.ReadCBytes(unsafe.Pointer(textAndTarget.ptr), textAndTarget.length), true
}

func readMatch(abi *libre2ABI, cs cString, matchPtr uintptr, dstCap []int) []int {
	match := (*cString)(unsafe.Pointer(matchPtr))
	subStrPtr := match.ptr
	if subStrPtr == 0 {
		return append(dstCap, -1, -1)
	}
	sIdx := subStrPtr - cs.ptr
	return append(dstCap, int(sIdx), int(sIdx+uintptr(match.length)))
}

func readMatches(abi *libre2ABI, cs cString, matchesPtr uintptr, n int, deliver func([]int)) {
	var dstCap [2]int

	for i := 0; i < n; i++ {
		dst := readMatch(abi, cs, matchesPtr+unsafe.Sizeof(cString{})*uintptr(i), dstCap[:0])
		deliver(dst)
	}
}
