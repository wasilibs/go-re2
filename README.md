# re2-go

re2-go is a drop-in replacement for the standard library [regexp][1] package which uses the C++
[re2][2] library for improved performance with large inputs or complex expressions. By default,
re2 is packaged as a WebAssembly module and accessed with the pure Go runtime, [wazero][3].
This means that it is compatible with any Go application, regardless of availability of cgo.

The library can also be used in a TinyGo application being compiled to WebAssembly.

Note that if your regular expressions or input are small, this library is slower than the
standard library. You will generally "know" if your application requires high performance for
complex regular expressions, for example in security filtering software. If you do not know
your app has such needs, you should turn away now.

## Behavior differences

The library is almost fully compatible with the standard library regexp package, with just a few
behavior differences. These are likely corner cases that don't affect typical applications. It is
best to confirm them before proceeding.

- Invalid utf-8 strings are not supported. The standard library silently replaces invalid utf-8
with the unicode replacement character. This library will stop consuming strings when encountering
invalid utf-8.

- `FindAll` with `\b` by itself can only find word boundary starts but not ends.

- `FindAll` with `^` by itself will match each character of the input instead of just the whole input.
Other expressions that match an empty string at the start of the input will behave similarly.

Continue to use the standard library if your usage would match any of these.

Searching this codebase for `// GAP` will allow finding tests that have been tweaked to demonstrate
behavior differences.

## API differences

All APIs found in `regexp` are available except

- `*Reader`: re2 does not support streaming input
- `*Func`: re2 does not support replacement with callback functions

Note that unlike many packages that wrap C++ libraries, there is no added `Close` type of method.
See the [rationale](./RATIONALE.md) for more d[README.md](README.md)etails.

## Usage

re2-go is a standard Go library package and can be added to a go.mod file. It will work fine in
Go or TinyGo projects.

```
go get github.com/anuraaga/re2-go
```

Because the library is a drop-in replacement for the standard library, an import alias can make
migrating code to use it simple.

```go
import "regexp"
```

can be changed to

```go
import regexp "github.com/anuraaga/re2-go"
```

### cgo

This library also supports opting into using cgo to wrap re2 instead of using WebAssembly. This
requires having re2 installed and available via `pkg-config` on the system. The build tag `re2_cgo`
can be used to enable cgo support.

## Performance

Benchmarks are run against every commit in the [bench][4] workflow. GitHub action runners are highly
virtualized and do not have stable performance across runs, but the relative numbers within a run
should still be somewhat, though not precisely, informative.

### wafbench

wafbench tests the performance of replacing the regex operator of the OWASP [CoreRuleSet][5] and
[Coraza][6] implementation with this library. This benchmark is considered a real world performance
test, with the regular expressions being real ones used in production. Security filtering rules
often have highly complex expressions.

One run looks like this

```
name \ time/op     build/wafbench_stdlib.txt  build/wafbench.txt  build/wafbench_cgo.txt
WAF/FTW-2                         40.5s ± 1%          38.4s ± 2%              36.8s ± 1%
WAF/POST/1-2                     4.57ms ± 7%         5.21ms ± 5%             4.78ms ± 5%
WAF/POST/1000-2                  26.6ms ± 5%          8.1ms ± 2%              7.0ms ± 1%
WAF/POST/10000-2                  239ms ± 2%           31ms ± 4%               23ms ± 5%
WAF/POST/100000-2                 2.38s ± 2%          0.24s ± 4%              0.17s ± 2%
```

`FTW` is the time to run the standard CoreRuleSet test framework. The performance of this
library with WebAssembly, wafbench.txt, shows a slight improvement over the standard library
in this baseline test case.

The FTW test suite will issue many requests with various payloads, generally somewhat small.
The `POST` tests show the same ruleset applied to requests with payload sizes as shown, in bytes.
We see that only with the absolute smallest payload of 1 byte does the standard library perform
a bit better than this library. For any larger size, even a fairly typical 1KB, re2-go
greatly outperforms.

cgo seems to offer about a 30% improvement on WebAssembly in this library. Many apps may accept
the somewhat slower performance in exchange for the build and deployment flexibility of
WebAssembly but either option will work with no changes to the codebase.

### Microbenchmarks

Microbenchmarks are the same as included in the Go standard library. Full results can be
viewed in the workflow, a sample of results for one run looks like this

