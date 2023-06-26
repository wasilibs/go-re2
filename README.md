# go-re2

go-re2 is a drop-in replacement for the standard library [regexp][1] package which uses the C++
[re2][2] library for improved performance with large inputs or complex expressions. By default,
re2 is packaged as a WebAssembly module and accessed with the pure Go runtime, [wazero][3].
This means that it is compatible with any Go application, regardless of availability of cgo.

The library can also be used in a TinyGo application being compiled to WebAssembly. Currently,
`regexp` when compiled with TinyGo always has very slow performance and sometimes fails to
compile expressions completely.

Note that if your regular expressions or input are small, this library is slower than the
standard library. You will generally "know" if your application requires high performance for
complex regular expressions, for example in security filtering software. If you do not know
your app has such needs and are not using TinyGo, you should turn away now.

## Behavior differences

The library is almost fully compatible with the standard library regexp package, with just a few
behavior differences. These are likely corner cases that don't affect typical applications. It is
best to confirm them before proceeding.

- Invalid utf-8 strings are treated differently. The standard library silently replaces invalid utf-8
with the unicode replacement character. This library will stop consuming strings when encountering
invalid utf-8.
  - `experimental.CompileLatin1` can be used to match against non-utf8 strings

- `reflect.DeepEqual` cannot compare `Regexp` objects.

Continue to use the standard library if your usage would match any of these.

Searching this codebase for `// GAP` will allow finding tests that have been tweaked to demonstrate
behavior differences.

## API differences

All APIs found in `regexp` are available except

- `*Reader`: re2 does not support streaming input
- `*Func`: re2 does not support replacement with callback functions

Note that unlike many packages that wrap C++ libraries, there is no added `Close` type of method.
See the [rationale](./RATIONALE.md) for more details.

### Experimental APIs

The [experimental](./experimental) package contains APIs not part of standard `regexp` that are
incubating. They may in the future be moved to stable packages. The experimental package does not
provide any guarantee of API stability even across minor version updates.

## Usage

go-re2 is a standard Go library package and can be added to a go.mod file. It will work fine in
Go or TinyGo projects.

```
go get github.com/wasilibs/go-re2
```

Because the library is a drop-in replacement for the standard library, an import alias can make
migrating code to use it simple.

```go
import "regexp"
```

can be changed to

```go
import regexp "github.com/wasilibs/go-re2"
```

### cgo

This library also supports opting into using cgo to wrap re2 instead of using WebAssembly. This
requires having re2 installed and available via `pkg-config` on the system. The build tag `re2_cgo`
can be used to enable cgo support.

#### Ubuntu

On Ubuntu install the gcc tool chain and the re2 library as follows:

```bash
sudo apt install build-essential
sudo apt-get install -y libre2-dev
```

#### Windows

On Windows start by installing [MSYS2][8]. Then open the MINGW64 terminal and install the gcc toolchain and re2 via pacman:

```bash
pacman -S mingw-w64-x86_64-gcc
pacman -S mingw-w64-x86_64-re2
```
If you want to run the resulting exe program outside the MINGW64 terminal you need to add a path to the MinGW-w64 libraries to the PATH environmental variable (adjust as needed for your system):

```cmd
SET PATH=C:\msys64\mingw64\bin;%PATH%
```

#### MacOS

On Mac start by installing [homebrew][9] including installation of the command line tools. Then install re2 via brew:

```bash
brew install re2
````

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
WAF/FTW-2                         29.82 ± ∞ ¹    26.38 ± ∞ ¹  -11.55% (p=0.008 n=5)    26.95 ± ∞ ¹   -9.61% (p=0.008 n=5)
WAF/POST/1-2                     3.359m ± ∞ ¹   3.550m ± ∞ ¹   +5.70% (p=0.008 n=5)   3.563m ± ∞ ¹   +6.09% (p=0.008 n=5)
WAF/POST/1000-2                 20.532m ± ∞ ¹   6.194m ± ∞ ¹  -69.83% (p=0.008 n=5)   5.211m ± ∞ ¹  -74.62% (p=0.008 n=5)
WAF/POST/10000-2                187.29m ± ∞ ¹   25.94m ± ∞ ¹  -86.15% (p=0.008 n=5)   17.69m ± ∞ ¹  -90.56% (p=0.008 n=5)
WAF/POST/100000-2               1852.4m ± ∞ ¹   220.2m ± ∞ ¹  -88.11% (p=0.008 n=5)   143.8m ± ∞ ¹  -92.23% (p=0.008 n=5)
```

