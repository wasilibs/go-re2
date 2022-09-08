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

var errFailedWrite = errors.New("failed to write to wasm memory")

//go:embed libre2.wasm
var libre2 []byte

var wasmRT wazero.Runtime

type libre2ABIDef struct {
	cre2New    api.Function
	cre2Delete api.Function
	cre2Match  api.Function

	malloc api.Function
	free   api.Function

	memory api.Memory
}

var libre2ABI libre2ABIDef

func init() {
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithWasmCore2())

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		panic(err)
	}

	// TODO(anuraaga): Find a way to compile wasm without this import.
	if _, err := rt.NewModuleBuilder("env").
		ExportFunction("__main_argc_argv", func(int32, int32) int32 {
			return 0
		}).
		Instantiate(ctx, rt); err != nil {
		panic(err)
	}

	mod, err := rt.InstantiateModuleFromBinary(ctx, libre2)
	if err != nil {
		panic(err)
	}

	abi := libre2ABIDef{
		cre2New:    mod.ExportedFunction("cre2_new"),
		cre2Delete: mod.ExportedFunction("cre2_delete"),
		cre2Match:  mod.ExportedFunction("cre2_match"),

		malloc: mod.ExportedFunction("malloc"),
		free:   mod.ExportedFunction("free"),

		memory: mod.Memory(),
	}

	wasmRT = rt
	libre2ABI = abi
}

type Regexp struct {
	ptr uint32
}

func MustCompile(str string) *Regexp {
	re, err := Compile(str)
	if err != nil {
		panic(err)
	}
	return re
}

func Compile(str string) (*Regexp, error) {
	ctx := context.Background()
	ret, err := libre2ABI.malloc.Call(ctx, uint64(len(str)))
	if err != nil {
		return nil, fmt.Errorf("failed to allocate wasm memory for pattern string: %w", err)
	}

	if !libre2ABI.memory.Write(ctx, uint32(ret[0]), []byte(str)) {
		return nil, errFailedWrite
	}

	ptr, err := libre2ABI.cre2New.Call(ctx, ret[0], uint64(len(str)), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to compile pattern: %w", err)
	}

	_, err = libre2ABI.free.Call(ctx, ret[0])
	if err != nil {
		panic(err)
	}

	re := &Regexp{ptr: uint32(ptr[0])}

	return re, nil
}

func (r *Regexp) MatchString(s string) bool {
	return r.Match([]byte(s))
}

func (r *Regexp) Match(s []byte) bool {
	ctx := context.Background()
	ret, err := libre2ABI.malloc.Call(ctx, uint64(len(s)))
	if err != nil {
		panic(err)
	}

	if !libre2ABI.memory.Write(ctx, uint32(ret[0]), s) {
		panic("failed to write string to wasm memory")
	}

	matched, err := libre2ABI.cre2Match.Call(ctx, uint64(r.ptr), ret[0], uint64(len(s)), 0, uint64(len(s)), 0, 0, 0)
	if err != nil {
		panic(err)
	}

	_, err = libre2ABI.free.Call(ctx, ret[0])
	if err != nil {
		panic(err)
	}

	return matched[0] == 1
}

func (r *Regexp) Close() error {
	ctx := context.Background()
	_, err := libre2ABI.cre2Delete.Call(ctx, uint64(r.ptr))
	if err != nil {
		return fmt.Errorf("failed to delete compiled pattern: %w", err)
	}
	return nil
}
