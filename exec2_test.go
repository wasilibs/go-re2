// Disable on tinygo.wasm for now since due to the poor performance of the default GC,
// these tests don't complete.
//go:build re2_test_exhaustive && !tinygo.wasm

package re2

import (
	_ "embed"
	"testing"
)

//go:embed testdata/re2-exhaustive.txt.bz2
var re2ExhaustiveTestdata []byte

// GAP: This test is excluded by default unlike the stdlib.
// This test compiles millions of regular expressions, and re2's low compilation
// performance makes it take a prohibitive amount of time even without the race
// detector as the stdlib guards on.
func TestRE2Exhaustive(t *testing.T) {
	testRE2(t, "testdata/re2-exhaustive.txt.bz2", re2ExhaustiveTestdata)
}