`FTW` is the time to run the standard CoreRuleSet test framework. The performance of this
library with WebAssembly, wafbench.txt, shows a slight improvement over the standard library
in this baseline test case.

The FTW test suite will issue many requests with various payloads, generally somewhat small.
The `POST` tests show the same ruleset applied to requests with payload sizes as shown, in bytes.
We see that only with the absolute smallest payload of 1 byte does the standard library perform
a bit better than this library. For any larger size, even a fairly typical 1KB, go-re2
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
Compile/Onepass-2                          4.638µ ± ∞ ¹     166.048µ ± ∞ ¹   +3480.16% (p=0.008 n=5)     9.355µ ± ∞ ¹   +101.70% (p=0.008 n=5)
Compile/Medium-2                           10.44µ ± ∞ ¹      202.93µ ± ∞ ¹   +1843.44% (p=0.008 n=5)     15.45µ ± ∞ ¹    +47.98% (p=0.008 n=5)
Compile/Hard-2                             86.80µ ± ∞ ¹      613.84µ ± ∞ ¹    +607.19% (p=0.008 n=5)    149.32µ ± ∞ ¹    +72.03% (p=0.008 n=5)
Match/Easy0/16-2                           4.355n ± ∞ ¹     348.200n ± ∞ ¹   +7895.41% (p=0.008 n=5)   224.400n ± ∞ ¹  +5052.70% (p=0.008 n=5)
Match/Easy0/32-2                           51.06n ± ∞ ¹      348.00n ± ∞ ¹    +581.55% (p=0.008 n=5)    223.20n ± ∞ ¹   +337.13% (p=0.008 n=5)
Match/Easy0/1K-2                           269.7n ± ∞ ¹       362.9n ± ∞ ¹     +34.56% (p=0.008 n=5)     224.8n ± ∞ ¹    -16.65% (p=0.008 n=5)
Match/Easy0/32K-2                         4629.0n ± ∞ ¹      1248.0n ± ∞ ¹     -73.04% (p=0.008 n=5)     223.9n ± ∞ ¹    -95.16% (p=0.008 n=5)
Match/Easy0/1M-2                        265456.0n ± ∞ ¹    189369.0n ± ∞ ¹     -28.66% (p=0.008 n=5)     226.0n ± ∞ ¹    -99.91% (p=0.008 n=5)
Match/Easy0/32M-2                      8870984.0n ± ∞ ¹   9164874.0n ± ∞ ¹      +3.31% (p=0.008 n=5)     227.3n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Easy0i/16-2                          4.358n ± ∞ ¹     331.800n ± ∞ ¹   +7513.58% (p=0.008 n=5)   225.300n ± ∞ ¹  +5069.80% (p=0.008 n=5)
Match/Easy0i/32-2                          896.4n ± ∞ ¹       331.4n ± ∞ ¹     -63.03% (p=0.008 n=5)     228.2n ± ∞ ¹    -74.54% (p=0.008 n=5)
Match/Easy0i/1K-2                        25037.0n ± ∞ ¹       345.7n ± ∞ ¹     -98.62% (p=0.008 n=5)     225.3n ± ∞ ¹    -99.10% (p=0.008 n=5)
Match/Easy0i/32K-2                     1084458.0n ± ∞ ¹      1263.0n ± ∞ ¹     -99.88% (p=0.008 n=5)     227.0n ± ∞ ¹    -99.98% (p=0.008 n=5)
Match/Easy0i/1M-2                     35171104.0n ± ∞ ¹    194931.0n ± ∞ ¹     -99.45% (p=0.008 n=5)     227.0n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Easy0i/32M-2                  1114773475.0n ± ∞ ¹   9466155.0n ± ∞ ¹     -99.15% (p=0.008 n=5)     227.0n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Easy1/16-2                           4.352n ± ∞ ¹     332.800n ± ∞ ¹   +7547.06% (p=0.008 n=5)   227.100n ± ∞ ¹  +5118.29% (p=0.008 n=5)
Match/Easy1/32-2                           46.76n ± ∞ ¹      331.70n ± ∞ ¹    +609.37% (p=0.008 n=5)    227.30n ± ∞ ¹   +386.10% (p=0.008 n=5)
Match/Easy1/1K-2                           704.4n ± ∞ ¹       347.1n ± ∞ ¹     -50.72% (p=0.008 n=5)     225.3n ± ∞ ¹    -68.02% (p=0.008 n=5)
Match/Easy1/32K-2                        31693.0n ± ∞ ¹      1155.0n ± ∞ ¹     -96.36% (p=0.008 n=5)     225.9n ± ∞ ¹    -99.29% (p=0.008 n=5)
Match/Easy1/1M-2                       1109254.0n ± ∞ ¹    195551.0n ± ∞ ¹     -82.37% (p=0.008 n=5)     225.7n ± ∞ ¹    -99.98% (p=0.008 n=5)
Match/Easy1/32M-2                     35197178.0n ± ∞ ¹   9157052.0n ± ∞ ¹     -73.98% (p=0.008 n=5)     226.0n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Medium/16-2                          4.352n ± ∞ ¹     332.000n ± ∞ ¹   +7528.68% (p=0.008 n=5)   228.200n ± ∞ ¹  +5143.57% (p=0.008 n=5)
Match/Medium/32-2                          953.6n ± ∞ ¹       332.7n ± ∞ ¹     -65.11% (p=0.008 n=5)     227.9n ± ∞ ¹    -76.10% (p=0.008 n=5)
Match/Medium/1K-2                        27244.0n ± ∞ ¹       346.0n ± ∞ ¹     -98.73% (p=0.008 n=5)     227.4n ± ∞ ¹    -99.17% (p=0.008 n=5)
Match/Medium/32K-2                     1109573.0n ± ∞ ¹      1179.0n ± ∞ ¹     -99.89% (p=0.008 n=5)     227.2n ± ∞ ¹    -99.98% (p=0.008 n=5)
Match/Medium/1M-2                     35902029.0n ± ∞ ¹    196174.0n ± ∞ ¹     -99.45% (p=0.008 n=5)     225.7n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Medium/32M-2                  1144243544.0n ± ∞ ¹   9527884.0n ± ∞ ¹     -99.17% (p=0.008 n=5)     227.3n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Hard/16-2                            4.352n ± ∞ ¹     331.400n ± ∞ ¹   +7514.89% (p=0.008 n=5)   226.400n ± ∞ ¹  +5102.21% (p=0.008 n=5)
Match/Hard/32-2                           1193.0n ± ∞ ¹       332.2n ± ∞ ¹     -72.15% (p=0.008 n=5)     227.8n ± ∞ ¹    -80.91% (p=0.008 n=5)
Match/Hard/1K-2                          36430.0n ± ∞ ¹       346.1n ± ∞ ¹     -99.05% (p=0.008 n=5)     225.2n ± ∞ ¹    -99.38% (p=0.008 n=5)
Match/Hard/32K-2                       1617883.0n ± ∞ ¹      1151.0n ± ∞ ¹     -99.93% (p=0.008 n=5)     226.4n ± ∞ ¹    -99.99% (p=0.008 n=5)
Match/Hard/1M-2                       51948253.0n ± ∞ ¹    188705.0n ± ∞ ¹     -99.64% (p=0.008 n=5)     225.2n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Hard/32M-2                    1663624407.0n ± ∞ ¹   8832033.0n ± ∞ ¹     -99.47% (p=0.008 n=5)     225.1n ± ∞ ¹   -100.00% (p=0.008 n=5)
Match/Hard1/16-2                          3664.0n ± ∞ ¹       475.6n ± ∞ ¹     -87.02% (p=0.008 n=5)     244.9n ± ∞ ¹    -93.32% (p=0.008 n=5)
Match/Hard1/32-2                          7182.0n ± ∞ ¹       607.6n ± ∞ ¹     -91.54% (p=0.008 n=5)     283.2n ± ∞ ¹    -96.06% (p=0.008 n=5)
Match/Hard1/1K-2                         219.455µ ± ∞ ¹       8.805µ ± ∞ ¹     -95.99% (p=0.008 n=5)     2.376µ ± ∞ ¹    -98.92% (p=0.008 n=5)
Match/Hard1/32K-2                        8018.96µ ± ∞ ¹      259.27µ ± ∞ ¹     -96.77% (p=0.008 n=5)     69.22µ ± ∞ ¹    -99.14% (p=0.008 n=5)
Match/Hard1/1M-2                         276.443m ± ∞ ¹       8.192m ± ∞ ¹     -97.04% (p=0.008 n=5)     2.174m ± ∞ ¹    -99.21% (p=0.008 n=5)
Match/Hard1/32M-2                        8220.78m ± ∞ ¹      263.59m ± ∞ ¹     -96.79% (p=0.008 n=5)     68.09m ± ∞ ¹    -99.17% (p=0.008 n=5)
MatchParallel/Easy0/16-2                   2.354n ± ∞ ¹     362.800n ± ∞ ¹  +15312.06% (p=0.008 n=5)   175.800n ± ∞ ¹  +7368.14% (p=0.008 n=5)
MatchParallel/Easy0/32-2                   25.98n ± ∞ ¹      361.40n ± ∞ ¹   +1291.07% (p=0.008 n=5)    178.00n ± ∞ ¹   +585.14% (p=0.008 n=5)
MatchParallel/Easy0/1K-2                   135.9n ± ∞ ¹       377.6n ± ∞ ¹    +177.85% (p=0.008 n=5)     179.3n ± ∞ ¹    +31.94% (p=0.008 n=5)
MatchParallel/Easy0/32K-2                 2332.0n ± ∞ ¹      1365.0n ± ∞ ¹     -41.47% (p=0.008 n=5)     179.5n ± ∞ ¹    -92.30% (p=0.008 n=5)
MatchParallel/Easy0/1M-2                134198.0n ± ∞ ¹    206358.0n ± ∞ ¹     +53.77% (p=0.008 n=5)     177.3n ± ∞ ¹    -99.87% (p=0.008 n=5)
MatchParallel/Easy0/32M-2              4454942.0n ± ∞ ¹   9385502.0n ± ∞ ¹    +110.68% (p=0.008 n=5)     174.4n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Easy0i/16-2                  2.356n ± ∞ ¹     360.500n ± ∞ ¹  +15201.36% (p=0.008 n=5)   174.800n ± ∞ ¹  +7319.35% (p=0.008 n=5)
MatchParallel/Easy0i/32-2                  475.6n ± ∞ ¹       360.5n ± ∞ ¹     -24.20% (p=0.008 n=5)     171.4n ± ∞ ¹    -63.96% (p=0.008 n=5)
MatchParallel/Easy0i/1K-2                12785.0n ± ∞ ¹       375.8n ± ∞ ¹     -97.06% (p=0.008 n=5)     172.0n ± ∞ ¹    -98.65% (p=0.008 n=5)
MatchParallel/Easy0i/32K-2              545032.0n ± ∞ ¹      1384.0n ± ∞ ¹     -99.75% (p=0.008 n=5)     174.2n ± ∞ ¹    -99.97% (p=0.008 n=5)
MatchParallel/Easy0i/1M-2             17558362.0n ± ∞ ¹    205672.0n ± ∞ ¹     -98.83% (p=0.008 n=5)     175.1n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Easy0i/32M-2          1116543044.0n ± ∞ ¹   9440540.0n ± ∞ ¹     -99.15% (p=0.008 n=5)     178.0n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Easy1/16-2                   2.357n ± ∞ ¹     359.600n ± ∞ ¹  +15156.68% (p=0.008 n=5)   180.600n ± ∞ ¹  +7562.28% (p=0.008 n=5)
MatchParallel/Easy1/32-2                   24.05n ± ∞ ¹      362.50n ± ∞ ¹   +1407.28% (p=0.008 n=5)    177.60n ± ∞ ¹   +638.46% (p=0.008 n=5)
MatchParallel/Easy1/1K-2                   356.5n ± ∞ ¹       376.1n ± ∞ ¹      +5.50% (p=0.008 n=5)     179.1n ± ∞ ¹    -49.76% (p=0.008 n=5)
MatchParallel/Easy1/32K-2                16163.0n ± ∞ ¹      1263.0n ± ∞ ¹     -92.19% (p=0.008 n=5)     175.3n ± ∞ ¹    -98.92% (p=0.008 n=5)
MatchParallel/Easy1/1M-2                554252.0n ± ∞ ¹    203559.0n ± ∞ ¹     -63.27% (p=0.008 n=5)     174.3n ± ∞ ¹    -99.97% (p=0.008 n=5)
MatchParallel/Easy1/32M-2             17759537.0n ± ∞ ¹   9107746.0n ± ∞ ¹     -48.72% (p=0.008 n=5)     171.9n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Medium/16-2                  2.354n ± ∞ ¹     359.900n ± ∞ ¹  +15188.87% (p=0.008 n=5)   176.000n ± ∞ ¹  +7376.64% (p=0.008 n=5)
MatchParallel/Medium/32-2                  480.6n ± ∞ ¹       360.8n ± ∞ ¹     -24.93% (p=0.008 n=5)     174.6n ± ∞ ¹    -63.67% (p=0.008 n=5)
MatchParallel/Medium/1K-2                13832.0n ± ∞ ¹       375.3n ± ∞ ¹     -97.29% (p=0.008 n=5)     176.7n ± ∞ ¹    -98.72% (p=0.008 n=5)
MatchParallel/Medium/32K-2              561633.0n ± ∞ ¹      1272.0n ± ∞ ¹     -99.77% (p=0.008 n=5)     177.2n ± ∞ ¹    -99.97% (p=0.008 n=5)
MatchParallel/Medium/1M-2             18249558.0n ± ∞ ¹    202400.0n ± ∞ ¹     -98.89% (p=0.008 n=5)     178.4n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Medium/32M-2          1144693635.0n ± ∞ ¹   9558875.0n ± ∞ ¹     -99.16% (p=0.008 n=5)     177.6n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Hard/16-2                    2.355n ± ∞ ¹     361.000n ± ∞ ¹  +15229.09% (p=0.008 n=5)   177.000n ± ∞ ¹  +7415.92% (p=0.008 n=5)
MatchParallel/Hard/32-2                    603.3n ± ∞ ¹       361.2n ± ∞ ¹     -40.13% (p=0.008 n=5)     179.4n ± ∞ ¹    -70.26% (p=0.008 n=5)
MatchParallel/Hard/1K-2                  18314.0n ± ∞ ¹       378.0n ± ∞ ¹     -97.94% (p=0.008 n=5)     172.0n ± ∞ ¹    -99.06% (p=0.008 n=5)
MatchParallel/Hard/32K-2                821073.0n ± ∞ ¹      1263.0n ± ∞ ¹     -99.85% (p=0.008 n=5)     175.4n ± ∞ ¹    -99.98% (p=0.008 n=5)
MatchParallel/Hard/1M-2               26636550.0n ± ∞ ¹    197907.0n ± ∞ ¹     -99.26% (p=0.008 n=5)     176.7n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Hard/32M-2            1663251850.0n ± ∞ ¹   8878713.0n ± ∞ ¹     -99.47% (p=0.008 n=5)     178.3n ± ∞ ¹   -100.00% (p=0.008 n=5)
MatchParallel/Hard1/16-2                  1849.0n ± ∞ ¹       512.7n ± ∞ ¹     -72.27% (p=0.008 n=5)     205.6n ± ∞ ¹    -88.88% (p=0.008 n=5)
MatchParallel/Hard1/32-2                  3721.0n ± ∞ ¹       649.6n ± ∞ ¹     -82.54% (p=0.008 n=5)     229.1n ± ∞ ¹    -93.84% (p=0.008 n=5)
MatchParallel/Hard1/1K-2                 110.300µ ± ∞ ¹       9.190µ ± ∞ ¹     -91.67% (p=0.008 n=5)     1.408µ ± ∞ ¹    -98.72% (p=0.008 n=5)
MatchParallel/Hard1/32K-2                4202.71µ ± ∞ ¹      252.89µ ± ∞ ¹     -93.98% (p=0.008 n=5)     39.51µ ± ∞ ¹    -99.06% (p=0.008 n=5)
MatchParallel/Hard1/1M-2                 129.208m ± ∞ ¹       8.189m ± ∞ ¹     -93.66% (p=0.008 n=5)     1.089m ± ∞ ¹    -99.16% (p=0.008 n=5)
MatchParallel/Hard1/32M-2                8224.11m ± ∞ ¹      263.74m ± ∞ ¹     -96.79% (p=0.008 n=5)     34.27m ± ∞ ¹    -99.58% (p=0.008 n=5)