```
name \ time/op                  build/bench_stdlib.txt  build/bench.txt   build/bench_cgo.txt
Find-2                                      211ns ± 6%        965ns ± 4%            424ns ± 1%
Compile/Onepass-2                          5.21µs ± 0%     445.49µs ± 2%          21.30µs ± 4%
Compile/Medium-2                           11.6µs ± 0%      511.8µs ± 1%           33.1µs ± 3%
Compile/Hard-2                             95.1µs ± 0%     1425.0µs ± 1%          252.4µs ± 5%
Match/Easy0/16-2                           4.33ns ± 0%     440.46ns ± 0%         221.72ns ± 1%
Match/Easy0/32-2                           52.4ns ± 0%      441.3ns ± 1%          221.7ns ± 1%
Match/Easy0/1K-2                            319ns ± 1%        453ns ± 0%            221ns ± 0%
Match/Easy0/32K-2                          5.56µs ± 0%       1.35µs ± 0%           0.24µs ±12%
Match/Easy0/1M-2                            296µs ± 1%         57µs ± 3%              0µs ± 0%
Match/Easy0/32M-2                          9.54ms ± 1%       3.27ms ± 9%           0.00ms ± 1%
Match/Easy0i/16-2                          4.34ns ± 1%     430.82ns ± 0%         225.70ns ± 2%
Match/Easy0i/32-2                           842ns ± 0%        431ns ± 0%            228ns ± 5%
Match/Easy0i/1K-2                          24.9µs ± 0%        0.4µs ± 0%            0.2µs ± 2%
Match/Easy0i/32K-2                         1.13ms ± 0%       0.00ms ± 0%           0.00ms ± 2%
Match/Easy0i/1M-2                          36.3ms ± 0%        0.1ms ± 4%            0.0ms ± 1%
Match/Easy0i/32M-2                          1.16s ± 0%        0.00s ±18%            0.00s ± 2%
Match/Easy1/16-2                           4.36ns ± 1%     429.28ns ± 0%         224.08ns ± 2%
Match/Easy1/32-2                           48.8ns ± 1%      428.5ns ± 0%          223.4ns ± 2%
Match/Easy1/1K-2                            663ns ± 2%        442ns ± 0%            224ns ± 2%
Match/Easy1/32K-2                          33.5µs ± 1%        1.4µs ± 1%            0.2µs ± 1%
Match/Easy1/1M-2                           1.20ms ± 1%       0.06ms ± 2%           0.00ms ± 1%
Match/Easy1/32M-2                          38.7ms ± 1%        3.1ms ± 4%            0.0ms ± 0%
Match/Medium/16-2                          4.34ns ± 0%     429.26ns ± 0%         221.08ns ± 1%
Match/Medium/32-2                           687ns ± 0%        430ns ± 0%            222ns ± 1%
Match/Medium/1K-2                          26.6µs ± 0%        0.4µs ± 0%            0.2µs ± 1%
Match/Medium/32K-2                         1.26ms ± 0%       0.00ms ± 2%           0.00ms ± 1%
Match/Medium/1M-2                          40.2ms ± 1%        0.1ms ± 3%            0.0ms ± 2%
Match/Medium/32M-2                          1.29s ± 0%        0.00s ± 6%            0.00s ± 0%
Match/Hard/16-2                            4.37ns ± 1%     430.50ns ± 0%         222.24ns ± 1%
Match/Hard/32-2                            1.21µs ± 2%       0.43µs ± 0%           0.22µs ± 2%
Match/Hard/1K-2                            39.9µs ±15%        0.4µs ± 0%            0.2µs ± 1%
Match/Hard/32K-2                           1.70ms ± 1%       0.00ms ± 0%           0.00ms ± 2%
Match/Hard/1M-2                            54.4ms ± 1%        0.1ms ± 2%            0.0ms ± 0%
Match/Hard/32M-2                            1.74s ± 1%        0.00s ± 7%            0.00s ±11%
Match/Hard1/16-2                           3.14µs ± 0%       0.54µs ± 0%           0.24µs ± 1%
Match/Hard1/32-2                           6.00µs ± 0%       0.67µs ± 0%           0.28µs ± 2%
Match/Hard1/1K-2                            186µs ± 0%          8µs ± 0%              3µs ± 0%
Match/Hard1/32K-2                          8.30ms ± 3%       0.23ms ± 0%           0.08ms ± 0%
Match/Hard1/1M-2                            266ms ± 3%          7ms ± 0%              3ms ± 0%
Match/Hard1/32M-2                           8.50s ± 2%        0.23s ± 0%            0.09s ± 0%

name \ alloc/op                 build/bench_stdlib.txt  build/bench.txt   build/bench_cgo.txt
Find-2                                      0.00B            72.00B ± 0%           16.00B ± 0%
Compile/Onepass-2                          4.06kB ± 0%     598.70kB ± 0%           0.16kB ± 0%
Compile/Medium-2                           9.42kB ± 0%     598.77kB ± 0%           0.24kB ± 0%
Compile/Hard-2                             84.8kB ± 0%      912.3kB ± 0%            2.4kB ± 0%
```

Most benchmarks are similar to `Find`, testing simple expressions with small input. In all of these,
the standard library performs much better. To reiterate the guidance at the top of this README, if
you only use simple expressions with small input, you should not use this library.

The compilation benchmarks show that re2 is much slower to compile expressions than the standard
library - this is more than just the overhead of foreign function invocation. This likely results
in the improved performance at runtime in other cases. They also show 500KB+ memory usage per
compilation - the resting memory usage per expression seems to be around ~300KB, much higher than
the standard library. There is a significantly more memory usage when using WebAssembly - if this
is not acceptable, setting up the build toolchain for cgo may be worth it.

The match benchmarks show the performance tradeoffs for complexity vs input size. We see the standard
library perform the best with low complexity and size, but for high complexity or high input size,
re2-go with WebAssembly outperforms, often significantly. Notable is `Hard1`, where even on the smallest
size this library outperforms. The expression is `ABCD|CDEF|EFGH|GHIJ|IJKL|KLMN|MNOP|OPQR|QRST|STUV|UVWX|WXYZ`,
a simple OR of literals - re2 has the concept of regex sets and likely is able to optimize this in a
special way. The CoreRuleSet contains many expressions of a form like this - this possibly indicates good
performance in real world use cases.

[1]: https://pkg.go.dev/regexp
[2]: https://github.com/google/re2
[3]: https://wazero.io
[4]: https://github.com/anuraaga/re2-go/actions/workflows/bench.yaml
[5]: https://github.com/coreruleset/coreruleset
[6]: https://github.com/corazawaf/coraza