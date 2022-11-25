//go:build !tinygo.wasm

package re2

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var errFailedWrite = errors.New("failed to read from wasm memory")
var errFailedRead = errors.New("failed to read from wasm memory")

//go:embed wasm/libcre2.so
var libre2 []byte

var wasmRT wazero.Runtime

type libre2ABIDef struct {
	cre2New                api.Function
	cre2Delete             api.Function
	cre2Match              api.Function
	cre2PartialMatch       api.Function
	cre2FindAndConsume     api.Function
	cre2NumCapturingGroups api.Function

	malloc api.Function
	free   api.Function

	memory api.Memory
}

var libre2ABI libre2ABIDef

func init() {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	mod, err := rt.InstantiateModuleFromBinary(ctx, libre2)
	if err != nil {
		panic(err)
	}

	abi := libre2ABIDef{
		cre2New:                mod.ExportedFunction("cre2_new"),
		cre2Delete:             mod.ExportedFunction("cre2_delete"),
		cre2Match:              mod.ExportedFunction("cre2_match"),
		cre2PartialMatch:       mod.ExportedFunction("cre2_partial_match_re"),
		cre2FindAndConsume:     mod.ExportedFunction("cre2_find_and_consume_re"),
		cre2NumCapturingGroups: mod.ExportedFunction("cre2_num_capturing_groups"),

		malloc: mod.ExportedFunction("malloc"),
		free:   mod.ExportedFunction("free"),

		memory: mod.Memory(),
	}

	wasmRT = rt
	libre2ABI = abi
}
func newRE(pattern string) (*Regexp, error) {
	ctx := context.Background()
	patternPtr := mustWriteString(ctx, pattern)
	defer mustFree(ctx, patternPtr)

	if !libre2ABI.memory.WriteString(ctx, uint32(patternPtr), pattern) {
		return nil, errFailedWrite
	}

	res, err := libre2ABI.cre2New.Call(ctx, patternPtr, uint64(len(pattern)), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern: %w", err)
	}
	ptr := res[0]

	patternParens := fmt.Sprintf("(%s)", pattern)
	patternParensPtr := mustWriteString(ctx, patternParens)
	defer mustFree(ctx, patternParensPtr)
	res, err = libre2ABI.cre2New.Call(ctx, patternParensPtr, uint64(len(patternParens)), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern: %w", err)
	}
	parensPtr := res[0]

	numGroups, err := libre2ABI.cre2NumCapturingGroups.Call(ctx, ptr)
	if err != nil {
		panic(err)
	}

	return &Regexp{
		ptr:       uintptr(ptr),
		parensPtr: uintptr(parensPtr),
		numGroups: int(numGroups[0]),
	}, nil
}

func matchString(re *Regexp, s string) bool {
	ctx := context.Background()
	sPtr := mustWriteString(ctx, s)
	defer mustFree(ctx, sPtr)
	matched, err := libre2ABI.cre2Match.Call(ctx, uint64(re.ptr), sPtr, uint64(len(s)), 0, uint64(len(s)), 0, 0, 0)
	if err != nil {
		panic(err)
	}

	return matched[0] == 1
}

func match(re *Regexp, s []byte) bool {
	ctx := context.Background()
	sPtr := mustWrite(ctx, s)
	defer mustFree(ctx, sPtr)

	res, err := libre2ABI.cre2Match.Call(ctx, uint64(re.ptr), sPtr, uint64(len(s)), 0, uint64(len(s)), 0, 0, 0)
	if err != nil {
		panic(err)
	}

	return res[0] == 1
}

func findAllString(re *Regexp, s string, n int) []string {
	ctx := context.Background()

	sRE2StringPtr, sPtr := mustWriteRE2String(ctx, s)
	defer mustFree(ctx, sPtr)
	defer mustFree(ctx, sRE2StringPtr)

	// TODO(anuraaga): Get more than one match per iteration.
	matchPtr := mustMalloc(ctx, 4*2)
	defer mustFree(ctx, matchPtr)

	var matches []string
	for {
		res, err := libre2ABI.cre2FindAndConsume.Call(ctx, uint64(re.parensPtr), sRE2StringPtr, matchPtr, 1)
		if err != nil {
			panic(err)
		}
		if res[0] == 0 {
			break
		}
		subStrPtr, ok := libre2ABI.memory.ReadUint32Le(ctx, uint32(matchPtr))
		if !ok {
			panic(errFailedRead)
		}
		sLen, ok := libre2ABI.memory.ReadUint32Le(ctx, uint32(matchPtr+4))
		if !ok {
			panic(errFailedRead)
		}

		sIdx := subStrPtr - uint32(sPtr)
		matches = append(matches, s[sIdx:sIdx+sLen])
		if len(matches) == n {
			break
		}
	}

	return matches
}

func findStringSubmatch(re *Regexp, s string) []string {
	ctx := context.Background()
	sPtr := mustWriteString(ctx, s)
	defer mustFree(ctx, sPtr)

	// One more for the full match which is not counted in the actual count of groups.
	numGroups := re.numGroups + 1
	matchArr := mustMalloc(ctx, 8*numGroups)

	res, err := libre2ABI.cre2Match.Call(ctx, uint64(re.ptr), sPtr, uint64(len(s)), 0, uint64(len(s)), 0, matchArr, uint64(numGroups))
	if err != nil {
		panic(err)
	}
	if res[0] == 0 {
		return nil
	}

	var matches []string
	for i := 0; i < numGroups; i++ {
		subStrPtr, ok := libre2ABI.memory.ReadUint32Le(ctx, uint32(matchArr+uint64(8*i)))
		if !ok {
			panic(errFailedRead)
		}
		sLen, ok := libre2ABI.memory.ReadUint32Le(ctx, uint32(matchArr+uint64(8*i+4)))
		if !ok {
			panic(errFailedRead)
		}

		sIdx := subStrPtr - uint32(sPtr)
		matches = append(matches, s[sIdx:sIdx+sLen])
	}

	return matches
}

func mustMalloc(ctx context.Context, size int) uint64 {
	ret, err := libre2ABI.malloc.Call(ctx, uint64(size))
	if err != nil {
		panic(err)
	}
	return ret[0]
}

func mustWrite(ctx context.Context, s []byte) uint64 {
	ptr := mustMalloc(ctx, len(s))

	if !libre2ABI.memory.Write(ctx, uint32(ptr), s) {
		panic("failed to write string to wasm memory")
	}

	return ptr
}

func mustWriteString(ctx context.Context, s string) uint64 {
	ptr := mustMalloc(ctx, len(s))

	if !libre2ABI.memory.WriteString(ctx, uint32(ptr), s) {
		panic("failed to write string to wasm memory")
	}

	return ptr
}

func mustWriteRE2String(ctx context.Context, s string) (uint64, uint64) {
	sPtr := mustWriteString(ctx, s)
	re2StringPtr := mustMalloc(ctx, 4*2)
	libre2ABI.memory.WriteUint32Le(ctx, uint32(re2StringPtr), uint32(sPtr))
	libre2ABI.memory.WriteUint32Le(ctx, uint32(re2StringPtr+4), uint32(len(s)))
	return re2StringPtr, sPtr
}

func mustFree(ctx context.Context, ptr uint64) {
	_, err := libre2ABI.free.Call(ctx, ptr)
	if err != nil {
		panic(err)
	}
}
