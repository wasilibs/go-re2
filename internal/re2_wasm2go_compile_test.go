//go:build !re2_cgo

package internal

import (
	"strings"
	"testing"
)

// A pattern RE2 refuses must not stay in the module's memory, which never
// shrinks: a program compiling patterns it doesn't control would grow with
// every invalid one.
func TestCompileFreesRefusedPattern(t *testing.T) {
	// An unclosed group, large enough that leaking it would grow the memory
	// by many pages over the loop.
	expr := "(" + strings.Repeat("a", 4096)

	// Settle the allocator before measuring.
	for range 10 {
		if _, err := Compile(expr, CompileOptions{}); err == nil {
			t.Fatal("expected a parse error")
		}
	}
	before := len(*hostMemory.Slice())

	for range 2000 {
		if _, err := Compile(expr, CompileOptions{}); err == nil {
			t.Fatal("expected a parse error")
		}
	}

	if after := len(*hostMemory.Slice()); after != before {
		t.Errorf("module memory grew from %d to %d bytes compiling a refused pattern", before, after)
	}
}
