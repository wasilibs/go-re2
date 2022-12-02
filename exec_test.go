package re2

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	_ "embed"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

//go:embed testdata/re2-search.txt
var re2SearchTestdata []byte

// TestRE2 tests this package's regexp API against test cases
// considered during RE2's exhaustive tests, which run all possible
// regexps over a given set of atoms and operators, up to a given
// complexity, over all possible strings over a given alphabet,
// up to a given size. Rather than try to link with RE2, we read a
// log file containing the test cases and the expected matches.
// The log file, re2-exhaustive.txt, is generated by running 'make log'
// in the open source RE2 distribution https://github.com/google/re2/.
//
// The test file format is a sequence of stanzas like:
//
//	strings
//	"abc"
//	"123x"
//	regexps
//	"[a-z]+"
//	0-3;0-3
//	-;-
//	"([0-9])([0-9])([0-9])"
//	-;-
//	-;0-3 0-1 1-2 2-3
//
// The stanza begins by defining a set of strings, quoted
// using Go double-quote syntax, one per line. Then the
// regexps section gives a sequence of regexps to run on
// the strings. In the block that follows a regexp, each line
// gives the semicolon-separated match results of running
// the regexp on the corresponding string.
// Each match result is either a single -, meaning no match, or a
// space-separated sequence of pairs giving the match and
// submatch indices. An unmatched subexpression formats
// its pair as a single - (not illustrated above).  For now
// each regexp run produces two match results, one for a
// “full match” that restricts the regexp to matching the entire
// string or nothing, and one for a “partial match” that gives
// the leftmost first match found in the string.
//
// Lines beginning with # are comments. Lines beginning with
// a capital letter are test names printed during RE2's test suite
// and are echoed into t but otherwise ignored.
//
// At time of writing, re2-exhaustive.txt is 59 MB but compresses to 385 kB,
// so we store re2-exhaustive.txt.bz2 in the repository and decompress it on the fly.
func TestRE2Search(t *testing.T) {
	testRE2(t, "testdata/re2-search.txt", re2SearchTestdata)
}

