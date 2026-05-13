package wasm2go

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

const (
	wasmPageSize     = 65536
	hostMemoryBytes  = 512 * 1024 * 1024
	hostMemoryPages  = hostMemoryBytes / wasmPageSize
	defaultInitPages = 3

	errnoSuccess = 0
	errnoBadf    = 8
	errnoFault   = 21
	errnoInval   = 28
	errnoIo      = 29
	errnoNosys   = 52
)

// ProcExitError is raised when the guest requests process termination via proc_exit.
type ProcExitError struct {
	Code int32
}

func (e ProcExitError) Error() string {
	return fmt.Sprintf("wasi proc_exit(%d)", e.Code)
}

var ErrProcExit = errors.New("wasi proc_exit")

// HostMemory is a simple growable shared memory implementation for wasm2go.
type HostMemory struct {
	backing []byte
	mem     []byte
	waiters sync.Map
}

func NewHostMemory(initialPages int64) *HostMemory {
	if initialPages <= 0 {
		initialPages = defaultInitPages
	}
	if initialPages > hostMemoryPages {
		initialPages = hostMemoryPages
	}

	backing := make([]byte, hostMemoryBytes)
	mem := backing[:initialPages*wasmPageSize]
	return &HostMemory{backing: backing, mem: mem}
}

func (m *HostMemory) Slice() *[]byte {
	return &m.mem
}

func (m *HostMemory) CapacityBytes() uint32 {
	return uint32(len(m.backing))
}

func (m *HostMemory) Grow(delta, max int64) int64 {
	currentPages := int64(len(m.mem) / wasmPageSize)
	if delta == 0 {
		return currentPages
	}
	if delta < 0 {
		return -1
	}

	newPages := currentPages + delta
	if max > 0 && newPages > max {
		return -1
	}
	if newPages > hostMemoryPages {
		return -1
	}

	m.mem = m.backing[:newPages*wasmPageSize]
	return currentPages
}

func (m *HostMemory) Waiters() *sync.Map {
	return &m.waiters
}

func (m *HostMemory) Read(offset, byteCount uint32) ([]byte, bool) {
	return m.slice(offset, byteCount)
}

func (m *HostMemory) ReadByte(offset uint32) (byte, bool) {
	buf, ok := m.slice(offset, 1)
	if !ok {
		return 0, false
	}
	return buf[0], true
}

func (m *HostMemory) ReadUint32Le(offset uint32) (uint32, bool) {
	return m.loadU32(int32(offset))
}

func (m *HostMemory) Write(offset uint32, b []byte) bool {
	buf, ok := m.slice(offset, uint32(len(b)))
	if !ok {
		return false
	}
	copy(buf, b)
	return true
}

func (m *HostMemory) WriteString(offset uint32, s string) bool {
	buf, ok := m.slice(offset, uint32(len(s)))
	if !ok {
		return false
	}
	copy(buf, s)
	return true
}

func (m *HostMemory) WriteUint32Le(offset uint32, v uint32) bool {
	return m.storeU32(int32(offset), v)
}

func (m *HostMemory) loadU32(ptr int32) (uint32, bool) {
	start := int(ptr)
	end := start + 4
	if start < 0 || end < start || end > len(m.mem) {
		return 0, false
	}
	return load32(m.mem[start:end]), true
}

func (m *HostMemory) storeU32(ptr int32, v uint32) bool {
	start := int(ptr)
	end := start + 4
	if start < 0 || end < start || end > len(m.mem) {
		return false
	}
	store32(m.mem[start:end], v)
	return true
}

func (m *HostMemory) storeU64(ptr int32, v uint64) bool {
	start := int(ptr)
	end := start + 8
	if start < 0 || end < start || end > len(m.mem) {
		return false
	}
	store64(m.mem[start:end], v)
	return true
}

func (m *HostMemory) slice(ptr, n uint32) ([]byte, bool) {
	start := int(ptr)
	end := start + int(n)
	if start < 0 || end < start || end > len(m.mem) {
		return nil, false
	}
	return m.mem[start:end], true
}

// HostEnv provides the imported env.memory for wasm2go modules.
type HostEnv struct {
	memory Memory
}

func NewHostEnv(memory Memory) *HostEnv {
	if memory == nil {
		memory = NewHostMemory(defaultInitPages)
	}
	return &HostEnv{memory: memory}
}

func (e *HostEnv) Xmemory() Memory {
	return e.memory
}

