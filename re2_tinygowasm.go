//go:build tinygo.wasm

package re2

import (
	"github.com/anuraaga/re2-go/cre2"
	"reflect"
	"unsafe"
)

func malloc(_ *libre2ABI, size uint32) uint32 {
	return uint32(uintptr(cre2.Malloc(int(size))))
}

func free(_ *libre2ABI, ptr uint32) {
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

func newRE(abi *libre2ABI, pattern cString, longest bool) uint32 {
	opt := cre2.NewOpt()
	defer cre2.DeleteOpt(opt)
	if longest {
		cre2.OptSetLongestMatch(opt, true)
	}
	return uint32(cre2.New(unsafe.Pointer(uintptr(pattern.ptr)), int(pattern.length), opt))
}

func reError(abi *libre2ABI, rePtr uint32) uint32 {
	return uint32(cre2.ErrorCode(unsafe.Pointer(uintptr(rePtr))))
}

func numCapturingGroups(abi *libre2ABI, rePtr uint32) int {
	return cre2.NumCapturingGroups(unsafe.Pointer(uintptr(rePtr)))
}

func release(re *Regexp) {
	cre2.Delete(unsafe.Pointer(uintptr(re.ptr)))
	cre2.Delete(unsafe.Pointer(uintptr(re.parensPtr)))
}

func match(re *Regexp, s cString, matchesPtr uint32, nMatches uint32) bool {
	return cre2.Match(unsafe.Pointer(uintptr(re.ptr)), unsafe.Pointer(uintptr(s.ptr)),
		int(s.length), 0, int(s.length), 0, unsafe.Pointer(uintptr(matchesPtr)), int(nMatches))
}

func findAndConsume(re *Regexp, csPtr pointer, matchPtr uint32, nMatch uint32) bool {
	cs := (*cString)(unsafe.Pointer(uintptr(csPtr.ptr)))

	sPtrOrig := cs.ptr

	res := cre2.FindAndConsume(unsafe.Pointer(uintptr(re.parensPtr)), unsafe.Pointer(uintptr(csPtr.ptr)), unsafe.Pointer(uintptr(matchPtr)), int(nMatch))

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
		ptr:    uint32(sh.Data),
		length: uint32(sh.Len),
	}
}

func newCStringFromBytes(_ *libre2ABI, s []byte) cString {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    uint32(sh.Data),
		length: uint32(sh.Len),
	}
}

func newCStringPtr(_ *libre2ABI, cs cString) pointer {
	return pointer{ptr: uint32(uintptr(unsafe.Pointer(&cs)))}
}

type cStringArray struct {
	// Reference to keep the array alive.
	arr []cString
	ptr uint32
}

func newCStringArray(abi *libre2ABI, n int) cStringArray {
	arr := make([]cString, n)
	ptr := uint32(uintptr(unsafe.Pointer(&arr[0])))
	return cStringArray{arr: arr, ptr: ptr}
}

type pointer struct {
	ptr uint32
}

func namedGroupsIter(_ *libre2ABI, rePtr uint32) uint32 {
	return uint32(uintptr(cre2.NamedGroupsIterNew(unsafe.Pointer(uintptr(rePtr)))))
}

func namedGroupsIterNext(_ *libre2ABI, iterPtr uint32) (string, int, bool) {
	var namePtr unsafe.Pointer
	var index int
	if !cre2.NamedGroupsIterNext(unsafe.Pointer(uintptr(iterPtr)), &namePtr, &index) {
		return "", 0, false
	}

	// C-string, find NULL
	nameLen := 0
	for *(*byte)(unsafe.Add(namePtr, nameLen)) != 0 {
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

func namedGroupsIterDelete(_ *libre2ABI, iterPtr uint32) {
	cre2.NamedGroupsIterDelete(unsafe.Pointer(uintptr(iterPtr)))
}

func globalReplace(re *Regexp, textAndTargetPtr uint32, rewritePtr uint32) ([]byte, bool) {
	if !cre2.GlobalReplace(unsafe.Pointer(uintptr(re.ptr)), unsafe.Pointer(uintptr(textAndTargetPtr)), unsafe.Pointer(uintptr(rewritePtr))) {
		// No replacements
		return nil, false
	}

	textAndTarget := (*cString)(unsafe.Pointer(uintptr(textAndTargetPtr)))
	// This was malloc'd by cre2, so free it
	defer cre2.Free(unsafe.Pointer(uintptr(textAndTarget.ptr)))

	var buf []byte
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh.Data = uintptr(textAndTarget.ptr)
	sh.Len = uintptr(textAndTarget.length)
	sh.Cap = uintptr(textAndTarget.length)

	// content of buf will be free'd, so copy it
	return append([]byte{}, buf...), true
}

func readMatch(abi *libre2ABI, cs cString, matchPtr uint32, dstCap []int) []int {
	match := (*cString)(unsafe.Pointer(uintptr(matchPtr)))
	subStrPtr := match.ptr
	if subStrPtr == 0 {
		return append(dstCap, -1, -1)
	}
	sIdx := subStrPtr - cs.ptr
	return append(dstCap, int(sIdx), int(sIdx+match.length))
}

func readMatches(abi *libre2ABI, cs cString, matchesPtr uint32, n int, deliver func([]int)) {
	var dstCap [2]int

	for i := 0; i < n; i++ {
		dst := readMatch(abi, cs, matchesPtr+uint32(8*i), dstCap[:0])
		deliver(dst)
	}
}
