package re2

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkParallelSearch measures FindIndex and Match throughput on a 1 KB
// input as the number of goroutines sharing one *Regexp grows past
// GOMAXPROCS. Ideally ops/s rises until workers == GOMAXPROCS and then
// stays flat.
func BenchmarkParallelSearch(b *testing.B) {
	re := MustCompileBenchmark("[XYZ]ABCDEFGHIJKLMNOPQRSTUVWXYZ$")
	input := makeText(1 << 10)

	nproc := runtime.GOMAXPROCS(0)
	for _, op := range []string{"FindIndex", "Match"} {
		for _, workers := range []int{1, nproc, nproc * 4, nproc * 16, nproc * 64} {
			b.Run(op+"/workers="+strconv.Itoa(workers), func(b *testing.B) {
				var counter atomic.Int64
				target := int64(b.N)
				var wg sync.WaitGroup
				wg.Add(workers)
				b.ResetTimer()
				for range workers {
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