// HostWASI is a minimal implementation of Xwasi_snapshot_preview1.
type HostWASI struct {
	stdout io.Writer
	stderr io.Writer

	memory *HostMemory
	start  time.Time
}

func NewHostWASI() *HostWASI {
	return &HostWASI{
		stdout: os.Stdout,
		stderr: os.Stderr,
		start:  time.Now(),
	}
}

// Init is discovered by generated code and called once after module creation.
func (w *HostWASI) Init(v any) {
	type memoryProvider interface {
		Xmemory() Memory
	}
	if p, ok := v.(memoryProvider); ok {
		w.memory, _ = p.Xmemory().(*HostMemory)
	}
}

func (w *HostWASI) Xenviron_get(v0, v1 int32) int32 {
	// No environment variables are exposed.
	return errnoSuccess
}

func (w *HostWASI) Xenviron_sizes_get(v0, v1 int32) int32 {
	if w.memory == nil {
		return errnoFault
	}
	if !w.memory.storeU32(v0, 0) || !w.memory.storeU32(v1, 0) {
		return errnoFault
	}
	return errnoSuccess
}

func (w *HostWASI) Xclock_time_get(v0 int32, v1 int64, v2 int32) int32 {
	_ = v1 // precision hint is ignored in this minimal host.

	var ns uint64
	switch v0 {
	case 0:
		ns = uint64(time.Now().UnixNano())
	case 1:
		ns = uint64(time.Since(w.start).Nanoseconds())
	case 2, 3:
		ns = uint64(time.Now().UnixNano())
	default:
		return errnoInval
	}

	if w.memory == nil {
		return errnoFault
	}
	if !w.memory.storeU64(v2, ns) {
		return errnoFault
	}
	return errnoSuccess
}

func (w *HostWASI) Xfd_close(v0 int32) int32 {
	switch v0 {
	case 0, 1, 2:
		return errnoSuccess
	default:
		return errnoBadf
	}
}

func (w *HostWASI) Xfd_prestat_get(v0, v1 int32) int32 {
	_ = v0
	_ = v1
	return errnoBadf
}

func (w *HostWASI) Xfd_prestat_dir_name(v0, v1, v2 int32) int32 {
	_ = v0
	_ = v1
	_ = v2
	return errnoBadf
}

func (w *HostWASI) Xfd_seek(v0 int32, v1 int64, v2, v3 int32) int32 {
	_ = v0
	_ = v1
	_ = v2
	_ = v3
	return errnoNosys
}

func (w *HostWASI) Xfd_write(v0, v1, v2, v3 int32) int32 {
	var out io.Writer
	switch v0 {
	case 1:
		out = w.stdout
	case 2:
		out = w.stderr
	default:
		return errnoBadf
	}

	if w.memory == nil {
		return errnoFault
	}

	var total uint32
	for i := range v2 {
		iovec := v1 + i*8
		bufPtr, ok := w.memory.loadU32(iovec)
		if !ok {
			return errnoFault
		}
		bufLen, ok := w.memory.loadU32(iovec + 4)
		if !ok {
			return errnoFault
		}
		chunk, ok := w.memory.slice(bufPtr, bufLen)
		if !ok {
			return errnoFault
		}

		n, err := out.Write(chunk)
		total += uint32(n)
		if err != nil {
			_ = w.memory.storeU32(v3, total)
			return errnoIo
		}
		if n != len(chunk) {
			_ = w.memory.storeU32(v3, total)
			return errnoIo
		}
	}

	if !w.memory.storeU32(v3, total) {
		return errnoFault
	}
	return errnoSuccess
}

func (w *HostWASI) Xpoll_oneoff(v0, v1, v2, v3 int32) int32 {
	_ = v0
	_ = v1
	_ = v2
	if w.memory == nil {
		return errnoFault
	}
	if !w.memory.storeU32(v3, 0) {
		return errnoFault
	}
	return errnoNosys
}

func (w *HostWASI) Xproc_exit(v0 int32) {
	panic(ProcExitError{Code: v0})
}

func (w *HostWASI) Xsched_yield() int32 {
	runtime.Gosched()
	return errnoSuccess
}

func NewHostModule() *Module {
	env := NewHostEnv(nil)
	wasi := NewHostWASI()
	return New(wasi, env)
}

var (
	_ Xwasi_snapshot_preview1 = (*HostWASI)(nil)
	_ Xenv                    = (*HostEnv)(nil)
	_ Memory                  = (*HostMemory)(nil)
)
