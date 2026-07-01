//go:build !tinygo.wasm && !re2_cgo && re2_wasm2go

package internal

import (
	"encoding/binary"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	wasm2go "github.com/wasilibs/go-re2/internal/wasm"
)

// defaultChildStackBytes is the size of the stack region reserved in linear
// memory for each child module. RE2 compiles and matches using
// heap memory and uses very little stack (we have measured no more than 3.25KB
// in testing). So we reserve a fixed, conservative value.
// Can be overridden with RE2_MAX_STACK_BYTES environment variable if needed.
const defaultChildStackBytes = 16 * 1024

var childRegionBytes uint32 = defaultChildStackBytes

var (
	hostMemory *wasm2go.HostMemory
	hostWASI   *wasm2go.HostWASI
	hostEnv    *wasm2go.HostEnv
	rootMod    *wasm2go.Module

	modPoolOnce sync.Once
	modPool     sync.Pool
	modCreateMu sync.Mutex
)

type libre2ABI struct{}

type wasmPtr uint32

var nilWasmPtr = wasmPtr(0)

var prevTID uint32

type childModule struct {
	mod        *wasm2go.Module
	tlsBasePtr uint32
}

func createChildModule(root *wasm2go.Module) *childModule {
	size := childRegionBytes

	ptr := uint32(root.Xmalloc(int32(size)))

	child := wasm2go.New(hostWASI, hostEnv)
	child.X__wasm_init_tls(int32(ptr))

	tid := atomic.AddUint32(&prevTID, 1)
	hostMemory.WriteUint32Le(ptr, ptr)
	hostMemory.WriteUint32Le(ptr+20, tid)
	*child.X__stack_pointer() = int32(ptr + size)

	ret := &childModule{mod: child, tlsBasePtr: ptr}
	runtime.SetFinalizer(ret, func(obj interface{}) {
		if cm, ok := obj.(*childModule); ok {
			cm.mod.Xfree(int32(cm.tlsBasePtr))
		}
	})
	return ret
}

func getChildModule() *childModule {
	modPoolOnce.Do(func() {
		initWASM()
	})
	return modPool.Get().(*childModule) //nolint:forcetypeassert // fixed-type pooling
}

func putChildModule(cm *childModule) {
	modPool.Put(cm)
}

func initWASM() {
	if v := os.Getenv("RE2_MAX_STACK_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			childRegionBytes = uint32(n)
		}
	}

	maxPages := defaultMaxPages
	if unsafe.Sizeof(uintptr(0)) < 8 {
		// On a 32-bit system. anything close to 4GB will fail (part of 4GB is already used by the rest of the process).
		// We go ahead and cap to 1GB to be extra conservative. It will be using interpreter mode anyways so either
		// the memory limit or the performance will be an issue either way.
		if maxPagesLimit := uint32(65536 / 4); maxPages > maxPagesLimit {
			maxPages = maxPagesLimit
		}
	}

	hostMemory = wasm2go.NewHostMemoryWithMax(3, int64(maxPages))
	hostWASI = wasm2go.NewHostWASI(hostMemory)
	hostEnv = wasm2go.NewHostEnv(hostMemory)
	rootMod = wasm2go.New(hostWASI, hostEnv)
	rootMod.X_initialize()
	modPool = sync.Pool{
		New: func() any {
			modCreateMu.Lock()
			defer modCreateMu.Unlock()
			return createChildModule(rootMod)
		},
	}
}

func newABI() *libre2ABI {
	return &libre2ABI{}
}

func (abi *libre2ABI) startOperation(memorySize int) allocation {
	return abi.reserve(uint32(memorySize))
}

func (abi *libre2ABI) endOperation(a allocation) {
	a.free()
}

func withModule(fn func(*wasm2go.Module) uint64) uint64 {
	modH := getChildModule()
	defer putChildModule(modH)
	return fn(modH.mod)
}

func withModuleNoResult(fn func(*wasm2go.Module)) {
	modH := getChildModule()
	defer putChildModule(modH)
	fn(modH.mod)
}

