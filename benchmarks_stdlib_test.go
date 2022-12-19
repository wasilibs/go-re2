//go:build re2_bench_stdlib

package re2

import "regexp"

func MustCompileBenchmark(expr string) *regexp.Regexp {
	return regexp.MustCompile(expr)
}

func CompileBenchmark(expr string) (*regexp.Regexp, error) {
	return regexp.Compile(expr)
}
