//go:build !tinygo.wasm

package re2

import (
	"context"
	_ "embed"
	"errors"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"strconv"
	"strings"
)

var errFailedWrite = errors.New("failed to read from wasm memory")
var errFailedRead = errors.New("failed to read from wasm memory")

//go:embed wasm/libcre2.so
var libre2 []byte

var wasmRT wazero.Runtime
var wasmCompiled wazero.CompiledModule

type libre2ABI struct {
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

	mod api.Module
}

func init() {
	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	code, err := rt.CompileModule(ctx, libre2)
	if err != nil {
		panic(err)
	}
	wasmCompiled = code

	wasmRT = rt
}

var moduleIdx = 0

func newABI() libre2ABI {
	ctx := context.Background()
	mod, err := wasmRT.InstantiateModule(ctx, wasmCompiled, wazero.NewModuleConfig().WithName(strconv.Itoa(moduleIdx)))
	if err != nil {
		panic(err)
	}
	moduleIdx++

	return libre2ABI{
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
		mod:    mod,
	}
}

func newRE(abi *libre2ABI, pattern cString, longest bool) uint32 {
	ctx := context.Background()
	res, err := abi.cre2OptNew.Call(ctx)
	if err != nil {
		panic(err)
	}
	optPtr := uint32(res[0])
	defer func() {
		if _, err := abi.cre2OptDelete.Call(ctx, uint64(optPtr)); err != nil {
			panic(err)
		}
	}()
	if longest {
		_, err = abi.cre2OptSetLongestMatch.Call(ctx, uint64(optPtr), 1)
		if err != nil {
			panic(err)
		}
	}
	res, err = abi.cre2New.Call(ctx, uint64(pattern.ptr), uint64(pattern.length), uint64(optPtr))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func reError(abi *libre2ABI, rePtr uint32) uint32 {
	ctx := context.Background()
	res, err := abi.cre2ErrorCode.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func numCapturingGroups(abi *libre2ABI, rePtr uint32) int {
	ctx := context.Background()
	res, err := abi.cre2NumCapturingGroups.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	return int(res[0])
}

func release(re *Regexp) {
	ctx := context.Background()
	if _, err := re.abi.cre2Delete.Call(ctx, uint64(re.ptr)); err != nil {
		panic(err)
	}
	if _, err := re.abi.cre2Delete.Call(ctx, uint64(re.parensPtr)); err != nil {
		panic(err)
	}
	re.abi.mod.Close(ctx)
}

func match(re *Regexp, s cString, matchesPtr uint32, nMatches uint32) bool {
	ctx := context.Background()
	res, err := re.abi.cre2Match.Call(ctx, uint64(re.ptr), uint64(s.ptr), uint64(s.length), 0, uint64(s.length), 0, uint64(matchesPtr), uint64(nMatches))
	if err != nil {
		panic(err)
	}

	return res[0] == 1
}

func findAndConsume(re *Regexp, csPtr pointer, matchPtr uint32, nMatch uint32) bool {
	ctx := context.Background()

	sPtrOrig, ok := re.abi.memory.ReadUint32Le(ctx, csPtr.ptr)
	if !ok {
		panic(errFailedRead)
	}

	sLenOrig, ok := re.abi.memory.ReadUint32Le(ctx, csPtr.ptr+4)
	if !ok {
		panic(errFailedRead)
	}

	res, err := re.abi.cre2FindAndConsume.Call(ctx, uint64(re.parensPtr), uint64(csPtr.ptr), uint64(matchPtr), uint64(nMatch))
	if err != nil {
		panic(err)
	}

	sPtrNew, ok := re.abi.memory.ReadUint32Le(ctx, csPtr.ptr)
	if !ok {
		panic(errFailedRead)
	}

	// If the regex matched an empty string, consumption will not advance the input, so we must do it ourselves.
	if sPtrNew == sPtrOrig && sLenOrig > 0 {
		if !re.abi.memory.WriteUint32Le(ctx, csPtr.ptr, sPtrOrig+1) {
			panic(errFailedWrite)
		}
		if !re.abi.memory.WriteUint32Le(ctx, csPtr.ptr+4, sLenOrig-1) {
			panic(errFailedWrite)
		}
	}

	return res[0] != 0
}

func readMatch(re *Regexp, cs cString, matchPtr uint32, dstCap []int) []int {
	ctx := context.Background()
	subStrPtr, ok := re.abi.memory.ReadUint32Le(ctx, matchPtr)
	if !ok {
		panic(errFailedRead)
	}
	if subStrPtr == 0 {
		return append(dstCap, -1, -1)
	}
	sLen, ok := re.abi.memory.ReadUint32Le(ctx, matchPtr+4)
	if !ok {
		panic(errFailedRead)
	}

	sIdx := subStrPtr - cs.ptr

	return append(dstCap, int(sIdx), int(sIdx+sLen))
}

func namedGroupsIter(abi *libre2ABI, rePtr uint32) uint32 {
	ctx := context.Background()

	groupsIter, err := abi.cre2NamedGroupsIterNew.Call(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}

	return uint32(groupsIter[0])
}

func namedGroupsIterNext(abi *libre2ABI, iterPtr uint32) (string, int, bool) {
	ctx := context.Background()

	// Not on the hot path so don't bother optimizing this.
	namePtrPtr := malloc(abi, 4)
	defer free(abi, namePtrPtr)
	indexPtr := malloc(abi, 4)
	defer free(abi, indexPtr)

	res, err := abi.cre2NamedGroupsIterNext.Call(ctx, uint64(iterPtr), uint64(namePtrPtr), uint64(indexPtr))
	if err != nil {
		panic(err)
	}

	if res[0] == 0 {
		return "", 0, false
	}

	namePtr, ok := abi.memory.ReadUint32Le(ctx, namePtrPtr)
	if !ok {
		panic(errFailedRead)
	}

	// C-string, read content until NULL.
	name := strings.Builder{}
	for {
		b, ok := abi.memory.ReadByte(ctx, namePtr)
		if !ok {
			panic(errFailedRead)
		}
		if b == 0 {
			break
		}
		name.WriteByte(b)
		namePtr++
	}

	index, ok := abi.memory.ReadUint32Le(ctx, indexPtr)
	if !ok {
		panic(errFailedRead)
	}

	return name.String(), int(index), true
}

func namedGroupsIterDelete(abi *libre2ABI, iterPtr uint32) {
	ctx := context.Background()

	_, err := abi.cre2NamedGroupsIterDelete.Call(ctx, uint64(iterPtr))
	if err != nil {
		panic(err)
	}
}

func globalReplace(re *Regexp, textAndTargetPtr uint32, rewritePtr uint32) ([]byte, bool) {
	ctx := context.Background()

	res, err := re.abi.cre2GlobalReplace.Call(ctx, uint64(re.ptr), uint64(textAndTargetPtr), uint64(rewritePtr))
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

	strPtr, ok := re.abi.memory.ReadUint32Le(ctx, textAndTargetPtr)
	if !ok {
		panic(errFailedRead)
	}
	// This was malloc'd by cre2, so free it
	defer free(&re.abi, strPtr)

	strLen, ok := re.abi.memory.ReadUint32Le(ctx, textAndTargetPtr+4)
	if !ok {
		panic(errFailedRead)
	}

	str, ok := re.abi.memory.Read(ctx, strPtr, strLen)
	if !ok {
		panic(errFailedRead)
	}

	// Read returns a view, so make sure to copy it
	return append([]byte{}, str...), true
}

type cString struct {
	ptr    uint32
	length uint32
	abi    *libre2ABI
}

func (s cString) release() {
	free(s.abi, s.ptr)
}

func newCString(abi *libre2ABI, s string) cString {
	ctx := context.Background()
	ptr := mustWriteString(ctx, abi, s)
	return cString{
		ptr:    ptr,
		length: uint32(len(s)),
		abi:    abi,
	}
}

func newCStringFromBytes(abi *libre2ABI, s []byte) cString {
	ctx := context.Background()
	ptr := mustWrite(ctx, abi, s)
	return cString{
		ptr:    ptr,
		length: uint32(len(s)),
		abi:    abi,
	}
}

func newCStringPtr(abi *libre2ABI, cs cString) pointer {
	ctx := context.Background()
	ptr := malloc(abi, 8)
	if !abi.memory.WriteUint32Le(ctx, ptr, cs.ptr) {
		panic(errFailedWrite)
	}
	if !abi.memory.WriteUint32Le(ctx, ptr+4, cs.length) {
		panic(errFailedWrite)
	}
	return pointer{ptr: ptr, abi: abi}
}

type pointer struct {
	ptr uint32
	abi *libre2ABI
}

func (p pointer) release() {
	free(p.abi, p.ptr)
}

func malloc(abi *libre2ABI, size uint32) uint32 {
	res, err := abi.malloc.Call(context.Background(), uint64(size))
	if err != nil {
		panic(err)
	}
	return uint32(res[0])
}

func free(abi *libre2ABI, ptr uint32) {
	_, err := abi.free.Call(context.Background(), uint64(ptr))
	if err != nil {
		panic(err)
	}
}

func mustWrite(ctx context.Context, abi *libre2ABI, s []byte) uint32 {
	ptr := malloc(abi, uint32(len(s)))

	if !abi.memory.Write(ctx, ptr, s) {
		panic("failed to write string to wasm memory")
	}

	return ptr
}

func mustWriteString(ctx context.Context, abi *libre2ABI, s string) uint32 {
	ptr := malloc(abi, uint32(len(s)))

	if !abi.memory.WriteString(ctx, ptr, s) {
		panic("failed to write string to wasm memory")
	}

	return ptr
}
