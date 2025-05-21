package experimental

import (
	"github.com/wasilibs/go-re2"
	"github.com/wasilibs/go-re2/internal"
)

// CompileLatin1 is like regexp.Compile but causes the matching to treat
// the input as arbitrary bytes rather than unicode strings. This is
// similar behavior to the rsc.io/binaryregexp package.
func CompileLatin1(expr string) (*re2.Regexp, error) {
	return internal.Compile(expr, internal.CompileOptions{Latin1: true}) //nolint:wrapcheck // just a method forwarder
}

// MustCompileLatin1 is like CompileLatin1 but panics if the expression cannot be parsed.
// It simplifies safe initialization of global variables holding compiled regular
// expressions.
func MustCompileLatin1(str string) *re2.Regexp {
	regexp, err := CompileLatin1(str)
	if err != nil {
		panic(`regexp: CompileLatin1(` + internal.QuoteForError(str) + `): ` + err.Error())
	}
	return regexp
}

// Set is a compiled collection of regular expressions that can be searched for simultaneously.
type Set = internal.Set

// CompileSet compiles the set of regular expression in preparation for matching.
func CompileSet(exprs []string) (*Set, error) {
	return internal.CompileSet(exprs, internal.CompileOptions{}) //nolint:wrapcheck // just a method forwarder
}
