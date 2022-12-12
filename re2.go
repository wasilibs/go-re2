package re2

import (
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type Regexp struct {
	ptr uintptr
	// Find methods seem to require the pattern to be enclosed in parentheses, so we keep a second
	// regex for them.
	parensPtr uintptr

	posix   bool
	longest bool

	expr       string
	exprParens string

	subexpNames []string

	abi *libre2ABI

	released bool
}

// MatchString reports whether the string s
// contains any match of the regular expression pattern.
// More complicated queries need to use Compile and the full Regexp interface.
func MatchString(pattern string, s string) (matched bool, err error) {
	re, err := Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

// Match reports whether the byte slice b
// contains any match of the regular expression pattern.
// More complicated queries need to use Compile and the full Regexp interface.
func Match(pattern string, b []byte) (matched bool, err error) {
	re, err := Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.Match(b), nil
}

// Copy returns a new Regexp object copied from re.
// Calling Longest on one copy does not affect another.
//
// Deprecated: In earlier releases, when using a Regexp in multiple goroutines,
// giving each goroutine its own copy helped to avoid lock contention.
// As of Go 1.12, using Copy is no longer necessary to avoid lock contention.
// Copy may still be appropriate if the reason for its use is to make
// two copies with different Longest settings.
func (re *Regexp) Copy() *Regexp {
	// Recompiling is slower than this should be but for a deprecated method it
	// is probably fine. The alternative would be to have reference counting to
	// make sure regex is only deleted when the last reference is gone.
	return MustCompile(re.expr)
}

// Compile parses a regular expression and returns, if successful,
// a Regexp object that can be used to match against text.
//
// When matching against text, the regexp returns a match that
// begins as early as possible in the input (leftmost), and among those
// it chooses the one that a backtracking search would have found first.
// This so-called leftmost-first matching is the same semantics
// that Perl, Python, and other implementations use, although this
// package implements it without the expense of backtracking.
// For POSIX leftmost-longest matching, see CompilePOSIX.
func Compile(expr string) (*Regexp, error) {
	return compile(expr, false, false, false)
}

// CompilePOSIX is like Compile but restricts the regular expression
// to POSIX ERE (egrep) syntax and changes the match semantics to
// leftmost-longest.
//
// That is, when matching against text, the regexp returns a match that
// begins as early as possible in the input (leftmost), and among those
// it chooses a match that is as long as possible.
// This so-called leftmost-longest matching is the same semantics
// that early regular expression implementations used and that POSIX
// specifies.
//
// However, there can be multiple leftmost-longest matches, with different
// submatch choices, and here this package diverges from POSIX.
// Among the possible leftmost-longest matches, this package chooses
// the one that a backtracking search would have found first, while POSIX
// specifies that the match be chosen to maximize the length of the first
// subexpression, then the second, and so on from left to right.
// The POSIX rule is computationally prohibitive and not even well-defined.
// See https://swtch.com/~rsc/regexp/regexp2.html#posix for details.
func CompilePOSIX(expr string) (*Regexp, error) {
	return compile(expr, true, true, false)
}

func compile(expr string, posix bool, longest bool, caseInsensitive bool) (*Regexp, error) {
	abi := newABI()
	abi.startOperation(len(expr) + 2 + 8)
	defer abi.endOperation()

	// Find requires the expression to be wrapped in parentheses to match Go's semantics.
	// We end up compiling two regexes, one with parens and one without. We can pass a single
	// input string if we use the parens version and take a slice of that for the non-parens.
	exprParens := fmt.Sprintf("(%s)", expr)
	csParens := newCString(abi, exprParens)
	cs := cString{ptr: csParens.ptr + 1, length: csParens.length - 2}

	rePtr := newRE(abi, cs, longest, posix, caseInsensitive)
	errCode, errArg := reError(abi, rePtr)
	switch errCode {
	case 0:
	// No error.
	case 1:
		return nil, fmt.Errorf("error parsing regexp: unexpected error: %#q", errArg)
	case 2:
		return nil, fmt.Errorf("error parsing regexp: invalid escape sequence: %#q", errArg)
	case 3:
		return nil, fmt.Errorf("error parsing regexp: bad character class: %#q", errArg)
	case 4:
		return nil, fmt.Errorf("error parsing regexp: invalid character class range: %#q", errArg)
	case 5:
		return nil, fmt.Errorf("error parsing regexp: missing closing ]: %#q", errArg)
	case 6:
		return nil, fmt.Errorf("error parsing regexp: missing closing ): %#q", errArg)
	case 7:
		return nil, fmt.Errorf("error parsing regexp: unexpected ): %#q", errArg)
	case 8:
		return nil, fmt.Errorf("error parsing regexp: trailing backslash at end of expression: %#q", errArg)
	case 9:
		return nil, fmt.Errorf("error parsing regexp: missing argument to repetition operator: %#q", errArg)
	case 10:
		return nil, fmt.Errorf("error parsing regexp: bad repitition argument: %#q", errArg)
	case 11:
		return nil, fmt.Errorf("error parsing regexp: invalid nested repetition operator: %#q", errArg)
	case 12:
		return nil, fmt.Errorf("error parsing regexp: bad perl operator: %#q", errArg)
	case 13:
		return nil, fmt.Errorf("error parsing regexp: invalid UTF-8 in regexp: %#q", errArg)
	case 14:
		return nil, fmt.Errorf("error parsing regexp: bad named capture group: %#q", errArg)
	case 15:
		// TODO(anuraaga): While the unit test passes, it is likely that the actual limit is currently
		// different than regexp.
		return nil, fmt.Errorf("error parsing regexp: expression too large")
	}

	reParensPtr := newRE(abi, csParens, longest, posix, caseInsensitive)

	subexp := subexpNames(abi, rePtr)

	re := &Regexp{
		ptr:         rePtr,
		parensPtr:   reParensPtr,
		posix:       posix,
		longest:     longest,
		expr:        expr,
		exprParens:  exprParens,
		subexpNames: subexp,
		abi:         abi,
	}

	runtime.SetFinalizer(re, (*Regexp).release)

	return re, nil
}

// MustCompile is like Compile but panics if the expression cannot be parsed.
// It simplifies safe initialization of global variables holding compiled regular
// expressions.
func MustCompile(str string) *Regexp {
	re, err := Compile(str)
	if err != nil {
		panic(`regexp: Compile(` + quote(str) + `): ` + err.Error())
	}
	return re
}

// MustCompilePOSIX is like CompilePOSIX but panics if the expression cannot be parsed.
// It simplifies safe initialization of global variables holding compiled regular
// expressions.
func MustCompilePOSIX(str string) *Regexp {
	regexp, err := CompilePOSIX(str)
	if err != nil {
		panic(`regexp: CompilePOSIX(` + quote(str) + `): ` + err.Error())
	}
	return regexp
}

func quote(s string) string {
	if strconv.CanBackquote(s) {
		return "`" + s + "`"
	}
	return strconv.Quote(s)
}

// QuoteMeta returns a string that escapes all regular expression metacharacters
// inside the argument text; the returned string is a regular expression matching
// the literal text.
func QuoteMeta(s string) string {
	return regexp.QuoteMeta(s)
}

// Expand appends template to dst and returns the result; during the
// append, Expand replaces variables in the template with corresponding
// matches drawn from src. The match slice should have been returned by
// FindSubmatchIndex.
//
// In the template, a variable is denoted by a substring of the form
// $name or ${name}, where name is a non-empty sequence of letters,
// digits, and underscores. A purely numeric name like $1 refers to
// the submatch with the corresponding index; other names refer to
// capturing parentheses named with the (?P<name>...) syntax. A
// reference to an out of range or unmatched index or a name that is not
// present in the regular expression is replaced with an empty slice.
//
// In the $name form, name is taken to be as long as possible: $1x is
// equivalent to ${1x}, not ${1}x, and, $10 is equivalent to ${10}, not ${1}0.
//
// To insert a literal $ in the output, use $$ in the template.
func (re *Regexp) Expand(dst []byte, template []byte, src []byte, match []int) []byte {
	return re.expand(dst, string(template), src, "", match)
}

// ExpandString is like Expand but the template and source are strings.
// It appends to and returns a byte slice in order to give the calling
// code control over allocation.
func (re *Regexp) ExpandString(dst []byte, template string, src string, match []int) []byte {
	return re.expand(dst, template, nil, src, match)
}

func (re *Regexp) expand(dst []byte, template string, bsrc []byte, src string, match []int) []byte {
	for len(template) > 0 {
		before, after, ok := strings.Cut(template, "$")
		if !ok {
			break
		}
		dst = append(dst, before...)
		template = after
		if template != "" && template[0] == '$' {
			// Treat $$ as $.
			dst = append(dst, '$')
			template = template[1:]
			continue
		}
		name, num, rest, ok := extract(template)
		if !ok {
			// Malformed; treat $ as raw text.
			dst = append(dst, '$')
			continue
		}
		template = rest
		if num >= 0 {
			if 2*num+1 < len(match) && match[2*num] >= 0 {
				if bsrc != nil {
					dst = append(dst, bsrc[match[2*num]:match[2*num+1]]...)
				} else {
					dst = append(dst, src[match[2*num]:match[2*num+1]]...)
				}
			}
		} else {
			for i, namei := range re.subexpNames {
				if name == namei && 2*i+1 < len(match) && match[2*i] >= 0 {
					if bsrc != nil {
						dst = append(dst, bsrc[match[2*i]:match[2*i+1]]...)
					} else {
						dst = append(dst, src[match[2*i]:match[2*i+1]]...)
					}
					break
				}
			}
		}
	}
	dst = append(dst, template...)
	return dst
}

// Find returns a slice holding the text of the leftmost match in b of the regular expression.
// A return value of nil indicates no match.
func (re *Regexp) Find(b []byte) []byte {
	re.abi.startOperation(len(b) + 8)
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var dstCap [2]int

	dst := re.find(cs, dstCap[:0])
	return matchedBytes(b, dst)
}

// FindIndex returns a two-element slice of integers defining the location of
// the leftmost match in b of the regular expression. The match itself is at
// b[loc[0]:loc[1]].
// A return value of nil indicates no match.
func (re *Regexp) FindIndex(b []byte) (loc []int) {
	re.abi.startOperation(len(b) + 8)
	defer re.abi.endOperation()
	cs := newCStringFromBytes(re.abi, b)

	return re.find(cs, nil)
}

// FindString returns a string holding the text of the leftmost match in s of the regular
// expression. If there is no match, the return value is an empty string,
// but it will also be empty if the regular expression successfully matches
// an empty string. Use FindStringIndex or FindStringSubmatch if it is
// necessary to distinguish these cases.
func (re *Regexp) FindString(s string) string {
	re.abi.startOperation(len(s) + 8)
	defer re.abi.endOperation()
	cs := newCString(re.abi, s)

	var dstCap [2]int

	dst := re.find(cs, dstCap[:0])
	return matchedString(s, dst)
}

// FindStringIndex returns a two-element slice of integers defining the
// location of the leftmost match in s of the regular expression. The match
// itself is at s[loc[0]:loc[1]].
// A return value of nil indicates no match.
func (re *Regexp) FindStringIndex(s string) (loc []int) {
	re.abi.startOperation(len(s) + 8)
	defer re.abi.endOperation()
	cs := newCString(re.abi, s)

	return re.find(cs, nil)
}

func (re *Regexp) find(cs cString, dstCap []int) []int {
	matchArr := newCStringArray(re.abi, 1)

	res := match(re, cs, matchArr.ptr, 1)
	if !res {
		return nil
	}

	return readMatch(re.abi, cs, matchArr.ptr, dstCap)
}

// FindAll is the 'All' version of Find; it returns a slice of all successive
// matches of the expression, as defined by the 'All' description in the
// package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAll(b []byte, n int) [][]byte {
	re.abi.startOperation(len(b) + 16)
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches [][]byte

	re.findAll(cs, n, func(match []int) {
		matches = append(matches, matchedBytes(b, match))
	})

	return matches
}

// FindAllIndex is the 'All' version of FindIndex; it returns a slice of all
// successive matches of the expression, as defined by the 'All' description
// in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllIndex(b []byte, n int) [][]int {
	re.abi.startOperation(len(b) + 16)
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches [][]int

	re.findAll(cs, n, func(match []int) {
		matches = append(matches, append([]int(nil), match...))
	})

	return matches
}

