package internal

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
)

const unknownCompileError = "unknown error compiling pattern"

// Negated return values of cre2_set_match_with_error, mirroring RE2::Set::ErrorKind.
const (
	setMatchNotCompiled  = 1
	setMatchOutOfMemory  = 2
	setMatchInconsistent = 3
)

// ErrSetEvaluation is wrapped by the errors FindAllWithError and
// FindAllStringWithError return. RE2::Set has no NFA fallback, so an evaluation
// that fails reports no matches even when patterns would have matched.
var ErrSetEvaluation = errors.New("re2: set evaluation did not run to completion")

func setEvaluationError(kind int) error {
	switch kind {
	case setMatchNotCompiled:
		return fmt.Errorf("%w: set is not compiled", ErrSetEvaluation)
	case setMatchOutOfMemory:
		return fmt.Errorf("%w: DFA out of memory", ErrSetEvaluation)
	case setMatchInconsistent:
		return fmt.Errorf("%w: inconsistent result", ErrSetEvaluation)
	default:
		return fmt.Errorf("%w: unknown error %d", ErrSetEvaluation, kind)
	}
}

type Set struct {
	ptr      wasmPtr
	abi      *libre2ABI
	opts     CompileOptions
	exprs    []string
	released uint32
}

func CompileSet(exprs []string, opts CompileOptions) (*Set, error) {
	abi := newABI()
	setPtr := newSet(abi, opts)
	set := &Set{
		ptr:   setPtr,
		abi:   abi,
		opts:  opts,
		exprs: exprs,
	}
	var estimatedMemorySize int
	for _, expr := range exprs {
		estimatedMemorySize += len(expr) + 2
	}

	alloc := abi.startOperation(estimatedMemorySize)
	defer abi.endOperation(alloc)

	for _, expr := range exprs {
		cs := alloc.newCString(expr)
		errMsg := setAdd(set, cs)
		if errMsg != "" {
			return nil, fmt.Errorf("%s", errMsg)
		}
	}
	if setCompile(set) == 0 {
		set.release()
		return nil, errors.New("error compiling regexp set: patterns too large")
	}
	// Use func(interface{}) form for nottinygc compatibility.
	runtime.SetFinalizer(set, func(obj interface{}) {
		if s, ok := obj.(*Set); ok {
			s.release()
		}
	})
	return set, nil
}

func (set *Set) release() {
	if !atomic.CompareAndSwapUint32(&set.released, 0, 1) {
		return
	}
	deleteSet(set.abi, set.ptr)
}

// FindAllString finds all matches of the regular expressions in the Set against the input string.
// It returns a slice of indices of the matched patterns. If n >= 0, it returns at most n matches; otherwise, it returns all of them.
//
// A failed evaluation is reported as no matches; use FindAllStringWithError to
// tell the two apart.
func (set *Set) FindAllString(s string, n int) []int {
	matches, _ := set.FindAllStringWithError(s, n)
	return matches
}

// FindAllStringWithError is like FindAllString but returns an error wrapping
// ErrSetEvaluation if the evaluation did not run to completion.
func (set *Set) FindAllStringWithError(s string, n int) ([]int, error) {
	if n == 0 {
		return nil, nil
	}
	if n < 0 {
		n = len(set.exprs)
	}
	alloc := set.abi.startOperation(len(s) + 8 + n*8)
	defer set.abi.endOperation(alloc)

	cs := alloc.newCString(s)

	var matches []int

	if err := set.findAll(&alloc, cs, n, func(match int) {
		matches = append(matches, match)
	}); err != nil {
		return nil, err
	}
	return matches, nil
}

// FindAll executes the Set against the input bytes. It returns a slice
// with the indices of the matched patterns. If n >= 0, it returns at most
// n matches; otherwise, it returns all of them.
//
// A failed evaluation is reported as no matches; use FindAllWithError to tell
// the two apart.
func (set *Set) FindAll(b []byte, n int) []int {
	matches, _ := set.FindAllWithError(b, n)
	return matches
}

// FindAllWithError is like FindAll but returns an error wrapping
// ErrSetEvaluation if the evaluation did not run to completion.
func (set *Set) FindAllWithError(b []byte, n int) ([]int, error) {
	if n == 0 {
		return nil, nil
	}
	if n < 0 {
		n = len(set.exprs)
	}
	alloc := set.abi.startOperation(len(b) + 8 + n*8)
	defer set.abi.endOperation(alloc)

	cs := alloc.newCStringFromBytes(b)

	var matches []int

	if err := set.findAll(&alloc, cs, n, func(match int) {
		matches = append(matches, match)
	}); err != nil {
		return nil, err
	}

	return matches, nil
}

func (set *Set) findAll(alloc *allocation, cs cString, n int, deliver func(match int)) error {
	matchArr := alloc.newCStringArray(n)
	defer matchArr.free()
	defer runtime.KeepAlive(matchArr)
	defer runtime.KeepAlive(set) // don't allow finalizer to run during method

	matchedCount := setMatch(set, cs, matchArr.ptr, n)
	if matchedCount < 0 {
		return setEvaluationError(-matchedCount)
	}
	matches := alloc.read(matchArr.ptr, n*4)
	for i := 0; i < matchedCount && i < n; i++ {
		deliver(int(binary.LittleEndian.Uint32(matches[i*4:])))
	}

	return nil
}
