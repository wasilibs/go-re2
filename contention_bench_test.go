package re2

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkParallelSearch measures search throughput on 1 KB inputs as
// the number of goroutines calling into the module grows past GOMAXPROCS.
// The inputs mimic a scanner verifying a candidate match: a window of
// minified JavaScript around an "atob(" that the pattern then rejects.
//
// "same" shares one *Regexp between all goroutines, "distinct" gives each
// goroutine one of 64 different ones. Match runs the same search as
// FindIndex without asking for the match position. Ideally ns/op drops until
// workers == GOMAXPROCS and then stays flat.
//
// Run with -tags re2_bench_stdlib for the regexp baseline.
func BenchmarkParallelSearch(b *testing.B) {
	const pattern = `(?i)atob\(\s*["'][A-Za-z0-9+/=]{%d,}["']\s*\)`
	input := benchJSWindow(1024)

	distinct := make([]interface {
		FindIndex(b []byte) []int
		Match(b []byte) bool
	}, 64)
	for i := range distinct {
		distinct[i] = MustCompileBenchmark(fmt.Sprintf(pattern, 40+i))
	}
	same := distinct[0]
	if same.FindIndex(input) != nil {
		b.Fatal("benchmark input must not match")
	}

	nproc := runtime.GOMAXPROCS(0)
	for _, op := range []string{"FindIndex", "Match"} {
		for _, mode := range []string{"same", "distinct"} {
			for _, workers := range []int{1, nproc, nproc * 4, nproc * 16, nproc * 64} {
				b.Run(op+"/"+mode+"/workers="+strconv.Itoa(workers), func(b *testing.B) {
					var counter atomic.Int64
					target := int64(b.N)
					var wg sync.WaitGroup
					wg.Add(workers)
					b.ResetTimer()
					for w := range workers {
						re := same
						if mode == "distinct" {
							re = distinct[w%len(distinct)]
						}
						go func() {
							defer wg.Done()
							for counter.Add(1) <= target {
								if op == "FindIndex" {
									re.FindIndex(input)
								} else {
									re.Match(input)
								}
							}
						}()
					}
					wg.Wait()
					b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "ops/s")
				})
			}
		}
	}
}

// benchJSWindow returns n bytes of minified JavaScript with an atob( call
// whose argument is too short for the benchmark pattern.
func benchJSWindow(n int) []byte {
	const chunk = `var e=document.createElement("script");e.async=!0;e.src=atob("aHR0cHM6Ly9");` +
		`window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments)}` +
		`gtag("js",new Date);gtag("config","G-XXXXXXX");`
	return []byte(strings.Repeat(chunk, n/len(chunk)+1)[:n])
}
