package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/sh"
)

func Test() error {
	return sh.RunV("go", "test", "./...")
}

func Format() error {
	return sh.RunV("go", "run", fmt.Sprintf("github.com/rinchsan/gosimports/cmd/gosimports@%s", gosImportsVer), "-w",
		"-local", "github.com/anuraaga/re2-go",
		".")
}

func Check() error {
	return sh.RunV("go", "run", fmt.Sprintf("github.com/golangci/golangci-lint/cmd/golangci-lint@%s", golangCILintVer), "run")
}

var benchArgs = []string{"test", "-bench=.", "-run=^$", "-v", "./..."}
var benchCGOArgs = []string{"test", "-tags=re2_cgo", "-bench=.", "-run=^$", "-v", "./..."}
var benchSTDLibArgs = []string{"test", "-tags=re2_bench_stdlib", "-bench=.", "-run=^$", "-v", "./..."}

// Bench runs benchmarks in the default configuration for a Go app, using wazero.
func Bench() error {
	return sh.RunV("go", benchArgs...)
}

// BenchCGO runs benchmarks with re2 accessed using cgo. A C++ toolchain and libre2 must be installed to run.
func BenchCGO() error {
	return sh.RunV("go", benchCGOArgs...)
}

// BenchSTDLib runs benchmarks using the regexp library in the standard library for comparison.
func BenchSTDLib() error {
	return sh.RunV("go", benchSTDLibArgs...)
}

// BenchAll runs all benchmark types and outputs with benchstat. A C++ toolchain and libre2 must be installed to run.
func BenchAll() error {
	if err := os.MkdirAll("build", 0755); err != nil {
		return err
	}

	wazero, err := sh.Output("go", benchArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "bench.txt"), []byte(wazero), 0644); err != nil {
		return err
	}

	cgo, err := sh.Output("go", benchCGOArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "bench_cgo.txt"), []byte(cgo), 0644); err != nil {
		return err
	}

	stdlib, err := sh.Output("go", benchSTDLibArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "bench_stdlib.txt"), []byte(stdlib), 0644); err != nil {
		return err
	}

	return sh.RunV("go", "run", fmt.Sprintf("golang.org/x/perf/cmd/benchstat@%s", benchstatVer),
		"build/bench_stdlib.txt", "build/bench.txt", "build/bench_cgo.txt")
}

var wafBenchArgs = []string{"test", "-bench=.", "-run=^$", "-v", "./wafbench"}
var wafBenchCGOArgs = []string{"test", "-tags=re2_cgo", "-bench=.", "-run=^$", "-v", "./wafbench"}
var wafBenchSTDLibArgs = []string{"test", "-tags=re2_bench_stdlib", "-bench=.", "-run=^$", "-v", "./wafbench"}

// WAFBench runs benchmarks in the default configuration for a Go app, using wazero.
func WAFBench() error {
	return sh.RunV("go", wafBenchArgs...)
}

// WAFBenchCGO runs benchmarks with re2 accessed using cgo. A C++ toolchain and libre2 must be installed to run.
func WAFBenchCGO() error {
	return sh.RunV("go", wafBenchCGOArgs...)
}

// WAFBenchSTDLib runs benchmarks using the regexp library in the standard library for comparison.
func WAFBenchSTDLib() error {
	return sh.RunV("go", wafBenchSTDLibArgs...)
}

// WAFBenchAll runs all benchmark types and outputs with benchstat. A C++ toolchain and libre2 must be installed to run.
func WAFBenchAll() error {
	if err := os.MkdirAll("build", 0755); err != nil {
		return err
	}

	wazero, err := sh.Output("go", wafBenchArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "wafbench.txt"), []byte(wazero), 0644); err != nil {
		return err
	}

	cgo, err := sh.Output("go", wafBenchCGOArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "wafbench_cgo.txt"), []byte(cgo), 0644); err != nil {
		return err
	}

	stdlib, err := sh.Output("go", wafBenchSTDLibArgs...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join("build", "wafbench_stdlib.txt"), []byte(stdlib), 0644); err != nil {
		return err
	}

	return sh.RunV("go", "run", fmt.Sprintf("golang.org/x/perf/cmd/benchstat@%s", benchstatVer),
		"build/wafbench_stdlib.txt", "build/wafbench.txt", "build/wafbench_cgo.txt")
}

var Default = Test
