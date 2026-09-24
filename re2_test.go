package re2

// Tests for behavior that the tests copied from the regexp package do not
// cover.

import (
	"reflect"
	"regexp"
	"testing"
)

// GAP: Upstream does not test with a nil byte slice, which can cause issues
// with cgo.
func TestNilBytes(t *testing.T) {
	for _, pat := range []string{``, `a*`, `(a)?`, `^`, `$`, `x|`, `a+`} {
		re := MustCompile(pat)
		std := regexp.MustCompile(pat)
		check := func(name string, got, want any) {
			t.Helper()
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%#q: %s(nil) = %v; want %v", pat, name, got, want)
			}
		}
		check("Match", re.Match(nil), std.Match(nil))
		check("Find", re.Find(nil), std.Find(nil))
		check("FindIndex", re.FindIndex(nil), std.FindIndex(nil))
		check("FindSubmatch", re.FindSubmatch(nil), std.FindSubmatch(nil))
		check("FindSubmatchIndex", re.FindSubmatchIndex(nil), std.FindSubmatchIndex(nil))
		check("FindAll", re.FindAll(nil, -1), std.FindAll(nil, -1))
		check("FindAllIndex", re.FindAllIndex(nil, -1), std.FindAllIndex(nil, -1))
		check("FindAllSubmatch", re.FindAllSubmatch(nil, -1), std.FindAllSubmatch(nil, -1))
		check("FindAllSubmatchIndex", re.FindAllSubmatchIndex(nil, -1), std.FindAllSubmatchIndex(nil, -1))
		check("ReplaceAll", string(re.ReplaceAll(nil, []byte("X"))), string(std.ReplaceAll(nil, []byte("X"))))
		check("ReplaceAllLiteral", string(re.ReplaceAllLiteral(nil, []byte("X"))), string(std.ReplaceAllLiteral(nil, []byte("X"))))
	}
}

// RE2's POSIX syntax always allows negated classes to match newline, but
// regexp.CompilePOSIX does not. The upstream POSIX tests set syntax.ClassNL,
// so they do not cover the default.
func TestCompilePOSIXNewline(t *testing.T) {
	for _, pat := range []string{`[^a]+`, `[^[:alpha:]]`, `[[:^alpha:]]`, `.`, `^a$`, `a\nb`, `[[:space:]]`} {
		re := MustCompilePOSIX(pat)
		std := regexp.MustCompilePOSIX(pat)
		if re.String() != pat {
			t.Errorf("%#q: String() = %#q", pat, re.String())
		}
		for _, text := range []string{"b\nc", "x\na\n", "a\nb"} {
			got := re.FindAllStringSubmatchIndex(text, -1)
			want := std.FindAllStringSubmatchIndex(text, -1)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%#q: FindAllStringSubmatchIndex(%q) = %v; want %v", pat, text, got, want)
			}
		}
	}
}

// Upstream only tests limits with patterns that cannot match empty. Empty
// matches abutting a preceding match are skipped and must not count against
// the limit.
func TestFindAllLimitSkipsEmptyMatches(t *testing.T) {
	re := MustCompile(`a*`)
	std := regexp.MustCompile(`a*`)
	const text = "baaab"
	textBytes := []byte(text)
	for n := 1; n <= 4; n++ {
		check := func(name string, got, want any) {
			t.Helper()
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s(%q, %d) = %v; want %v", name, text, n, got, want)
			}
		}
		check("FindAll", re.FindAll(textBytes, n), std.FindAll(textBytes, n))
		check("FindAllIndex", re.FindAllIndex(textBytes, n), std.FindAllIndex(textBytes, n))
		check("FindAllString", re.FindAllString(text, n), std.FindAllString(text, n))
		check("FindAllStringIndex", re.FindAllStringIndex(text, n), std.FindAllStringIndex(text, n))
		check("FindAllSubmatch", re.FindAllSubmatch(textBytes, n), std.FindAllSubmatch(textBytes, n))
		check("FindAllSubmatchIndex", re.FindAllSubmatchIndex(textBytes, n), std.FindAllSubmatchIndex(textBytes, n))
		check("FindAllStringSubmatch", re.FindAllStringSubmatch(text, n), std.FindAllStringSubmatch(text, n))
		check("FindAllStringSubmatchIndex", re.FindAllStringSubmatchIndex(text, n), std.FindAllStringSubmatchIndex(text, n))
	}
}

// GAP: RE2 searches byte by byte and treats the position between two UTF-8
// continuation bytes as not a word boundary, so \B matches inside a multibyte
// character. regexp only reports positions between runes. The upstream RE2
// search tests skip \B on non-ASCII text for the same reason.
func TestNonWordBoundaryInsideRune(t *testing.T) {
	re := MustCompile(`\B`)

	// regexp returns [[4 4]].
	if got, want := re.FindAllStringIndex("b日", -1), [][]int{{2, 2}, {3, 3}, {4, 4}}; !reflect.DeepEqual(got, want) {
		t.Errorf("FindAllStringIndex(%q) = %v; want %v", "b日", got, want)
	}
	// regexp returns ["b日"].
	if got, want := re.Split("b日", -1), []string{"b\xe6", "\x97", "\xa5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Split(%q) = %q; want %q", "b日", got, want)
	}
	// regexp returns false, the only positions between runes are word
	// boundaries.
	if !re.MatchString("1ſA") {
		t.Errorf("MatchString(%q) = false; want true", "1ſA")
	}
}
