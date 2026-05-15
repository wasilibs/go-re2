package re2

import (
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkParallelMatch measures throughput of a shared *Regexp under
// increasing levels of goroutine concurrency. Each MatchString call pulls
// a child wasm module from the shared pool; with short inputs the pool's
// Get/Put cost is a meaningful fraction of per-op time, so this exposes
// pool contention as worker count grows past GOMAXPROCS.
func BenchmarkParallelMatch(b *testing.B) {
	re := MustCompileBenchmark(`\d`)
	input := "abc7def"

	nproc := runtime.GOMAXPROCS(0)
	for _, workers := range []int{1, nproc, nproc * 4, nproc * 25} {
		b.Run("workers="+strconv.Itoa(workers), func(b *testing.B) {
			var counter atomic.Int64
			target := int64(b.N)
			var wg sync.WaitGroup
			wg.Add(workers)
			b.ResetTimer()
			for range workers {
				go func() {
					defer wg.Done()
					for counter.Add(1) <= target {
						re.MatchString(input)
					}
				}()
			}
			wg.Wait()
		})
	}
}
