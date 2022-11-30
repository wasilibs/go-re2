//go:build !tinygo.wasm

package re2

import (
	"context"
	_ "embed"
	"errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"strings"
)

var errFailedWrite = errors.New("failed to read from wasm memory")
var errFailedRead = errors.New("failed to read from wasm memory")

//go:embed wasm/libcre2.so
var libre2 []byte

var wasmRT wazero.Runtime

type libre2ABIDef struct {
	cre2New                   api.Function
	cre2Delete                api.Function
	cre2Match                 api.Function
	cre2PartialMatch          api.Function
	cre2FindAndConsume        api.Function
	cre2NumCapturingGroups    api.Function
	cre2ErrorCode             api.Function
	cre2NamedGroupsIterNew    api.Function
	cre2NamedGroupsIterNext   api.Function
	cre2NamedGroupsIterDelete api.Function
	cre2GlobalReplace         api.Function
	cre2OptNew                api.Function
	cre2OptDelete             api.Function
	cre2OptSetLongestMatch    api.Function
	cre2OptSetPosixSyntax     api.Function

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
		cre2New:                   mod.ExportedFunction("cre2_new"),
		cre2Delete:                mod.ExportedFunction("cre2_delete"),
		cre2Match:                 mod.ExportedFunction("cre2_match"),
		cre2PartialMatch:          mod.ExportedFunction("cre2_partial_match_re"),
		cre2FindAndConsume:        mod.ExportedFunction("cre2_find_and_consume_re"),
		cre2NumCapturingGroups:    mod.ExportedFunction("cre2_num_capturing_groups"),
		cre2ErrorCode:             mod.ExportedFunction("cre2_error_code"),
		cre2NamedGroupsIterNew:    mod.ExportedFunction("cre2_named_groups_iter_new"),
		cre2NamedGroupsIterNext:   mod.ExportedFunction("cre2_named_groups_iter_next"),
		cre2NamedGroupsIterDelete: mod.ExportedFunction("cre2_named_groups_iter_delete"),
		cre2GlobalReplace:         mod.ExportedFunction("cre2_global_replace_re"),
		cre2OptNew:                mod.ExportedFunction("cre2_opt_new"),
		cre2OptDelete:             mod.ExportedFunction("cre2_opt_delete"),
		cre2OptSetLongestMatch:    mod.ExportedFunction("cre2_opt_set_longest_match"),
		cre2OptSetPosixSyntax:     mod.ExportedFunction("cre2_opt_set_posix_syntax"),

		malloc: mod.ExportedFunction("malloc"),
		free:   mod.ExportedFunction("free"),

		memory: mod.Memory(),
	}

	wasmRT = rt
	libre2ABI = abi
}