func testRE2(t *testing.T, file string, content []byte) {
	var txt io.Reader
	if strings.HasSuffix(file, ".bz2") {
		z := bzip2.NewReader(bytes.NewReader(content))
		txt = z
		file = file[:len(file)-len(".bz2")] // for error messages
	} else {
		txt = bytes.NewReader(content)
	}
	lineno := 0
	scanner := bufio.NewScanner(txt)
	var (
		str           []string
		input         []string
		inStrings     bool
		re            *Regexp
		reLongest     *Regexp
		refull        *Regexp
		refullLongest *Regexp
		nfail         int
		ncase         int
	)
	for lineno := 1; scanner.Scan(); lineno++ {
		line := scanner.Text()
		switch {
		case line == "":
			t.Fatalf("%s:%d: unexpected blank line", file, lineno)
		case line[0] == '#':
			continue
		case 'A' <= line[0] && line[0] <= 'Z':
			// Test name.
			t.Logf("%s\n", line)
			continue
		case line == "strings":
			str = str[:0]
			inStrings = true
		case line == "regexps":
			inStrings = false
		case line[0] == '"':
			q, err := strconv.Unquote(line)
			if err != nil {
				// Fatal because we'll get out of sync.
				t.Fatalf("%s:%d: unquote %s: %v", file, lineno, line, err)
			}
			if inStrings {
				str = append(str, q)
				continue
			}
			// Is a regexp.
			if len(input) != 0 {
				t.Fatalf("%s:%d: out of sync: have %d strings left before %#q", file, lineno, len(input), q)
			}
			re, err = tryCompile(q)
			if err != nil {
				if err.Error() == "error parsing regexp: invalid escape sequence: `\\C`" {
					// We don't and likely never will support \C; keep going.
					continue
				}
				t.Errorf("%s:%d: compile %#q: %v", file, lineno, q, err)
				if nfail++; nfail >= 100 {
					t.Fatalf("stopping after %d errors", nfail)
				}
				continue
			}
			reLongest = re.Copy()
			reLongest.Longest()
			full := `\A(?:` + q + `)\z`
			refull, err = tryCompile(full)
			refullLongest = refull.Copy()
			refullLongest.Longest()
			if err != nil {
				// Fatal because q worked, so this should always work.
				t.Fatalf("%s:%d: compile full %#q: %v", file, lineno, full, err)
			}
			input = str
		case line[0] == '-' || '0' <= line[0] && line[0] <= '9':
			// A sequence of match results.
			ncase++
			if re == nil {
				// Failed to compile: skip results.
				continue
			}
			if len(input) == 0 {
				t.Fatalf("%s:%d: out of sync: no input remaining", file, lineno)
			}
			var text string
			text, input = input[0], input[1:]
			if !isSingleBytes(text) && strings.Contains(re.String(), `\B`) {
				// RE2's \B considers every byte position,
				// so it sees 'not word boundary' in the
				// middle of UTF-8 sequences. This package
				// only considers the positions between runes,
				// so it disagrees. Skip those cases.
				continue
			}
			res := strings.Split(line, ";")
			if len(res) != len(run) {
				t.Fatalf("%s:%d: have %d test results, want %d", file, lineno, len(res), len(run))
			}
			for i := range res {
				have, suffix := run[i](re, reLongest, refull, refullLongest, text)
				want := parseResult(t, file, lineno, res[i])
				if !same(have, want) {
					t.Errorf("%s:%d: %#q%s.FindSubmatchIndex(%#q) = %v, want %v", file, lineno, re, suffix, text, have, want)
					if nfail++; nfail >= 100 {
						t.Fatalf("stopping after %d errors", nfail)
					}
					continue
				}
				b, suffix := matchCases[i](re, reLongest, refull, refullLongest, text)
				if b != (want != nil) {
					t.Errorf("%s:%d: %#q%s.MatchString(%#q) = %v, want %v", file, lineno, re, suffix, text, b, !b)
					if nfail++; nfail >= 100 {
						t.Fatalf("stopping after %d errors", nfail)
					}
					continue
				}
			}

		default:
			t.Fatalf("%s:%d: out of sync: %s\n", file, lineno, line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("%s:%d: %v", file, lineno, err)
	}
	if len(input) != 0 {
		t.Fatalf("%s:%d: out of sync: have %d strings left at EOF", file, lineno, len(input))
	}
	t.Logf("%d cases tested", ncase)
}

var run = []func(*Regexp, *Regexp, *Regexp, *Regexp, string) ([]int, string){
	runFull,
	runPartial,
	runFullLongest,
	runPartialLongest,
}

func runFull(re, reLongest, refull, refullLongest *Regexp, text string) ([]int, string) {
	return refull.FindStringSubmatchIndex(text), "[full]"
}

func runPartial(re, reLongest, refull, refullLongest *Regexp, text string) ([]int, string) {
	return re.FindStringSubmatchIndex(text), ""
}

func runFullLongest(re, reLongest, refull, refullLongest *Regexp, text string) ([]int, string) {
	return refullLongest.FindStringSubmatchIndex(text), "[full,longest]"
}

func runPartialLongest(re, reLongest, refull, refullLongest *Regexp, text string) ([]int, string) {
	return reLongest.FindStringSubmatchIndex(text), "[longest]"
}

var matchCases = []func(*Regexp, *Regexp, *Regexp, *Regexp, string) (bool, string){
	matchFull,
	matchPartial,
	matchFullLongest,
	matchPartialLongest,
}

func matchFull(re, reLongest, refull, refullLongest *Regexp, text string) (bool, string) {
	return refull.MatchString(text), "[full]"
}

func matchPartial(re, reLongest, refull, refullLongest *Regexp, text string) (bool, string) {
	return re.MatchString(text), ""
}

func matchFullLongest(re, reLongest, refull, refullLongest *Regexp, text string) (bool, string) {
	return refullLongest.MatchString(text), "[full,longest]"
}

func matchPartialLongest(re, reLongest, refull, refullLongest *Regexp, text string) (bool, string) {
	return reLongest.MatchString(text), "[longest]"
}

func isSingleBytes(s string) bool {
	for _, c := range s {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func tryCompile(s string) (re *Regexp, err error) {
	// Protect against panic during Compile.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return Compile(s)
}

func parseResult(t *testing.T, file string, lineno int, res string) []int {
	// A single - indicates no match.
	if res == "-" {
		return nil
	}
	// Otherwise, a space-separated list of pairs.
	n := 1
	for j := 0; j < len(res); j++ {
		if res[j] == ' ' {
			n++
		}
	}
	out := make([]int, 2*n)
	i := 0
	n = 0
	for j := 0; j <= len(res); j++ {
		if j == len(res) || res[j] == ' ' {
			// Process a single pair.  - means no submatch.
			pair := res[i:j]
			if pair == "-" {
				out[n] = -1
				out[n+1] = -1
			} else {
				loStr, hiStr, _ := strings.Cut(pair, "-")
				lo, err1 := strconv.Atoi(loStr)
				hi, err2 := strconv.Atoi(hiStr)
				if err1 != nil || err2 != nil || lo > hi {
					t.Fatalf("%s:%d: invalid pair %s", file, lineno, pair)
				}
				out[n] = lo
				out[n+1] = hi
			}
			n += 2
			i = j + 1
		}
	}
	return out
}

func same(x, y []int) bool {
	if len(x) != len(y) {
		return false
	}
	for i, xi := range x {
		if xi != y[i] {
			return false
		}
	}
	return true
}

var text []byte

func makeText(n int) []byte {
	if len(text) >= n {
		return text[:n]
	}
	text = make([]byte, n)
	x := ^uint32(0)
	for i := range text {
		x += x
		x ^= 1
		if int32(x) < 0 {
			x ^= 0x88888eef
		}
		if x%31 == 0 {
			text[i] = '\n'
		} else {
			text[i] = byte(x%(0x7E+1-0x20) + 0x20)
		}
	}
	return text
}