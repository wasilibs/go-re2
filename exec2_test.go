//go:build re2_test_exhaustive

package re2

import (
	_ "embed"
	"testing"
)

// GAP: This test is excluded by default unlike the stdlib.
// This test compiles millions of regular expressions, and re2's low compilation
// performance makes it take a prohibitive amount of time even without the race
// detector as the stdlib guards on.
func TestRE2Exhaustive(t *testing.T) {
	testRE2(t, "testdata/re2-exhaustive.txt.bz2")
}
