package main

import (
	"fmt"

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

// Bench runs benchmarks in the default configuration for a Go app, using wazero.
func Bench() error {
	return sh.RunV("go", "test", "-bench=.", "-v", "./...")
}

var Default = Test