func newRE(abi *libre2ABI, pattern cString, opts CompileOptions) wasmPtr {
	_ = abi
	optPtr := uint32(withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_opt_new())
	}))

	defer func() {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_delete(int32(optPtr))
		})
	}()

	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xcre2_opt_set_max_mem(int32(optPtr), int64(maxSize))
	})

	if opts.Longest {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_longest_match(int32(optPtr), 1)
		})
	}
	if opts.Posix {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_posix_syntax(int32(optPtr), 1)
		})
	}
	if opts.CaseInsensitive {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_case_sensitive(int32(optPtr), 0)
		})
	}
	if opts.Latin1 {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_latin1_encoding(int32(optPtr))
		})
	}

	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_new(int32(pattern.ptr), int32(pattern.length), int32(optPtr)))
	})
	return wasmPtr(res)
}

func reError(abi *libre2ABI, rePtr wasmPtr) (int, string) {
	_ = abi
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_error_code(int32(rePtr)))
	})
	code := int(res)
	if code == 0 {
		return 0, ""
	}

	res = withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_error_arg(int32(rePtr)))
	})
	msg := copyCString(wasmPtr(res))
	return code, msg
}

func numCapturingGroups(abi *libre2ABI, rePtr wasmPtr) int {
	_ = abi
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_num_capturing_groups(int32(rePtr)))
	})
	return int(res)
}

func deleteRE(abi *libre2ABI, rePtr wasmPtr) {
	_ = abi
	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xcre2_delete(int32(rePtr))
	})
}

func release(re *Regexp) {
	deleteRE(re.abi, re.ptr)
}

func match(re *Regexp, s cString, matchesPtr wasmPtr, nMatches uint32) bool {
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_match(int32(re.ptr), int32(s.ptr), int32(s.length), 0, int32(s.length), 0, int32(matchesPtr), int32(nMatches)))
	})

	return res == 1
}

func matchFrom(re *Regexp, s cString, startPos int, matchesPtr wasmPtr, nMatches uint32) bool {
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_match(int32(re.ptr), int32(s.ptr), int32(s.length), int32(startPos), int32(s.length), 0, int32(matchesPtr), int32(nMatches)))
	})

	return res == 1
}

func readMatch(alloc *allocation, cs cString, matchPtr wasmPtr, dstCap []int) []int {
	matchBuf := alloc.read(matchPtr, 8)
	subStrPtr := binary.LittleEndian.Uint32(matchBuf)
	sLen := binary.LittleEndian.Uint32(matchBuf[4:])
	sIdx := subStrPtr - uint32(cs.ptr)

	return append(dstCap, int(sIdx), int(sIdx+sLen))
}

func readMatches(alloc *allocation, cs cString, matchesPtr wasmPtr, n int, deliver func([]int) bool) {
	var dstCap [2]int

	matchesBuf := alloc.read(matchesPtr, 8*n)
	for i := range n {
		subStrPtr := binary.LittleEndian.Uint32(matchesBuf[8*i:])
		if subStrPtr == 0 {
			if !deliver(append(dstCap[:0], -1, -1)) {
				break
			}
			continue
		}
		sLen := binary.LittleEndian.Uint32(matchesBuf[8*i+4:])
		sIdx := subStrPtr - uint32(cs.ptr)
		if !deliver(append(dstCap[:0], int(sIdx), int(sIdx+sLen))) {
			break
		}
	}
}

func namedGroupsIter(abi *libre2ABI, rePtr wasmPtr) wasmPtr {
	_ = abi
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_named_groups_iter_new(int32(rePtr)))
	})

	return wasmPtr(res)
}

func namedGroupsIterNext(abi *libre2ABI, iterPtr wasmPtr) (string, int, bool) {
	_ = abi

	// Not on the hot path so don't bother optimizing this yet.
	ptrs := malloc(abi, 8)
	defer free(abi, ptrs)
	namePtrPtr := ptrs
	indexPtr := namePtrPtr + 4

	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_named_groups_iter_next(int32(iterPtr), int32(namePtrPtr), int32(indexPtr)))
	})

	if res == 0 {
		return "", 0, false
	}

	namePtr := hostMemory.ReadUint32Le(uint32(namePtrPtr))

	name := copyCString(wasmPtr(namePtr))

	index := hostMemory.ReadUint32Le(uint32(indexPtr))

	return name, int(index), true
}

func namedGroupsIterDelete(abi *libre2ABI, iterPtr wasmPtr) {
	_ = abi
	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xcre2_named_groups_iter_delete(int32(iterPtr))
	})
}