func (re *Regexp) FindAllString(s string, n int) []string {
	re.abi.startOperation(len(s) + 16)
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches []string

	re.findAll(cs, n, func(match []int) {
		matches = append(matches, matchedString(s, match))
	})

	return matches
}

func (re *Regexp) FindAllStringIndex(s string, n int) [][]int {
	re.abi.startOperation(len(s) + 16)
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches [][]int

	re.findAll(cs, n, func(match []int) {
		matches = append(matches, append([]int(nil), match...))
	})

	return matches
}

func (re *Regexp) findAll(cs cString, n int, deliver func(match []int)) {
	var dstCap [2]int

	if n < 0 {
		n = cs.length + 1
	}

	csOrig := cs

	csPtr := newCStringPtr(re.abi, cs)

	matchArr := newCStringArray(re.abi, 1)

	count := 0
	prevMatchEnd := -1
	for i := 0; i < cs.length+1; i++ {
		if !findAndConsume(re, csPtr, matchArr.ptr, 1) {
			break
		}

		matches := readMatch(re.abi, csOrig, matchArr.ptr, dstCap[:0])
		accept := true
		if matches[0] == matches[1] {
			// We've found an empty match.
			if matches[0] == prevMatchEnd {
				// We don't allow an empty match right
				// after a previous match, so ignore it.
				accept = false
			}
		}
		if accept {
			deliver(matches)
			count++
		}
		prevMatchEnd = matches[1]

		if count == n {
			break
		}
	}
}

