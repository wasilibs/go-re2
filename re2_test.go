package re2

import (
	"context"
	_ "embed"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"regexp"
	"strings"
	"testing"
)

func TestSimple(t *testing.T) {
	r := MustCompile("bear")
	defer r.Close()

	require.True(t, r.MatchString("bear"))
	require.False(t, r.MatchString("cat"))
}

func TestTinyGo(t *testing.T) {
	defer swapInTinyGo()

	r := MustCompile("bear")
	defer r.Close()

	require.True(t, r.MatchString("bear"))
	require.False(t, r.MatchString("cat"))
}

var compileBenchData = []struct{ name, re string }{
	{"Onepass", `^a.[l-nA-Cg-j]?e$`},
	{"Medium", `^((a|b|[d-z0-9])*(日){4,5}.)+$`},
	{"Hard", strings.Repeat(`((abc)*|`, 50) + strings.Repeat(`)`, 50)},
}

func BenchmarkCompile(b *testing.B) {
	tests := []struct {
		name    string
		compile func(string) (interface{}, error)
		close   func(interface{})
	}{
		{
			name: "libre2",
			compile: func(re string) (interface{}, error) {
				return Compile(re)
			},
			close: func(r interface{}) {
				r.(*Regexp).Close()
			},
		},
		{
			name: "stdlib",
			compile: func(re string) (interface{}, error) {
				return regexp.Compile(re)
			},
			close: func(interface{}) {},
		},
		{
			name: "tinygo",
			compile: func(re string) (interface{}, error) {
				defer swapInTinyGo()
				return Compile(re)
			},
			close: func(r interface{}) {
				defer swapInTinyGo()
				r.(*Regexp).Close()
			},
		},
	}
	for _, tc := range tests {
		tt := tc
		b.Run(tt.name, func(b *testing.B) {
			for _, data := range compileBenchData {
				b.Run(data.name, func(b *testing.B) {
					b.ReportAllocs()
					for i := 0; i < b.N; i++ {
						if r, err := tt.compile(data.re); err != nil {
							b.Fatal(err)
						} else {
							tt.close(r)
						}
					}
				})
			}
		})
	}
}

var benchData = []struct{ name, re string }{
	{"Easy0", "ABCDEFGHIJKLMNOPQRSTUVWXYZ$"},
	{"Easy0i", "(?i)ABCDEFGHIJklmnopqrstuvwxyz$"},
	{"Easy1", "A[AB]B[BC]C[CD]D[DE]E[EF]F[FG]G[GH]H[HI]I[IJ]J$"},
	{"Medium", "[XYZ]ABCDEFGHIJKLMNOPQRSTUVWXYZ$"},
	{"Hard", "[ -~]*ABCDEFGHIJKLMNOPQRSTUVWXYZ$"},
	{"Hard1", "ABCD|CDEF|EFGH|GHIJ|IJKL|KLMN|MNOP|OPQR|QRST|STUV|UVWX|WXYZ"},
}

var benchSizes = []struct {
	name string
	n    int
}{
	{"16", 16},
	{"32", 32},
	{"1K", 1 << 10},
	{"32K", 32 << 10},
	{"1M", 1 << 20},
	{"32M", 32 << 20},
}

func BenchmarkMatch(b *testing.B) {
	tests := []struct {
		name    string
		compile func(string) interface{}
		match   func(re interface{}, text []byte) bool
		close   func(interface{})
	}{
		{
			name: "libre2",
			compile: func(re string) interface{} {
				return MustCompile(re)
			},
			match: func(re interface{}, text []byte) bool {
				return re.(*Regexp).Match(text)
			},
			close: func(r interface{}) {
				r.(*Regexp).Close()
			},
		},
		{
			name: "stdlib",
			compile: func(re string) interface{} {
				return regexp.MustCompile(re)
			},
			match: func(re interface{}, text []byte) bool {
				return re.(*regexp.Regexp).Match(text)
			},
			close: func(interface{}) {},
		},
		{
			name: "tinygo",
			compile: func(re string) interface{} {
				defer swapInTinyGo()
				return MustCompile(re)
			},
			match: func(re interface{}, text []byte) bool {
				defer swapInTinyGo()
				return re.(*Regexp).Match(text)
			},
			close: func(r interface{}) {
				defer swapInTinyGo()
				r.(*Regexp).Close()
			},
		},
	}

	for _, tc := range tests {
		tt := tc
		b.Run(tt.name, func(b *testing.B) {
			for _, data := range benchData {
				r := tt.compile(data.re)
				for _, size := range benchSizes {
					if (testing.Short()) && size.n > 1<<10 {
						continue
					}
					t := makeText(size.n)
					b.Run(data.name+"/"+size.name, func(b *testing.B) {
						b.SetBytes(int64(size.n))
						for i := 0; i < b.N; i++ {
							if tt.match(r, t) {
								b.Fatal("match!")
							}
						}
					})
				}
				tt.close(r)
			}
		})
	}

}

var text []byte

func makeText(n int) []byte {
	if len(text) >= n {
		return text[:n]
	}
	text = make([]byte, n)
	x := ^uint32(0)
	for i := range text {
		x += x
		x ^= 1
		if int32(x) < 0 {
			x ^= 0x88888eef
		}
		if x%31 == 0 {
			text[i] = '\n'
		} else {
			text[i] = byte(x%(0x7E+1-0x20) + 0x20)
		}
	}
	return text
}

//go:embed tinygo.wasm
var tinygoWasm []byte

var tinygoRT wazero.Runtime
var tinygoABI libre2ABIDef

func init() {
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithWasmCore2())

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		panic(err)
	}

	// TODO(anuraaga): Find a way to compile wasm without this import.
	if _, err := rt.NewModuleBuilder("env").
		ExportFunction("__main_argc_argv", func(int32, int32) int32 {
			return 0
		}).
		Instantiate(ctx, rt); err != nil {
		panic(err)
	}

	mod, err := rt.InstantiateModuleFromBinary(ctx, tinygoWasm)
	if err != nil {
		panic(err)
	}

	abi := libre2ABIDef{
		cre2New:    mod.ExportedFunction("cre2_new"),
		cre2Delete: mod.ExportedFunction("cre2_delete"),
		cre2Match:  mod.ExportedFunction("cre2_match"),

		malloc: mod.ExportedFunction("malloc"),
		free:   mod.ExportedFunction("free"),

		memory: mod.Memory(),
	}

	tinygoRT = rt
	tinygoABI = abi
}

func swapInTinyGo() func() {
	abi := libre2ABI
	libre2ABI = tinygoABI
	return func() {
		libre2ABI = abi
	}
}
