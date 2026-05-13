package wasm2go

import (
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/wasilibs/go-re2/internal/wasm/memory"
)

const (
	wasmPageSize     = 65536
	hostMemoryPages  = 65536 // 4GiB / 64KiB pages
	defaultInitPages = 3

	errnoSuccess = 0
	errnoBadf    = 8
	errnoFault   = 21
	errnoInval   = 28
	errnoIo      = 29
	errnoNosys   = 52
)

var ErrProcExit = errors.New("wasi proc_exit")

// HostMemory is a simple growable shared memory implementation for wasm2go.
type HostMemory struct {
	memory.Memory
	waiters sync.Map
}

func NewHostMemory(initialPages int64) *HostMemory {
	if initialPages <= 0 {
		initialPages = defaultInitPages
	}
	if initialPages > hostMemoryPages {
		initialPages = hostMemoryPages
	}
	hm := &HostMemory{}
	hm.Max = hostMemoryPages
	if hm.Memory.Grow(initialPages, hostMemoryPages) == -1 {
		panic("failed to initialize host memory")
	}
	return hm
}

func (m *HostMemory) CapacityBytes() uint64 {
	return uint64(m.Max) * wasmPageSize
}

func (m *HostMemory) Grow(delta, max int64) int64 {
	if max > 0 && max < m.Max {
		m.Max = max
	}
	return m.Memory.Grow(delta, m.Max)
}

func (m *HostMemory) Waiters() *sync.Map {
	return &m.waiters
}

func (m *HostMemory) Read(offset, byteCount uint32) []byte {
	start := int(offset)
	end := start + int(byteCount)
	return m.Buf[start:end]
}

func (m *HostMemory) ReadByte(offset uint32) byte {
	return m.Buf[int(offset)]
}

func (m *HostMemory) ReadUint32Le(offset uint32) uint32 {
	start := int(offset)
	end := start + 4
	return load32(m.Buf[start:end])
}

func (m *HostMemory) Write(offset uint32, b []byte) {
	start := int(offset)
	end := start + len(b)
	copy(m.Buf[start:end], b)
}

func (m *HostMemory) WriteString(offset uint32, s string) {
	start := int(offset)
	end := start + len(s)
	copy(m.Buf[start:end], s)
}

func (m *HostMemory) WriteByte(offset uint32, b byte) {
	m.Buf[int(offset)] = b
}

func (m *HostMemory) WriteUint32Le(offset uint32, v uint32) {
	start := int(offset)
	end := start + 4
	store32(m.Buf[start:end], v)
}

func (m *HostMemory) WriteUint64Le(offset uint32, v uint64) {
	start := int(offset)
	end := start + 8
	store64(m.Buf[start:end], v)
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
	env    []string
}

func NewHostWASI() *HostWASI {
	return &HostWASI{
		stdout: os.Stdout,
		stderr: os.Stderr,
		start:  time.Now(),
		env:    os.Environ(),
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
	if w.memory == nil {
		return errnoFault
	}

	ptrs := uint32(v0)
	buf := uint32(v1)
	for _, e := range w.env {
		w.memory.WriteUint32Le(ptrs, buf)
		ptrs += 4

		w.memory.WriteString(buf, e)
		buf += uint32(len(e))

		w.memory.WriteByte(buf, 0)
		buf++
	}

	return errnoSuccess
}

func (w *HostWASI) Xenviron_sizes_get(v0, v1 int32) int32 {
	if w.memory == nil {
		return errnoFault
	}

	var bufSize uint32
	for _, e := range w.env {
		bufSize += uint32(len(e) + 1)
	}

	w.memory.WriteUint32Le(uint32(v0), uint32(len(w.env)))
	w.memory.WriteUint32Le(uint32(v1), bufSize)
	return errnoSuccess
}

func (w *HostWASI) Xclock_time_get(v0 int32, _ int64, v2 int32) int32 {
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
	w.memory.WriteUint64Le(uint32(v2), ns)
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

func (w *HostWASI) Xfd_prestat_get(int32, int32) int32 {
	return errnoBadf
}

func (w *HostWASI) Xfd_prestat_dir_name(int32, int32, int32) int32 {
	return errnoBadf
}

func (w *HostWASI) Xfd_seek(int32, int64, int32, int32) int32 {
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
		bufPtr := w.memory.ReadUint32Le(uint32(iovec))
		bufLen := w.memory.ReadUint32Le(uint32(iovec + 4))
		start := int(bufPtr)
		end := start + int(bufLen)
		chunk := w.memory.Buf[start:end]

		n, err := out.Write(chunk)
		total += uint32(n)
		if err != nil {
			w.memory.WriteUint32Le(uint32(v3), total)
			return errnoIo
		}
		if n != len(chunk) {
			w.memory.WriteUint32Le(uint32(v3), total)
			return errnoIo
		}
	}

	w.memory.WriteUint32Le(uint32(v3), total)
	return errnoSuccess
}

func (w *HostWASI) Xpoll_oneoff(_, _, _, v3 int32) int32 {
	if w.memory == nil {
		return errnoFault
	}
	w.memory.WriteUint32Le(uint32(v3), 0)
	return errnoNosys
}

func (w *HostWASI) Xproc_exit(v0 int32) {
	os.Exit(int(v0))
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