// FindAllSubmatch is the 'All' version of FindSubmatch; it returns a slice
// of all successive matches of the expression, as defined by the 'All'
// description in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllSubmatch(b []byte, n int) [][][]byte {
	re.abi.startOperation(len(b) + 8*len(re.subexpNames) + 8)
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches [][][]byte

	re.findAllSubmatch(cs, n, func(match [][]int) {
		matched := make([][]byte, len(match))
		for i, m := range match {
			matched[i] = matchedBytes(b, m)
		}
		matches = append(matches, matched)
	})

	return matches
}

// FindAllSubmatchIndex is the 'All' version of FindSubmatchIndex; it returns
// a slice of all successive matches of the expression, as defined by the
// 'All' description in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllSubmatchIndex(b []byte, n int) [][]int {
	re.abi.startOperation(len(b) + 8*len(re.subexpNames) + 8)
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches [][]int

	re.findAllSubmatch(cs, n, func(match [][]int) {
		var flat []int
		for _, m := range match {
			flat = append(flat, m...)
		}
		matches = append(matches, flat)
	})

	return matches
}

// FindAllStringSubmatch is the 'All' version of FindStringSubmatch; it
// returns a slice of all successive matches of the expression, as defined by
// the 'All' description in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllStringSubmatch(s string, n int) [][]string {
	re.abi.startOperation(len(s) + 8*len(re.subexpNames) + 8)
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches [][]string

	re.findAllSubmatch(cs, n, func(match [][]int) {
		matched := make([]string, len(match))
		for i, m := range match {
			matched[i] = matchedString(s, m)
		}
		matches = append(matches, matched)
	})

	return matches
}

