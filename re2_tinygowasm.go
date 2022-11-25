//go:build tinygo.wasm

package re2

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

//export cre2_new
func cre2New(patternPtr unsafe.Pointer, patternLen uint32, opts unsafe.Pointer) unsafe.Pointer

//export cre2_delete
func cre2Delete(rePtr unsafe.Pointer)

//export cre2_opt_new
func cre2OptNew() unsafe.Pointer

//export cre2_opt_delete
func cre2OptDelete(ptr unsafe.Pointer)

//export cre2_opt_set_max_mem
func cre2OptSetMaxMem(ptr unsafe.Pointer, maxMem uint64)

//export cre2_match
func cre2Match(rePtr unsafe.Pointer, textPtr unsafe.Pointer, textLen uint32, startPos uint32, endPos uint32,
	anchor uint32, matchArrPtr unsafe.Pointer, nmatch uint32) uint32

//export cre2_find_and_consume_re
func cre2FindAndConsumeRE(rePtr unsafe.Pointer, textRE2String unsafe.Pointer, match unsafe.Pointer, nMatch uint32) uint32

//export cre2_num_capturing_groups
func cre2NumCapturingGroups(rePtr unsafe.Pointer) uint32

func newRE(pattern string) (*Regexp, error) {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&pattern))
	rePtr := cre2New(unsafe.Pointer(sh.Data), uint32(sh.Len), nil)
	runtime.KeepAlive(pattern)

	numGroups := cre2NumCapturingGroups(rePtr)

	patternParens := fmt.Sprintf("(%s)", pattern)
	sh = (*reflect.StringHeader)(unsafe.Pointer(&patternParens))
	parensREPtr := cre2New(unsafe.Pointer(sh.Data), uint32(sh.Len), nil)

	// TODO(anuraaga): Propagate compilation errors from re2.
	return &Regexp{
		ptr:       uintptr(rePtr),
		parensPtr: uintptr(parensREPtr),
		numGroups: int(numGroups),
	}, nil
}

func matchString(re *Regexp, s string) bool {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))
	res := cre2Match(unsafe.Pointer(re.ptr), unsafe.Pointer(sh.Data), uint32(sh.Len), 0, uint32(sh.Len), 0, nil, 0)
	runtime.KeepAlive(s)
	return res == 1
}

func match(re *Regexp, s []byte) bool {
	res := cre2Match(unsafe.Pointer(re.ptr), unsafe.Pointer(&s[0]), uint32(len(s)), 0, uint32(len(s)), 0, nil, 0)
	return res == 1
}

type re2String struct {
	sPtr uintptr
	sLen uint32
}

func findAllString(re *Regexp, s string, n int) []string {
	if n == 0 {
		return nil
	}

	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))

	text := re2String{sh.Data, uint32(sh.Len)}

	match := re2String{}

	var matches []string

	for {
		res := cre2FindAndConsumeRE(unsafe.Pointer(re.ptr), unsafe.Pointer(&text), unsafe.Pointer(&match), 1)
		if res == 0 {
			break
		}
		sIdx := uint32(match.sPtr - text.sPtr)
		sLen := match.sLen

		matches = append(matches, s[sIdx:sIdx+sLen])

		if len(matches) == n {
			break
		}
	}

	return matches
}

func findStringSubmatch(re *Regexp, s string) []string {
	sh := (*reflect.StringHeader)(unsafe.Pointer(&s))

	// One more for the full match which is not counted in the actual count of groups.
	numGroups := re.numGroups + 1

	matchArr := make([]re2String, numGroups)

	res := cre2Match(re.ptr, unsafe.Pointer(sh.Data), uint32(sh.Len), 0, uint32(sh.Len), 0, unsafe.Pointer(&matchArr[0]), uint32(numGroups))
	if res == 0 {
		return nil
	}

	var matches []string
	for i := 0; i < numGroups; i++ {
		sLen := matchArr[i].sLen
		sIdx := uint32(uintptr(matchArr[i].sPtr) - sh.Data)
		matches = append(matches, s[sIdx:sIdx+sLen])
	}

	return matches
}
