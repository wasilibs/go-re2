//go:build !re2_bench_stdlib

package re2

func MustCompileBenchmark(expr string) *Regexp {
	return MustCompile(expr)
}

func CompileBenchmark(expr string) (*Regexp, error) {
	return Compile(expr)
}