// FindAllStringSubmatchIndex is the 'All' version of
// FindStringSubmatchIndex; it returns a slice of all successive matches of
// the expression, as defined by the 'All' description in the package
// comment.
// A return value of nil indicates no match.
func (re *Regexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	re.abi.startOperation(len(s) + 8*len(re.subexpNames) + 8)
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches [][]int

	re.findAllSubmatch(cs, n, func(match [][]int) {
		var flat []int
		for _, m := range match {
			flat = append(flat, m...)
		}
		matches = append(matches, flat)
	})

	return matches
}

func (re *Regexp) findAllSubmatch(cs cString, n int, deliver func(match [][]int)) {
	if n < 0 {
		n = cs.length + 1
	}

	csOrig := cs

	csPtr := newCStringPtr(re.abi, cs)

	numGroups := len(re.subexpNames)
	matchArr := newCStringArray(re.abi, numGroups)

	count := 0
	prevMatchEnd := -1
	for i := 0; i < cs.length+1; i++ {
		if !findAndConsume(re, csPtr, matchArr.ptr, uint32(numGroups)) {
			break
		}

		var matches [][]int
		accept := true
		readMatches(re.abi, csOrig, matchArr.ptr, numGroups, func(match []int) {
			if len(matches) == 0 {
				// First match, check if it's an empty match following a match, which we ignore.
				// TODO: Don't iterate further when ignoring.
				if match[0] == match[1] && match[0] == prevMatchEnd {
					accept = false
				}
				prevMatchEnd = match[1]
			}
			matches = append(matches, append([]int(nil), match...))
		})
		if accept {
			deliver(matches)
		}
		count++

		if count == n {
			break
		}
	}
}

