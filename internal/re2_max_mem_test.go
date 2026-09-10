package internal

import (
	"strings"
	"testing"
)

// largeExpr does not compile within a small memory budget, but does within the default one.
var largeExpr = `(?i)\b(?:` + strings.Repeat(`alpha[a-z]{3,12}beta|`, 400) + `zulu)\b`

func TestCompileMaxMem(t *testing.T) {
	if _, err := Compile(largeExpr, CompileOptions{MaxMem: 256 << 10}); err == nil {
		t.Error("compiled with a budget too small for the expression")
	}
	if _, err := Compile(largeExpr, CompileOptions{MaxMem: 4 << 20}); err != nil {
		t.Errorf("failed to compile with a raised budget: %v", err)
	}
	if _, err := Compile(largeExpr, CompileOptions{}); err != nil {
		t.Errorf("failed to compile with the default budget: %v", err)
	}
}

func TestSetMaxMem(t *testing.T) {
	exprs := []string{largeExpr}
	if _, err := CompileSet(exprs, CompileOptions{MaxMem: 256 << 10}); err == nil {
		t.Error("set compiled with a budget too small for the expression")
	}
	if _, err := CompileSet(exprs, CompileOptions{MaxMem: 4 << 20}); err != nil {
		t.Errorf("set failed to compile with a raised budget: %v", err)
	}
	if _, err := CompileSet(exprs, CompileOptions{}); err != nil {
		t.Errorf("set failed to compile with the default budget: %v", err)
	}
}
