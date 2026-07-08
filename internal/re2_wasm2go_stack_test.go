//go:build !re2_cgo && !re2_wazero

package internal

import (
	"strings"
	"testing"

	wasm2go "github.com/wasilibs/go-re2/internal/wasm"
)

// stackPaint is a sentinel byte written across a child's stack scratch area so we
// can detect how far down the stack pointer traveled (high-water mark).
const stackPaint = 0xAA

// tlsSkip is the low portion of the region we avoid painting so we don't clobber
// the ~132-byte thread-local/pthread block at the bottom.
const tlsSkip = 256

// paintChildStack fills a child region's stack area (above the TLS block) with
// the sentinel.
func paintChildStack(base uint32) {
	buf := *hostMemory.Slice()
	for i := base + tlsSkip; i < base+childRegionBytes; i++ {
		buf[i] = stackPaint
	}
}

// childStackHighWater returns the number of stack bytes touched below the top of
// the region, and whether the paint area was fully consumed (a true overflow risk).
func childStackHighWater(base uint32) (peak uint32, exhausted bool) {
	buf := *hostMemory.Slice()
	top := base + childRegionBytes
	low := base + tlsSkip
	if buf[low] != stackPaint {
		return childRegionBytes - tlsSkip, true
	}
	for a := base + tlsSkip; a < top; a++ {
		if buf[a] != stackPaint {
			break
		}
		low = a
	}
	return top - low, false
}

// Equivalent to:
//
//	re := regexp.MustCompile(pattern)
//	if input != "" {
//		re.FindStringSubmatchIndex(input)
//	}
func run(m *wasm2go.Module, pattern, input string) bool {
	opt := m.Xcre2_opt_new()
	defer m.Xcre2_opt_delete(opt)
	m.Xcre2_opt_set_max_mem(opt, int64(maxSize))
	pp := m.Xmalloc(int32(len(pattern)))
	defer m.Xfree(pp)
	hostMemory.WriteString(uint32(pp), pattern)
	re := m.Xcre2_new(pp, int32(len(pattern)), opt)
	defer m.Xcre2_delete(re)
	if m.Xcre2_error_code(re) != 0 {
		return false
	}
	if input != "" {
		ip := m.Xmalloc(int32(len(input)))
		defer m.Xfree(ip)
		hostMemory.WriteString(uint32(ip), input)
		ng := m.Xcre2_num_capturing_groups(re) + 1
		ma := m.Xmalloc(ng * 8)
		defer m.Xfree(ma)
		m.Xcre2_match(re, ip, int32(len(input)), 0, int32(len(input)), 0, ma, ng)
	}
	return true
}

// TestChildStackWithinBudget guards the assumption behind the reduced per-child
// stack reservation, if upstream RE2 somehow changes to use much more stack than
// currently, this should find it.
func TestChildStackWithinBudget(t *testing.T) {
	modPoolOnce.Do(func() { initWASM() })

	stackBudget := childRegionBytes - tlsSkip
	limit := stackBudget / 2 // Fail if use more than 50%

	bigInput := strings.Repeat("abc12345 ", 100000)
	nest := func(n int) string { return strings.Repeat("(", n) + `\w` + strings.Repeat(")", n) }

	cases := []struct{ name, pattern, input string }{
		{"simple", `\d+`, "abc12345"},
		{"big_input", `\d+`, bigInput},
		{"wide_alternation", strings.Repeat("a|", 2000) + "a", "aaaa"},
		{"many_groups", strings.Repeat("(a)", 1000), "aaa"},
		{"named_groups", `(?P<y>\d{4})-(?P<m>\d{2})-(?P<d>\d{2})`, "2026-07-01"},
		// Deep-nesting cases are compile-only: a match would allocate one submatch
		// slot per group (heap, not stack) and isn't what this test guards.
		{"nest_1k", nest(1000), ""},
		{"nest_100k", nest(100000), ""},
		{"nest_near_border", nest(999999), ""},
	}

	for _, tc := range cases {
		child := createChildModule(rootMod)
		paintChildStack(child.tlsBasePtr)
		if !run(child.mod, tc.pattern, tc.input) && !strings.HasPrefix(tc.name, "nest") {
			t.Errorf("%s: expected pattern to compile", tc.name)
		}
		peak, exhausted := childStackHighWater(child.tlsBasePtr)
		if exhausted {
			t.Errorf("%s: stack paint fully consumed; peak >= %d bytes (budget %d)", tc.name, peak, stackBudget)
		}
		if peak > limit {
			t.Errorf("%s: peak stack %d bytes exceeds safety limit %d (budget %d); reduce risk or raise defaultChildStackBytes",
				tc.name, peak, limit, stackBudget)
		}
	}
}