// FindSubmatch returns a slice of slices holding the text of the leftmost
// match of the regular expression in b and the matches, if any, of its
// subexpressions, as defined by the 'Submatch' descriptions in the package
// comment.
// A return value of nil indicates no match.
func (re *Regexp) FindSubmatch(b []byte) [][]byte {
	re.abi.startOperation(len(b) + 8*len(re.subexpNames))
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches [][]byte

	re.findSubmatch(cs, func(match []int) {
		matches = append(matches, matchedBytes(b, match))
	})

	return matches
}

// FindSubmatchIndex returns a slice holding the index pairs identifying the
// leftmost match of the regular expression in b and the matches, if any, of
// its subexpressions, as defined by the 'Submatch' and 'Index' descriptions
// in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindSubmatchIndex(b []byte) []int {
	re.abi.startOperation(len(b) + 8*len(re.subexpNames))
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, b)

	var matches []int

	re.findSubmatch(cs, func(match []int) {
		matches = append(matches, match...)
	})

	return matches
}

func (re *Regexp) FindStringSubmatch(s string) []string {
	re.abi.startOperation(len(s) + 8*len(re.subexpNames))
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches []string

	re.findSubmatch(cs, func(match []int) {
		matches = append(matches, matchedString(s, match))
	})

	return matches
}

// FindStringSubmatchIndex returns a slice holding the index pairs
// identifying the leftmost match of the regular expression in s and the
// matches, if any, of its subexpressions, as defined by the 'Submatch' and
// 'Index' descriptions in the package comment.
// A return value of nil indicates no match.
func (re *Regexp) FindStringSubmatchIndex(s string) []int {
	re.abi.startOperation(len(s) + 8*len(re.subexpNames))
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)

	var matches []int

	re.findSubmatch(cs, func(match []int) {
		matches = append(matches, match...)
	})

	return matches
}

func (re *Regexp) findSubmatch(cs cString, deliver func(match []int)) {
	numGroups := len(re.subexpNames)
	matchArr := newCStringArray(re.abi, numGroups)

	if !match(re, cs, matchArr.ptr, uint32(numGroups)) {
		return
	}

	readMatches(re.abi, cs, matchArr.ptr, numGroups, deliver)
}

// Longest makes future searches prefer the leftmost-longest match.
// That is, when matching against text, the regexp returns a match that
// begins as early as possible in the input (leftmost), and among those
// it chooses a match that is as long as possible.
// This method modifies the Regexp and may not be called concurrently
// with any other methods.
func (re *Regexp) Longest() {
	re.abi.startOperation(len(re.expr) + 2)
	defer re.abi.endOperation()

	if re.longest {
		return
	}

	// longest is not a mutable option in re2 so we must release and recompile.
	deleteRE(re.abi, re.ptr)
	deleteRE(re.abi, re.parensPtr)

	csParens := newCString(re.abi, re.exprParens)
	cs := cString{ptr: csParens.ptr + 1, length: csParens.length - 2}
	re.ptr = newRE(re.abi, cs, true, re.posix, false)
	re.parensPtr = newRE(re.abi, csParens, true, re.posix, false)
}

// NumSubexp returns the number of parenthesized subexpressions in this Regexp.
func (re *Regexp) NumSubexp() int {
	return len(re.subexpNames) - 1
}

// Split slices s into substrings separated by the expression and returns a slice of
// the substrings between those expression matches.
//
// The slice returned by this method consists of all the substrings of s
// not contained in the slice returned by FindAllString. When called on an expression
// that contains no metacharacters, it is equivalent to strings.SplitN.
//
// Example:
//
//	s := regexp.MustCompile("a*").Split("abaabaccadaaae", 5)
//	// s: ["", "b", "b", "c", "cadaaae"]
//
// The count determines the number of substrings to return:
//
//	n > 0: at most n substrings; the last substring will be the unsplit remainder.
//	n == 0: the result is nil (zero substrings)
//	n < 0: all substrings
func (re *Regexp) Split(s string, n int) []string {
	// Copied as is from
	// https://github.com/golang/go/blob/78472603c6bac7a52d42d565558b9c0cb12c3f9a/src/regexp/regexp.go#L1253
	// The logic in this function is only for taking match indexes to split the string, regex itself
	// delegates to our implementation.

	if n == 0 {
		return nil
	}

	if len(re.expr) > 0 && len(s) == 0 {
		return []string{""}
	}

	matches := re.FindAllStringIndex(s, n)
	strings := make([]string, 0, len(matches))

	beg := 0
	end := 0
	for _, match := range matches {
		if n > 0 && len(strings) >= n-1 {
			break
		}

		end = match[0]
		if match[1] != 0 {
			strings = append(strings, s[beg:end])
		}
		beg = match[1]
	}

	if end != len(s) {
		strings = append(strings, s[beg:])
	}

	return strings
}

