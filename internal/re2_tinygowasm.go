//go:build tinygo.wasm || re2_cgo

package internal

import (
	"reflect"
	"unsafe"

	"github.com/wasilibs/go-re2/internal/cre2"
)

type wasmPtr unsafe.Pointer

var nilWasmPtr = wasmPtr(nil)

type libre2ABI struct{}

func newABI() *libre2ABI {
	return &libre2ABI{}
}

func (abi *libre2ABI) startOperation(memorySize int) {
}

func (abi *libre2ABI) endOperation() {
}

func newRE(abi *libre2ABI, pattern cString, opts CompileOptions) wasmPtr {
	opt := cre2.NewOpt()
	defer cre2.DeleteOpt(opt)
	cre2.OptSetLogErrors(opt, false)
	if opts.Longest {
		cre2.OptSetLongestMatch(opt, true)
	}
	if opts.Posix {
		cre2.OptSetPosixSyntax(opt, true)
	}
	if opts.CaseInsensitive {
		cre2.OptSetCaseSensitive(opt, false)
	}
	if opts.Latin1 {
		cre2.OptSetLatin1Encoding(opt)
	}
	return wasmPtr(cre2.New(pattern.ptr, pattern.length, opt))
}

func reError(abi *libre2ABI, rePtr wasmPtr) (int, string) {
	code := cre2.ErrorCode(unsafe.Pointer(rePtr))
	if code == 0 {
		return 0, ""
	}

	arg := cString{}
	cre2.ErrorArg(unsafe.Pointer(rePtr), unsafe.Pointer(&arg))

	return code, cre2.CopyCStringN(arg.ptr, arg.length)
}

func numCapturingGroups(abi *libre2ABI, rePtr wasmPtr) int {
	return cre2.NumCapturingGroups(unsafe.Pointer(rePtr))
}

func deleteRE(_ *libre2ABI, rePtr wasmPtr) {
	cre2.Delete(unsafe.Pointer(rePtr))
}

func release(re *Regexp) {
	deleteRE(re.abi, re.ptr)
}

func match(re *Regexp, s cString, matchesPtr wasmPtr, nMatches uint32) bool {
	return cre2.Match(unsafe.Pointer(re.ptr), s.ptr,
		s.length, 0, s.length, 0, unsafe.Pointer(matchesPtr), int(nMatches))
}

func matchFrom(re *Regexp, s cString, startPos int, matchesPtr wasmPtr, nMatches uint32) bool {
	return cre2.Match(unsafe.Pointer(re.ptr), s.ptr,
		s.length, startPos, s.length, 0, unsafe.Pointer(matchesPtr), int(nMatches))
}

type cString struct {
	ptr    unsafe.Pointer
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
		ptr:    unsafe.Pointer(sh.Data),
		length: int(sh.Len),
	}
}

func newCStringFromBytes(_ *libre2ABI, s []byte) cString {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	return cString{
		ptr:    unsafe.Pointer(sh.Data),
		length: int(sh.Len),
	}
}

type pointer struct {
	ptr wasmPtr
}

func newCStringPtrFromBytes(_ *libre2ABI, s []byte) pointer {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&s))
	csPtr := cre2.Malloc(int(unsafe.Sizeof(cString{})))
	cs := (*cString)(csPtr)
	cs.ptr = unsafe.Pointer(sh.Data)
	cs.length = int(sh.Len)
	return pointer{ptr: wasmPtr(csPtr)}
}

func newCStringPtr(_ *libre2ABI, s string) pointer {
	if len(s) == 0 {
		// TinyGo uses a null pointer to represent an empty string, but this
		// prevents us from distinguishing a match on the empty string vs no
		// match for subexpressions. So we replace with an empty-length slice
		// to a string that isn't null.
		s = "a"[0:0]
	}
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	csPtr := cre2.Malloc(int(unsafe.Sizeof(cString{})))
	cs := (*cString)(csPtr)
	cs.ptr = unsafe.Pointer(sh.Data)
	cs.length = int(sh.Len)
	return pointer{ptr: wasmPtr(csPtr)}
}

func (p pointer) free() {
	cre2.Free(unsafe.Pointer(p.ptr))
}

type cStringArray struct {
	ptr wasmPtr
}

func newCStringArray(abi *libre2ABI, n int) cStringArray {
	sz := int(unsafe.Sizeof(cString{})) * n
	ptr := cre2.Malloc(sz)
	for i := 0; i < sz; i++ {
		*(*byte)(unsafe.Add(ptr, i)) = 0
	}

	return cStringArray{ptr: wasmPtr(ptr)}
}

func (a cStringArray) free() {
	cre2.Free(unsafe.Pointer(a.ptr))
}

func namedGroupsIter(_ *libre2ABI, rePtr wasmPtr) wasmPtr {
	return wasmPtr(cre2.NamedGroupsIterNew(unsafe.Pointer(rePtr)))
}

func namedGroupsIterNext(_ *libre2ABI, iterPtr wasmPtr) (string, int, bool) {
	var namePtr unsafe.Pointer
	var index int
	if !cre2.NamedGroupsIterNext(unsafe.Pointer(iterPtr), &namePtr, &index) {
		return "", 0, false
	}

	name := cre2.CopyCString(namePtr)
	return name, index, true
}

func namedGroupsIterDelete(_ *libre2ABI, iterPtr wasmPtr) {
	cre2.NamedGroupsIterDelete(unsafe.Pointer(iterPtr))
}

func globalReplace(re *Regexp, textAndTargetPtr wasmPtr, rewritePtr wasmPtr) ([]byte, bool) {
	// cre2 will allocate even when no matches, make sure to free before
	// checking result.
	res := cre2.GlobalReplace(unsafe.Pointer(re.ptr), unsafe.Pointer(textAndTargetPtr), unsafe.Pointer(rewritePtr))

	textAndTarget := (*cString)(textAndTargetPtr)
	// This was malloc'd by cre2, so free it
	defer cre2.Free(textAndTarget.ptr)

	if !res {
		// No replacements
		return nil, false
	}

	// content of buf will be free'd, so copy it
	return cre2.CopyCBytes(textAndTarget.ptr, textAndTarget.length), true
}

func readMatch(abi *libre2ABI, cs cString, matchPtr wasmPtr, dstCap []int) []int {
	match := (*cString)(matchPtr)
	subStrPtr := match.ptr
	if subStrPtr == nil {
		return append(dstCap, -1, -1)
	}
	sIdx := uintptr(subStrPtr) - uintptr(cs.ptr)
	return append(dstCap, int(sIdx), int(sIdx+uintptr(match.length)))
}

func readMatches(abi *libre2ABI, cs cString, matchesPtr wasmPtr, n int, deliver func([]int)) {
	var dstCap [2]int

	for i := 0; i < n; i++ {
		dst := readMatch(abi, cs, wasmPtr(unsafe.Add(unsafe.Pointer(matchesPtr), unsafe.Sizeof(cString{})*uintptr(i))), dstCap[:0])
		deliver(dst)
	}
}
