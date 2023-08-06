package re2

import (
	"runtime"
	"testing"
)

// TestReplaceAllNoMatch is a regression test for https://github.com/wasilibs/go-re2/issues/56
func TestReplaceAllNoMatch(t *testing.T) {
	animalRegex := MustCompile(`cat`)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	startAlloc := ms.HeapInuse

	for i := 0; i < 1000000; i++ {
		_ = animalRegex.ReplaceAllLiteralString(`The quick brown fox jumps over the lazy dog`, "animal")
	}

	runtime.GC()
	runtime.ReadMemStats(&ms)
	endAlloc := ms.HeapInuse

	if endAlloc > startAlloc && endAlloc-startAlloc > 1000000 {
		t.Errorf("memory usage increased by %d", endAlloc-startAlloc)
	}
}
