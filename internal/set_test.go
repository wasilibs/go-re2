package internal

import (
	"errors"
	"strings"
	"testing"
)

func newUncompiledSet(t *testing.T, exprs []string) *Set {
	t.Helper()

	abi := newABI()
	set := &Set{
		ptr:   newSet(abi, CompileOptions{}),
		abi:   abi,
		exprs: exprs,
	}
	t.Cleanup(set.release)

	alloc := abi.startOperation(64)
	defer abi.endOperation(alloc)

	for _, expr := range exprs {
		if errMsg := setAdd(set, alloc.newCString(expr)); errMsg != "" {
			t.Fatalf("adding %q: %s", expr, errMsg)
		}
	}
	return set
}

func TestSetFindAllWithErrorNotCompiled(t *testing.T) {
	set := newUncompiledSet(t, []string{"abc"})

	matches, err := set.FindAllWithError([]byte("abc"), -1)
	if err == nil {
		t.Fatal("FindAllWithError: want error, got nil")
	}
	if !errors.Is(err, ErrSetEvaluation) {
		t.Errorf("FindAllWithError: error %v does not wrap ErrSetEvaluation", err)
	}
	if !strings.Contains(err.Error(), "not compiled") {
		t.Errorf("FindAllWithError: error %v does not name the failure kind", err)
	}
	if matches != nil {
		t.Errorf("FindAllWithError: want nil matches on failure, got %v", matches)
	}

	if got := set.FindAll([]byte("abc"), -1); got != nil {
		t.Errorf("FindAll: want nil, got %v", got)
	}
}

func TestSetFindAllStringWithErrorNotCompiled(t *testing.T) {
	set := newUncompiledSet(t, []string{"abc"})

	matches, err := set.FindAllStringWithError("abc", -1)
	if err == nil {
		t.Fatal("FindAllStringWithError: want error, got nil")
	}
	if !errors.Is(err, ErrSetEvaluation) {
		t.Errorf("FindAllStringWithError: error %v does not wrap ErrSetEvaluation", err)
	}
	if matches != nil {
		t.Errorf("FindAllStringWithError: want nil matches on failure, got %v", matches)
	}

	if got := set.FindAllString("abc", -1); got != nil {
		t.Errorf("FindAllString: want nil, got %v", got)
	}
}
