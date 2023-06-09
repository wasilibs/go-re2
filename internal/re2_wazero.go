//go:build !tinygo.wasm && !re2_cgo

package internal

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var (
	errFailedWrite = errors.New("failed to read from wasm memory")
	errFailedRead  = errors.New("failed to read from wasm memory")
)

//go:embed wasm/libcre2.so
var libre2 []byte

var (
	wasmRT       wazero.Runtime
	wasmCompiled wazero.CompiledModule
)

type libre2ABI struct {
	cre2New                   lazyFunction
	cre2Delete                lazyFunction
	cre2Match                 lazyFunction
	cre2NumCapturingGroups    lazyFunction
	cre2ErrorCode             lazyFunction
	cre2ErrorArg              lazyFunction
	cre2NamedGroupsIterNew    lazyFunction
	cre2NamedGroupsIterNext   lazyFunction
	cre2NamedGroupsIterDelete lazyFunction
	cre2GlobalReplace         lazyFunction
	cre2OptNew                lazyFunction
	cre2OptDelete             lazyFunction
	cre2OptSetLongestMatch    lazyFunction
	cre2OptSetPosixSyntax     lazyFunction
	cre2OptSetCaseSensitive   lazyFunction
	cre2OptSetLatin1Encoding  lazyFunction

	malloc api.Function
	free   api.Function

	wasmMemory api.Memory

	mod       api.Module
	callStack []uint64

	memory sharedMemory
	mu     sync.Mutex
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

func newABI() *libre2ABI {
	ctx := context.Background()
	mod, err := wasmRT.InstantiateModule(ctx, wasmCompiled, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		panic(err)
	}

	callStack := make([]uint64, 8) // Needs to be sized to the method with most parameters, which is cre2_match

	abi := &libre2ABI{
		cre2New:                   newLazyFunction(mod, "cre2_new", callStack),
		cre2Delete:                newLazyFunction(mod, "cre2_delete", callStack),
		cre2Match:                 newLazyFunction(mod, "cre2_match", callStack),
		cre2NumCapturingGroups:    newLazyFunction(mod, "cre2_num_capturing_groups", callStack),
		cre2ErrorCode:             newLazyFunction(mod, "cre2_error_code", callStack),
		cre2ErrorArg:              newLazyFunction(mod, "cre2_error_arg", callStack),
		cre2NamedGroupsIterNew:    newLazyFunction(mod, "cre2_named_groups_iter_new", callStack),
		cre2NamedGroupsIterNext:   newLazyFunction(mod, "cre2_named_groups_iter_next", callStack),
		cre2NamedGroupsIterDelete: newLazyFunction(mod, "cre2_named_groups_iter_delete", callStack),
		cre2GlobalReplace:         newLazyFunction(mod, "cre2_global_replace_re", callStack),
		cre2OptNew:                newLazyFunction(mod, "cre2_opt_new", callStack),
		cre2OptDelete:             newLazyFunction(mod, "cre2_opt_delete", callStack),
		cre2OptSetLongestMatch:    newLazyFunction(mod, "cre2_opt_set_longest_match", callStack),
		cre2OptSetPosixSyntax:     newLazyFunction(mod, "cre2_opt_set_posix_syntax", callStack),
		cre2OptSetCaseSensitive:   newLazyFunction(mod, "cre2_opt_set_case_sensitive", callStack),
		cre2OptSetLatin1Encoding:  newLazyFunction(mod, "cre2_opt_set_latin1_encoding", callStack),

		malloc: mod.ExportedFunction("malloc"),
		free:   mod.ExportedFunction("free"),

		wasmMemory: mod.Memory(),
		mod:        mod,
		callStack:  callStack,
	}

	return abi
}

func (abi *libre2ABI) startOperation(memorySize int) {
	abi.mu.Lock()
	abi.memory.reserve(abi, uint32(memorySize))
}

func (abi *libre2ABI) endOperation() {
	abi.mu.Unlock()
}

func newRE(abi *libre2ABI, pattern cString, opts CompileOptions) uintptr {
	ctx := context.Background()
	optPtr := uintptr(0)
	if opts != (CompileOptions{}) {
		res, err := abi.cre2OptNew.Call0(ctx)
		if err != nil {
			panic(err)
		}
		optPtr = uintptr(res)
		defer func() {
			if _, err := abi.cre2OptDelete.Call1(ctx, uint64(optPtr)); err != nil {
				panic(err)
			}
		}()
		if opts.Longest {
			_, err = abi.cre2OptSetLongestMatch.Call2(ctx, uint64(optPtr), 1)
			if err != nil {
				panic(err)
			}
		}
		if opts.Posix {
			_, err = abi.cre2OptSetPosixSyntax.Call2(ctx, uint64(optPtr), 1)
			if err != nil {
				panic(err)
			}
		}
		if opts.CaseInsensitive {
			_, err = abi.cre2OptSetCaseSensitive.Call2(ctx, uint64(optPtr), 0)
			if err != nil {
				panic(err)
			}
		}
		if opts.Latin1 {
			_, err = abi.cre2OptSetLatin1Encoding.Call1(ctx, uint64(optPtr))
			if err != nil {
				panic(err)
			}
		}
	}

	res, err := abi.cre2New.Call3(ctx, uint64(pattern.ptr), uint64(pattern.length), uint64(optPtr))
	if err != nil {
		panic(err)
	}
	return uintptr(res)
}

func reError(abi *libre2ABI, rePtr uintptr) (int, string) {
	ctx := context.Background()
	res, err := abi.cre2ErrorCode.Call1(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	code := int(res)
	if code == 0 {
		return 0, ""
	}

	argPtr := newCStringArray(abi, 1)
	_, err = abi.cre2ErrorArg.Call2(ctx, uint64(rePtr), uint64(argPtr.ptr))
	if err != nil {
		panic(err)
	}
	sPtr := binary.LittleEndian.Uint32(abi.memory.read(abi, argPtr.ptr, 4))
	sLen := binary.LittleEndian.Uint32(abi.memory.read(abi, argPtr.ptr+4, 4))

	return code, string(abi.memory.read(abi, uintptr(sPtr), int(sLen)))
}

func numCapturingGroups(abi *libre2ABI, rePtr uintptr) int {
	ctx := context.Background()
	res, err := abi.cre2NumCapturingGroups.Call1(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}
	return int(res)
}

func deleteRE(abi *libre2ABI, rePtr uintptr) {
	ctx := context.Background()
	if _, err := abi.cre2Delete.Call1(ctx, uint64(rePtr)); err != nil {
		panic(err)
	}
}

func release(re *Regexp) {
	ctx := context.Background()
	deleteRE(re.abi, re.ptr)
	if err := re.abi.mod.Close(ctx); err != nil {
		fmt.Printf("error closing wazero module: %v", err)
	}
}

func match(re *Regexp, s cString, matchesPtr uintptr, nMatches uint32) bool {
	ctx := context.Background()
	res, err := re.abi.cre2Match.Call8(ctx, uint64(re.ptr), uint64(s.ptr), uint64(s.length), 0, uint64(s.length), 0, uint64(matchesPtr), uint64(nMatches))
	if err != nil {
		panic(err)
	}

	return res == 1
}

func matchFrom(re *Regexp, s cString, startPos int, matchesPtr uintptr, nMatches uint32) bool {
	ctx := context.Background()
	res, err := re.abi.cre2Match.Call8(ctx, uint64(re.ptr), uint64(s.ptr), uint64(s.length), uint64(startPos), uint64(s.length), 0, uint64(matchesPtr), uint64(nMatches))
	if err != nil {
		panic(err)
	}

	return res == 1
}

func readMatch(abi *libre2ABI, cs cString, matchPtr uintptr, dstCap []int) []int {
	matchBuf := abi.memory.read(abi, matchPtr, 8)
	subStrPtr := uintptr(binary.LittleEndian.Uint32(matchBuf))
	sLen := uintptr(binary.LittleEndian.Uint32(matchBuf[4:]))
	sIdx := subStrPtr - cs.ptr

	return append(dstCap, int(sIdx), int(sIdx+sLen))
}

func readMatches(abi *libre2ABI, cs cString, matchesPtr uintptr, n int, deliver func([]int)) {
	var dstCap [2]int

	matchesBuf := abi.memory.read(abi, matchesPtr, 8*n)
	for i := 0; i < n; i++ {
		subStrPtr := uintptr(binary.LittleEndian.Uint32(matchesBuf[8*i:]))
		if subStrPtr == 0 {
			deliver(append(dstCap[:0], -1, -1))
			continue
		}
		sLen := uintptr(binary.LittleEndian.Uint32(matchesBuf[8*i+4:]))
		sIdx := subStrPtr - cs.ptr
		deliver(append(dstCap[:0], int(sIdx), int(sIdx+sLen)))
	}
}

func namedGroupsIter(abi *libre2ABI, rePtr uintptr) uintptr {
	ctx := context.Background()

	res, err := abi.cre2NamedGroupsIterNew.Call1(ctx, uint64(rePtr))
	if err != nil {
		panic(err)
	}

	return uintptr(res)
}

func namedGroupsIterNext(abi *libre2ABI, iterPtr uintptr) (string, int, bool) {
	ctx := context.Background()

	// Not on the hot path so don't bother optimizing this yet.
	ptrs := malloc(abi, 8)
	defer free(abi, ptrs)
	namePtrPtr := ptrs
	indexPtr := namePtrPtr + 4

	res, err := abi.cre2NamedGroupsIterNext.Call3(ctx, uint64(iterPtr), uint64(namePtrPtr), uint64(indexPtr))
	if err != nil {
		panic(err)
	}

	if res == 0 {
		return "", 0, false
	}

	namePtr, ok := abi.wasmMemory.ReadUint32Le(uint32(namePtrPtr))
	if !ok {
		panic(errFailedRead)
	}

	// C-string, read content until NULL.
	name := strings.Builder{}
	for {
		b, ok := abi.wasmMemory.ReadByte(namePtr)
		if !ok {
			panic(errFailedRead)
		}
		if b == 0 {
			break
		}
		name.WriteByte(b)
		namePtr++
	}

	index, ok := abi.wasmMemory.ReadUint32Le(uint32(indexPtr))
	if !ok {
		panic(errFailedRead)
	}

	return name.String(), int(index), true
}

func namedGroupsIterDelete(abi *libre2ABI, iterPtr uintptr) {
	ctx := context.Background()

	_, err := abi.cre2NamedGroupsIterDelete.Call1(ctx, uint64(iterPtr))
	if err != nil {
		panic(err)
	}
}

func globalReplace(re *Regexp, textAndTargetPtr uintptr, rewritePtr uintptr) ([]byte, bool) {
	ctx := context.Background()

	res, err := re.abi.cre2GlobalReplace.Call3(ctx, uint64(re.ptr), uint64(textAndTargetPtr), uint64(rewritePtr))
	if err != nil {
		panic(err)
	}

	if int64(res) == -1 {
		panic("out of memory")
	}

	if res == 0 {
		// No replacements
		return nil, false
	}

	strPtr, ok := re.abi.wasmMemory.ReadUint32Le(uint32(textAndTargetPtr))
	if !ok {
		panic(errFailedRead)
	}
	// This was malloc'd by cre2, so free it
	defer free(re.abi, uintptr(strPtr))

	strLen, ok := re.abi.wasmMemory.ReadUint32Le(uint32(textAndTargetPtr + 4))
	if !ok {
		panic(errFailedRead)
	}

	str, ok := re.abi.wasmMemory.Read(strPtr, strLen)
	if !ok {
		panic(errFailedRead)
	}

	// Read returns a view, so make sure to copy it
	return append([]byte{}, str...), true
}

type cString struct {
	ptr    uintptr
	length int
}

func newCString(abi *libre2ABI, s string) cString {
	ptr := abi.memory.writeString(abi, s)
	return cString{
		ptr:    ptr,
		length: len(s),
	}
}

func newCStringFromBytes(abi *libre2ABI, s []byte) cString {
	ptr := abi.memory.write(abi, s)
	return cString{
		ptr:    ptr,
		length: len(s),
	}
}

func newCStringPtr(abi *libre2ABI, cs cString) pointer {
	ptr := abi.memory.allocate(8)
	if !abi.wasmMemory.WriteUint32Le(uint32(ptr), uint32(cs.ptr)) {
		panic(errFailedWrite)
	}
	if !abi.wasmMemory.WriteUint32Le(uint32(ptr+4), uint32(cs.length)) {
		panic(errFailedWrite)
	}
	return pointer{ptr: ptr, abi: abi}
}

type cStringArray struct {
	ptr uintptr
}

func newCStringArray(abi *libre2ABI, n int) cStringArray {
	ptr := abi.memory.allocate(uint32(n * 8))
	return cStringArray{ptr: ptr}
}

type pointer struct {
	ptr uintptr
	abi *libre2ABI
}

func malloc(abi *libre2ABI, size uint32) uintptr {
	callStack := abi.callStack
	callStack[0] = uint64(size)
	if err := abi.malloc.CallWithStack(context.Background(), callStack); err != nil {
		panic(err)
	}
	return uintptr(callStack[0])
}

func free(abi *libre2ABI, ptr uintptr) {
	callStack := abi.callStack
	callStack[0] = uint64(ptr)
	if err := abi.free.CallWithStack(context.Background(), callStack); err != nil {
		panic(err)
	}
}

type sharedMemory struct {
	size    uint32
	bufPtr  uint32
	nextIdx uint32
}

func (m *sharedMemory) reserve(abi *libre2ABI, size uint32) {
	m.nextIdx = 0
	if m.size >= size {
		return
	}

	if m.bufPtr != 0 {
		free(abi, uintptr(m.bufPtr))
	}

	m.bufPtr = uint32(malloc(abi, size))
	m.size = size
}

func (m *sharedMemory) allocate(size uint32) uintptr {
	if m.nextIdx+size > m.size {
		panic("not enough reserved shared memory")
	}

	ptr := m.bufPtr + m.nextIdx
	m.nextIdx += size
	return uintptr(ptr)
}

func (m *sharedMemory) read(abi *libre2ABI, ptr uintptr, size int) []byte {
	buf, ok := abi.wasmMemory.Read(uint32(ptr), uint32(size))
	if !ok {
		panic(errFailedRead)
	}
	return buf
}

func (m *sharedMemory) write(abi *libre2ABI, b []byte) uintptr {
	ptr := m.allocate(uint32(len(b)))
	abi.wasmMemory.Write(uint32(ptr), b)
	return ptr
}

func (m *sharedMemory) writeString(abi *libre2ABI, s string) uintptr {
	ptr := m.allocate(uint32(len(s)))
	abi.wasmMemory.WriteString(uint32(ptr), s)
	return ptr
}

type lazyFunction struct {
	f         api.Function
	mod       api.Module
	name      string
	callStack []uint64
}

func newLazyFunction(mod api.Module, name string, callStack []uint64) lazyFunction {
	return lazyFunction{mod: mod, name: name, callStack: callStack}
}

func (f *lazyFunction) Call0(ctx context.Context) (uint64, error) {
	return f.callWithStack(ctx)
}

func (f *lazyFunction) Call1(ctx context.Context, arg1 uint64) (uint64, error) {
	f.callStack[0] = arg1
	return f.callWithStack(ctx)
}

func (f *lazyFunction) Call2(ctx context.Context, arg1 uint64, arg2 uint64) (uint64, error) {
	f.callStack[0] = arg1
	f.callStack[1] = arg2
	return f.callWithStack(ctx)
}

func (f *lazyFunction) Call3(ctx context.Context, arg1 uint64, arg2 uint64, arg3 uint64) (uint64, error) {
	f.callStack[0] = arg1
	f.callStack[1] = arg2
	f.callStack[2] = arg3
	return f.callWithStack(ctx)
}

func (f *lazyFunction) Call8(ctx context.Context, arg1 uint64, arg2 uint64, arg3 uint64, arg4 uint64, arg5 uint64, arg6 uint64, arg7 uint64, arg8 uint64) (uint64, error) {
	f.callStack[0] = arg1
	f.callStack[1] = arg2
	f.callStack[2] = arg3
	f.callStack[3] = arg4
	f.callStack[4] = arg5
	f.callStack[5] = arg6
	f.callStack[6] = arg7
	f.callStack[7] = arg8
	return f.callWithStack(ctx)
}

func (f *lazyFunction) callWithStack(ctx context.Context) (uint64, error) {
	if f.f == nil {
		f.f = f.mod.ExportedFunction(f.name)
	}
	if err := f.f.CallWithStack(ctx, f.callStack); err != nil {
		return 0, err
	}
	return f.callStack[0], nil
}