func newSet(abi *libre2ABI, opts CompileOptions) wasmPtr {
	_ = abi
	optPtr := uint32(withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_opt_new())
	}))
	defer func() {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_delete(int32(optPtr))
		})
	}()

	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xcre2_opt_set_max_mem(int32(optPtr), int64(maxSize))
	})

	if opts.Longest {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_longest_match(int32(optPtr), 1)
		})
	}
	if opts.Posix {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_posix_syntax(int32(optPtr), 1)
		})
	}
	if opts.CaseInsensitive {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_case_sensitive(int32(optPtr), 0)
		})
	}
	if opts.Latin1 {
		withModuleNoResult(func(m *wasm2go.Module) {
			m.Xcre2_opt_set_latin1_encoding(int32(optPtr))
		})
	}

	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_set_new(int32(optPtr), 0))
	})
	return wasmPtr(res)
}

func setAdd(set *Set, s cString) string {
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_set_add(int32(set.ptr), int32(s.ptr), int32(s.length)))
	})
	if res == 0 {
		return unknownCompileError
	}
	msgPtr := wasmPtr(res)
	msg := copyCString(msgPtr)
	if msg != "ok" {
		free(set.abi, msgPtr)
		return "error parsing regexp: " + msg
	}
	return ""
}

func setCompile(set *Set) int32 {
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_set_compile(int32(set.ptr)))
	})
	return int32(res)
}

func setMatch(set *Set, cs cString, matchedPtr wasmPtr, nMatch int) int {
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xcre2_set_match(int32(set.ptr), int32(cs.ptr), int32(cs.length), int32(matchedPtr), int32(nMatch)))
	})
	return int(res)
}

func deleteSet(abi *libre2ABI, setPtr wasmPtr) {
	_ = abi
	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xcre2_set_delete(int32(setPtr))
	})
}

type cString struct {
	ptr    wasmPtr
	length int
}

type cStringArray struct {
	ptr wasmPtr
}

func (a cStringArray) free() {
	// We pool allocation and don't need to explicitly free.
}

func malloc(abi *libre2ABI, size uint32) wasmPtr {
	_ = abi
	res := withModule(func(m *wasm2go.Module) uint64 {
		return uint64(m.Xmalloc(int32(size)))
	})
	return wasmPtr(res)
}

func free(abi *libre2ABI, ptr wasmPtr) {
	_ = abi
	withModuleNoResult(func(m *wasm2go.Module) {
		m.Xfree(int32(ptr))
	})
}

func copyCString(ptr wasmPtr) string {
	res := strings.Builder{}
	for {
		b := hostMemory.ReadByte(uint32(ptr))
		if b == 0 {
			break
		}
		res.WriteByte(b)
		ptr++
	}
	return res.String()
}

type allocation struct {
	size    uint32
	bufPtr  wasmPtr
	nextIdx uint32
	abi     *libre2ABI
}

func (abi *libre2ABI) reserve(size uint32) allocation {
	ptr := malloc(abi, size)
	return allocation{
		size:    size,
		bufPtr:  ptr,
		nextIdx: 0,
		abi:     abi,
	}
}

func (a *allocation) free() {
	free(a.abi, a.bufPtr)
}

func (a *allocation) allocate(size uint32) wasmPtr {
	if a.nextIdx+size > a.size {
		panic("not enough reserved shared memory")
	}

	ptr := uint32(a.bufPtr) + a.nextIdx
	a.nextIdx += size
	return wasmPtr(ptr)
}

func (a *allocation) read(ptr wasmPtr, size int) []byte {
	return hostMemory.Read(uint32(ptr), uint32(size))
}

func (a *allocation) write(b []byte) wasmPtr {
	ptr := a.allocate(uint32(len(b)))
	hostMemory.Write(uint32(ptr), b)
	return ptr
}

func (a *allocation) writeString(s string) wasmPtr {
	ptr := a.allocate(uint32(len(s)))
	hostMemory.WriteString(uint32(ptr), s)
	return ptr
}

func (a *allocation) newCString(s string) cString {
	ptr := a.writeString(s)
	return cString{
		ptr:    ptr,
		length: len(s),
	}
}

func (a *allocation) newCStringFromBytes(s []byte) cString {
	ptr := a.write(s)
	return cString{
		ptr:    ptr,
		length: len(s),
	}
}

func (a *allocation) newCStringArray(n int) cStringArray {
	ptr := a.allocate(uint32(n * 8))
	return cStringArray{ptr: ptr}
}