// SubexpNames returns the names of the parenthesized subexpressions
// in this Regexp. The name for the first sub-expression is names[1],
// so that if m is a match slice, the name for m[i] is SubexpNames()[i].
// Since the Regexp as a whole cannot be named, names[0] is always
// the empty string. The slice should not be modified.
func (re *Regexp) SubexpNames() []string {
	return re.subexpNames
}

// SubexpIndex returns the index of the first subexpression with the given name,
// or -1 if there is no subexpression with that name.
//
// Note that multiple subexpressions can be written using the same name, as in
// (?P<bob>a+)(?P<bob>b+), which declares two subexpressions named "bob".
// In this case, SubexpIndex returns the index of the leftmost such subexpression
// in the regular expression.
func (re *Regexp) SubexpIndex(name string) int {
	if name != "" {
		for i, s := range re.subexpNames {
			if name == s {
				return i
			}
		}
	}
	return -1
}

func (re *Regexp) Match(s []byte) bool {
	re.abi.startOperation(len(s))
	defer re.abi.endOperation()

	cs := newCStringFromBytes(re.abi, s)
	res := match(re, cs, 0, 0)
	runtime.KeepAlive(s)
	return res
}

func (re *Regexp) MatchString(s string) bool {
	re.abi.startOperation(len(s))
	defer re.abi.endOperation()

	cs := newCString(re.abi, s)
	res := match(re, cs, 0, 0)
	runtime.KeepAlive(s)
	return res
}

func (re *Regexp) release() {
	re.abi.startOperation(0)
	defer re.abi.endOperation()
	if re.released {
		return
	}
	re.released = true
	release(re)
}

// ReplaceAll returns a copy of src, replacing matches of the Regexp
// with the replacement text repl. Inside repl, $ signs are interpreted as
// in Expand, so for instance $1 represents the text of the first submatch.
func (re *Regexp) ReplaceAll(src, repl []byte) []byte {
	// TODO: See if it's worth not converting repl to string here, the stdlib does it
	// so follow suit for now.
	replRE2 := convertReplacement(string(repl), re.subexpNames)

	re.abi.startOperation(len(src) + len(replRE2) + 16)
	defer re.abi.endOperation()

	srcCS := newCStringFromBytes(re.abi, src)

	res, matched := re.replaceAll(srcCS, replRE2)
	if !matched {
		return src
	}
	return res
}

// ReplaceAllLiteral returns a copy of src, replacing matches of the Regexp
// with the replacement bytes repl. The replacement repl is substituted directly,
// without using Expand.
func (re *Regexp) ReplaceAllLiteral(src, repl []byte) []byte {
	replRE2 := []byte(escapeReplacement(string(repl)))

	re.abi.startOperation(len(src) + len(replRE2) + 16)
	defer re.abi.endOperation()

	srcCS := newCStringFromBytes(re.abi, src)

	res, matched := re.replaceAll(srcCS, replRE2)
	if !matched {
		return src
	}

	return res
}

// ReplaceAllLiteralString returns a copy of src, replacing matches of the Regexp
// with the replacement string repl. The replacement repl is substituted directly,
// without using Expand.
func (re *Regexp) ReplaceAllLiteralString(src, repl string) string {
	replRE2 := []byte(escapeReplacement(repl))

	re.abi.startOperation(len(src) + len(replRE2) + 16)
	defer re.abi.endOperation()

	srcCS := newCString(re.abi, src)

	res, matched := re.replaceAll(srcCS, replRE2)
	if !matched {
		return src
	}

	return string(res)
}