func newRE(pattern cString, longest bool) uint32 {
	ctx := context.Background()
	res, err := libre2ABI.cre2OptNew.Call(ctx)
	if err != nil {
		panic(err)
	}
	optPtr := uint32(res[0])
	defer func() {
		if _, err := libre2ABI.cre2OptDelete.Call(ctx, uint64(optPtr)); err != nil {
			panic(err)
		}
	}()
	if longest {
		_, err = libre2ABI.cre2OptSetLongestMatch.Call(ctx, uint64(optPtr), 1)
		if err != nil {
			panic(err)
		}
	}
	res, err = libre2ABI.cre2New.Call(ctx, uint64(pattern.ptr), uint64(pattern.length), uint64(optPtr))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func reError(rePtr uint32) uint32 {
	ctx := context.Background()
	res, err := libre2ABI.cre2ErrorCode.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func numCapturingGroups(rePtr uint32) int {
	ctx := context.Background()
	res, err := libre2ABI.cre2NumCapturingGroups.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	return int(res[0])
}

func match(rePtr uint32, s cString, matchesPtr uint32, nMatches uint32) bool {
	ctx := context.Background()
	res, err := libre2ABI.cre2Match.Call(ctx, uint64(rePtr), uint64(s.ptr), uint64(s.length), 0, uint64(s.length), 0, uint64(matchesPtr), uint64(nMatches))
	if err != nil {
		panic(err)
	}

	return res[0] == 1
}

func findAndConsume(re *Regexp, csPtr pointer, matchPtr uint32, nMatch uint32) bool {
	ctx := context.Background()

	sPtrOrig, ok := libre2ABI.memory.ReadUint32Le(ctx, csPtr.ptr)
	if !ok {
		panic(errFailedRead)
	}

	sLenOrig, ok := libre2ABI.memory.ReadUint32Le(ctx, csPtr.ptr+4)
	if !ok {
		panic(errFailedRead)
	}

	res, err := libre2ABI.cre2FindAndConsume.Call(ctx, uint64(re.parensPtr), uint64(csPtr.ptr), uint64(matchPtr), uint64(nMatch))
	if err != nil {
		panic(err)
	}

	sPtrNew, ok := libre2ABI.memory.ReadUint32Le(ctx, csPtr.ptr)
	if !ok {
		panic(errFailedRead)
	}

	// If the regex matched an empty string, consumption will not advance the input, so we must do it ourselves.
	if sPtrNew == sPtrOrig && sLenOrig > 0 {
		if !libre2ABI.memory.WriteUint32Le(ctx, csPtr.ptr, sPtrOrig+1) {
			panic(errFailedWrite)
		}
		if !libre2ABI.memory.WriteUint32Le(ctx, csPtr.ptr+4, sLenOrig-1) {
			panic(errFailedWrite)
		}
	}

	return res[0] != 0
}

func readMatch(cs cString, matchPtr uint32, dstCap []int) []int {
	ctx := context.Background()
	subStrPtr, ok := libre2ABI.memory.ReadUint32Le(ctx, matchPtr)
	if !ok {
		panic(errFailedRead)
	}
	if subStrPtr == 0 {
		return append(dstCap, -1, -1)
	}
	sLen, ok := libre2ABI.memory.ReadUint32Le(ctx, matchPtr+4)
	if !ok {
		panic(errFailedRead)
	}

	sIdx := subStrPtr - cs.ptr

	return append(dstCap, int(sIdx), int(sIdx+sLen))
}

func namedGroupsIter(rePtr uint32) uint32 {
	ctx := context.Background()

	groupsIter, err := libre2ABI.cre2NamedGroupsIterNew.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}

	return uint32(groupsIter[0])
}

func namedGroupsIterNext(iterPtr uint32) (string, int, bool) {
	ctx := context.Background()

	// Not on the hot path so don't bother optimizing this.
	namePtrPtr := malloc(4)
	defer free(namePtrPtr)
	indexPtr := malloc(4)
	defer free(indexPtr)

	res, err := libre2ABI.cre2NamedGroupsIterNext.Call(ctx, uint64(iterPtr), uint64(namePtrPtr), uint64(indexPtr))
	if err != nil {
		panic(err)
	}

	if res[0] == 0 {
		return "", 0, false
	}

	namePtr, ok := libre2ABI.memory.ReadUint32Le(ctx, namePtrPtr)
	if !ok {
		panic(errFailedRead)
	}

	// C-string, read content until NULL.
	name := strings.Builder{}
	for {
		b, ok := libre2ABI.memory.ReadByte(ctx, namePtr)
		if !ok {
			panic(errFailedRead)
		}
		if b == 0 {
			break
		}
		name.WriteByte(b)
		namePtr++
	}

	index, ok := libre2ABI.memory.ReadUint32Le(ctx, indexPtr)
	if !ok {
		panic(errFailedRead)
	}

	return name.String(), int(index), true
}

func namedGroupsIterDelete(iterPtr uint32) {
	ctx := context.Background()

	_, err := libre2ABI.cre2NamedGroupsIterDelete.Call(ctx, uint64(iterPtr))
	if err != nil {
		panic(err)
	}
}

func globalReplace(rePtr uint32, textAndTargetPtr uint32, rewritePtr uint32) ([]byte, bool) {
	ctx := context.Background()

	res, err := libre2ABI.cre2GlobalReplace.Call(ctx, uint64(rePtr), uint64(textAndTargetPtr), uint64(rewritePtr))
	if err != nil {
		panic(err)
	}

	if int64(res[0]) == -1 {
		panic("out of memory")
	}

	if res[0] == 0 {
		// No replacements
		return nil, false
	}

	strPtr, ok := libre2ABI.memory.ReadUint32Le(ctx, textAndTargetPtr)
	if !ok {
		panic(errFailedRead)
	}
	// This was malloc'd by cre2, so free it
	defer free(strPtr)

	strLen, ok := libre2ABI.memory.ReadUint32Le(ctx, textAndTargetPtr+4)
	if !ok {
		panic(errFailedRead)
	}

	str, ok := libre2ABI.memory.Read(ctx, strPtr, strLen)
	if !ok {
		panic(errFailedRead)
	}

	// Read returns a view, so make sure to copy it
	return append([]byte{}, str...), true
}

type cString struct {
	ptr    uint32
	length uint32
}

func (s cString) release() {
	free(s.ptr)
}

func newCString(s string) cString {
	ctx := context.Background()
	ptr := mustWriteString(ctx, s)
	return cString{
		ptr:    uint32(ptr),
		length: uint32(len(s)),
	}
}

func newCStringFromBytes(s []byte) cString {
	ctx := context.Background()
	ptr := mustWrite(ctx, s)
	return cString{
		ptr:    uint32(ptr),
		length: uint32(len(s)),
	}
}

func newCStringPtr(cs cString) pointer {
	ctx := context.Background()
	ptr := mustMalloc(ctx, 8)
	if !libre2ABI.memory.WriteUint32Le(ctx, uint32(ptr), cs.ptr) {
		panic(errFailedWrite)
	}
	if !libre2ABI.memory.WriteUint32Le(ctx, uint32(ptr+4), cs.length) {
		panic(errFailedWrite)
	}
	return pointer{ptr: uint32(ptr)}
}

type pointer struct {
	ptr uint32
}

func (p pointer) release() {
	free(p.ptr)
}

func malloc(size uint32) uint32 {
	res, err := libre2ABI.malloc.Call(context.Background(), uint64(size))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func free(ptr uint32) {
	_, err := libre2ABI.free.Call(context.Background(), uint64(ptr))
	if err != nil {
		panic(err)
	}
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

func mustFree(ctx context.Context, ptr uint64) {
	_, err := libre2ABI.free.Call(ctx, ptr)
	if err != nil {
		panic(err)
	}
}