name \ alloc/op                 build/bench_stdlib.txt  build/bench.txt   build/bench_cgo.txt
Find-2                                  0.00 ± ∞ ¹       72.00 ± ∞ ¹          ? (p=0.008 n=5)    16.00 ± ∞ ¹         ? (p=0.008 n=5)
Compile/Onepass-2                    4056.00 ± ∞ ¹   285361.00 ± ∞ ¹  +6935.53% (p=0.008 n=5)    80.00 ± ∞ ¹   -98.03% (p=0.008 n=5)
Compile/Medium-2                     9424.00 ± ∞ ¹   285345.00 ± ∞ ¹  +2927.85% (p=0.008 n=5)    80.00 ± ∞ ¹   -99.15% (p=0.008 n=5)
Compile/Hard-2                      84760.00 ± ∞ ¹   285265.00 ± ∞ ¹   +236.56% (p=0.008 n=5)    80.00 ± ∞ ¹   -99.91% (p=0.008 n=5)
```

Most benchmarks are similar to `Find`, testing simple expressions with small input. In all of these,
the standard library performs much better. To reiterate the guidance at the top of this README, if
you only use simple expressions with small input, you should not use this library.

The compilation benchmarks show that re2 is much slower to compile expressions than the standard
library - this is more than just the overhead of foreign function invocation. This likely results
in the improved performance at runtime in other cases. They also show 280KB+ memory usage per
compilation - the resting memory usage per expression seems to be around ~250KB, much higher than
the standard library. There is significantly more memory usage when using WebAssembly - if this
is not acceptable, setting up the build toolchain for cgo may be worth it. Note the allocation
numbers for cgo are inaccurate as cgo will allocate memory outside of Go - however it should be
inline with the standard library (this needs to be explored in the future).

The match benchmarks show the performance tradeoffs for complexity vs input size. We see the standard
library perform the best with low complexity and size, but for high complexity or high input size,
go-re2 with WebAssembly outperforms, often significantly. Notable is `Hard1`, where even on the smallest
size this library outperforms. The expression is `ABCD|CDEF|EFGH|GHIJ|IJKL|KLMN|MNOP|OPQR|QRST|STUV|UVWX|WXYZ`,
a simple OR of literals - re2 has the concept of regex sets and likely is able to optimize this in a
special way. The CoreRuleSet contains many expressions of a form like this - this possibly indicates good
performance in real world use cases.

Note that because WebAssembly currently only supports single-threaded operation, any compiled expression
can not be executed concurrently and uses locks for safety. When executing many expressions in sequence, it can
be common to not have much contention, but it may be necessary to use a `sync.Pool` of compiled expressions
for concurrency in certain cases, at the expense of more memory usage. When looking at `MatchParallel`, we see
almost perfect scaling in the stdlib case indicating fully parallel execution, no scaling with wazero, and some
scaling with cgo - thread safety is managed by re2 itself in cgo mode which also uses mutexes internally.

[1]: https://pkg.go.dev/regexp
[2]: https://github.com/google/re2
[3]: https://wazero.io
[4]: https://github.com/wasilibs/go-re2/actions/workflows/bench.yaml
[5]: https://github.com/coreruleset/coreruleset
[6]: https://github.com/corazawaf/coraza
[8]: https://www.msys2.org/
[9]: https://brew.sh/