// ReplaceAllString returns a copy of src, replacing matches of the Regexp
// with the replacement string repl. Inside repl, $ signs are interpreted as
// in Expand, so for instance $1 represents the text of the first submatch.
func (re *Regexp) ReplaceAllString(src, repl string) string {
	replRE2 := convertReplacement(repl, re.subexpNames)

	re.abi.startOperation(len(src) + len(replRE2) + 16)
	defer re.abi.endOperation()

	srcCS := newCString(re.abi, src)

	res, matched := re.replaceAll(srcCS, replRE2)
	if !matched {
		return src
	}

	return string(res)
}

func (re *Regexp) replaceAll(srcCS cString, repl []byte) ([]byte, bool) {
	replCS := newCStringFromBytes(re.abi, repl)

	replCSPtr := newCStringPtr(re.abi, replCS)
	srcCSPtr := newCStringPtr(re.abi, srcCS)

	res, matched := globalReplace(re, srcCSPtr.ptr, replCSPtr.ptr)
	if !matched {
		return nil, false
	}
	return res, true
}

// String returns the source text used to compile the regular expression.
func (re *Regexp) String() string {
	return re.expr
}

func subexpNames(abi *libre2ABI, rePtr uintptr) []string {
	// Does not include whole expression match, e.g. $0
	numGroups := numCapturingGroups(abi, rePtr)

	res := make([]string, numGroups+1)

	iter := namedGroupsIter(abi, rePtr)
	defer namedGroupsIterDelete(abi, iter)

	for {
		name, index, ok := namedGroupsIterNext(abi, iter)
		if !ok {
			break
		}
		res[index] = name
	}

	return res
}

// Copied from
// https://github.com/golang/go/blob/0fd7be7ee5f36215b5d6b8f23f35d60bf749805a/src/regexp/regexp.go#L932
// except expansion from regex results is replaced with conversion to re2 replacement syntax.
func convertReplacement(template string, subexpNames []string) []byte {
	var dst []byte

	template = escapeReplacement(template)

	for len(template) > 0 {
		before, after, ok := strings.Cut(template, "$")
		if !ok {
			break
		}
		dst = append(dst, before...)
		template = after
		if template != "" && template[0] == '$' {
			// Treat $$ as $.
			dst = append(dst, '$')
			template = template[1:]
			continue
		}
		name, num, rest, ok := extract(template)
		if !ok {
			// Malformed; treat $ as raw text.
			dst = append(dst, '$')
			continue
		}
		template = rest
		if num < 0 {
			// Named group. We convert it to its corresponding numbered group. If the same name
			// is present multiple times, we concatenate all the numbered groups - this means
			// if one matches, it will be present while the non-matches will be empty. This works
			// because it is invalid for a regex to have the same name in multiple groups that
			// can match at the same time.
			for i, s := range subexpNames {
				if s != "" && name == s {
					dst = append(dst, '\\')
					dst = strconv.AppendUint(dst, uint64(i), 10)
				}
			}
			continue
		}
		if num >= len(subexpNames) {
			// Not present numbered group, drop it.
			continue
		}
		dst = append(dst, '\\')
		dst = strconv.AppendUint(dst, uint64(num), 10)
	}
	dst = append(dst, template...)
	return dst
}

// extract returns the name from a leading "name" or "{name}" in str.
// (The $ has already been removed by the caller.)
// If it is a number, extract returns num set to that number; otherwise num = -1.
// Copied as is from
// https://github.com/golang/go/blob/0fd7be7ee5f36215b5d6b8f23f35d60bf749805a/src/regexp/regexp.go#L981
func extract(str string) (name string, num int, rest string, ok bool) {
	if str == "" {
		return
	}
	brace := false
	if str[0] == '{' {
		brace = true
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		rune, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(rune) && !unicode.IsDigit(rune) && rune != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		// empty name is not okay
		return
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			// missing closing brace
			return
		}
		i++
	}

	// Parse number.
	num = 0
	for i := 0; i < len(name); i++ {
		if name[i] < '0' || '9' < name[i] || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[i]) - '0'
	}
	// Disallow leading zeros.
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}

	rest = str[i:]
	ok = true
	return
}

func escapeReplacement(repl string) string {
	return strings.ReplaceAll(repl, `\`, `\\`)
}

func matchedBytes(s []byte, match []int) []byte {
	if match == nil || match[0] == -1 {
		return nil
	}
	return s[match[0]:match[1]:match[1]]
}

func matchedString(s string, match []int) string {
	if match == nil || match[0] == -1 {
		return ""
	}
	return s[match[0]:match[1]]
}
