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

[1]: https://pkg.go.dev/regexp
[2]: https://github.com/google/re2
[3]: https://wazero.io