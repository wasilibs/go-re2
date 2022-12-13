package re2

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func BenchmarkFind(b *testing.B) {
	b.StopTimer()
	re := MustCompileBenchmark("a+b+")
	wantSubs := "aaabb"
	s := []byte("acbb" + wantSubs + "dd")
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		subs := re.Find(s)
		if string(subs) != wantSubs {
			b.Fatalf("Find(%q) = %q; want %q", s, subs, wantSubs)
		}
	}
}

func BenchmarkFindAllNoMatches(b *testing.B) {
	re := MustCompileBenchmark("a+b+")
	s := []byte("acddee")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		all := re.FindAll(s, -1)
		if all != nil {
			b.Fatalf("FindAll(%q) = %q; want nil", s, all)
		}
	}
}

func BenchmarkFindString(b *testing.B) {
	b.StopTimer()
	re := MustCompileBenchmark("a+b+")
	wantSubs := "aaabb"
	s := "acbb" + wantSubs + "dd"
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		subs := re.FindString(s)
		if subs != wantSubs {
			b.Fatalf("FindString(%q) = %q; want %q", s, subs, wantSubs)
		}
	}
}

func BenchmarkFindSubmatch(b *testing.B) {
	b.StopTimer()
	re := MustCompileBenchmark("a(a+b+)b")
	wantSubs := "aaabb"
	s := []byte("acbb" + wantSubs + "dd")
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		subs := re.FindSubmatch(s)
		if string(subs[0]) != wantSubs {
			b.Fatalf("FindSubmatch(%q)[0] = %q; want %q", s, subs[0], wantSubs)
		}
		if string(subs[1]) != "aab" {
			b.Fatalf("FindSubmatch(%q)[1] = %q; want %q", s, subs[1], "aab")
		}
	}
}

func BenchmarkFindStringSubmatch(b *testing.B) {
	b.StopTimer()
	re := MustCompileBenchmark("a(a+b+)b")
	wantSubs := "aaabb"
	s := "acbb" + wantSubs + "dd"
	b.StartTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		subs := re.FindStringSubmatch(s)
		if subs[0] != wantSubs {
			b.Fatalf("FindStringSubmatch(%q)[0] = %q; want %q", s, subs[0], wantSubs)
		}
		if subs[1] != "aab" {
			b.Fatalf("FindStringSubmatch(%q)[1] = %q; want %q", s, subs[1], "aab")
		}
	}
}

func BenchmarkLiteral(b *testing.B) {
	x := strings.Repeat("x", 50) + "y"
	b.StopTimer()
	re := MustCompileBenchmark("y")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			b.Fatalf("no match!")
		}
	}
}

func BenchmarkNotLiteral(b *testing.B) {
	x := strings.Repeat("x", 50) + "y"
	b.StopTimer()
	re := MustCompileBenchmark(".y")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			b.Fatalf("no match!")
		}
	}
}

func BenchmarkMatchClass(b *testing.B) {
	b.StopTimer()
	x := strings.Repeat("xxxx", 20) + "w"
	re := MustCompileBenchmark("[abcdw]")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			b.Fatalf("no match!")
		}
	}
}

func BenchmarkMatchClass_InRange(b *testing.B) {
	b.StopTimer()
	// 'b' is between 'a' and 'c', so the charclass
	// range checking is no help here.
	x := strings.Repeat("bbbb", 20) + "c"
	re := MustCompileBenchmark("[ac]")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		if !re.MatchString(x) {
			b.Fatalf("no match!")
		}
	}
}

func BenchmarkReplaceAll(b *testing.B) {
	x := "abcdefghijklmnopqrstuvwxyz"
	b.StopTimer()
	re := MustCompileBenchmark("[cjrw]")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.ReplaceAllString(x, "")
	}
}

func BenchmarkAnchoredLiteralShortNonMatch(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	re := MustCompileBenchmark("^zbc(d|e)")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkAnchoredLiteralLongNonMatch(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	for i := 0; i < 15; i++ {
		x = append(x, x...)
	}
	re := MustCompileBenchmark("^zbc(d|e)")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkAnchoredShortMatch(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	re := MustCompileBenchmark("^.bc(d|e)")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkAnchoredLongMatch(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	for i := 0; i < 15; i++ {
		x = append(x, x...)
	}
	re := MustCompileBenchmark("^.bc(d|e)")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkOnePassShortA(b *testing.B) {
	b.StopTimer()
	x := []byte("abcddddddeeeededd")
	re := MustCompileBenchmark("^.bc(d|e)*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkNotOnePassShortA(b *testing.B) {
	b.StopTimer()
	x := []byte("abcddddddeeeededd")
	re := MustCompileBenchmark(".bc(d|e)*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkOnePassShortB(b *testing.B) {
	b.StopTimer()
	x := []byte("abcddddddeeeededd")
	re := MustCompileBenchmark("^.bc(?:d|e)*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkNotOnePassShortB(b *testing.B) {
	b.StopTimer()
	x := []byte("abcddddddeeeededd")
	re := MustCompileBenchmark(".bc(?:d|e)*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkOnePassLongPrefix(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	re := MustCompileBenchmark("^abcdefghijklmnopqrstuvwxyz.*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkOnePassLongNotPrefix(b *testing.B) {
	b.StopTimer()
	x := []byte("abcdefghijklmnopqrstuvwxyz")
	re := MustCompileBenchmark("^.bcdefghijklmnopqrstuvwxyz.*$")
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		re.Match(x)
	}
}

func BenchmarkMatchParallelShared(b *testing.B) {
	x := []byte("this is a long line that contains foo bar baz")
	re := MustCompileBenchmark("foo (ba+r)? baz")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			re.Match(x)
		}
	})
}

func BenchmarkMatchParallelCopied(b *testing.B) {
	x := []byte("this is a long line that contains foo bar baz")
	re := MustCompileBenchmark("foo (ba+r)? baz")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		re := re.Copy()
		for pb.Next() {
			re.Match(x)
		}
	})
}

var sink string

func BenchmarkQuoteMetaAll(b *testing.B) {
	specials := make([]byte, 0)
	for i := byte(0); i < utf8.RuneSelf; i++ {
		if special(i) {
			specials = append(specials, i)
		}
	}
	s := string(specials)
	b.SetBytes(int64(len(s)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = QuoteMeta(s)
	}
}

func BenchmarkQuoteMetaNone(b *testing.B) {
	s := "abcdefghijklmnopqrstuvwxyz"
	b.SetBytes(int64(len(s)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = QuoteMeta(s)
	}
}

var compileBenchData = []struct{ name, re string }{
	{"Onepass", `^a.[l-nA-Cg-j]?e$`},
	{"Medium", `^((a|b|[d-z0-9])*(日){4,5}.)+$`},
	{"Hard", strings.Repeat(`((abc)*|`, 50) + strings.Repeat(`)`, 50)},
}

func BenchmarkCompile(b *testing.B) {
	for _, data := range compileBenchData {
		b.Run(data.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := CompileBenchmark(data.re); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMatch(b *testing.B) {
	for _, data := range benchData {
		r := MustCompileBenchmark(data.re)
		for _, size := range benchSizes {
			if testing.Short() && size.n > 1<<10 {
				continue
			}
			t := makeText(size.n)
			b.Run(data.name+"/"+size.name, func(b *testing.B) {
				b.SetBytes(int64(size.n))
				for i := 0; i < b.N; i++ {
					if r.Match(t) {
						b.Fatal("match!")
					}
				}
			})
		}
	}
}

func BenchmarkMatchParallel(b *testing.B) {
	for _, data := range benchData {
		r := MustCompileBenchmark(data.re)
		for _, size := range benchSizes {
			if testing.Short() && size.n > 1<<10 {
				continue
			}
			t := makeText(size.n)
			b.Run(data.name+"/"+size.name, func(b *testing.B) {
				b.SetBytes(int64(size.n))
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						if r.Match(t) {
							b.Fatal("match!")
						}
					}
				})
			})
		}
	}
}

func BenchmarkMatch_onepass_regex(b *testing.B) {
	r := MustCompileBenchmark(`(?s)\A.*\z`)
	for _, size := range benchSizes {
		if testing.Short() && size.n > 1<<10 {
			continue
		}
		t := makeText(size.n)
		b.Run(size.name, func(b *testing.B) {
			b.SetBytes(int64(size.n))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if !r.Match(t) {
					b.Fatal("not match!")
				}
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

// Bitmap used by func special to check whether a character needs to be escaped.
var specialBytes [16]byte

// special reports whether byte b needs to be escaped by QuoteMeta.
func special(b byte) bool {
	return b < utf8.RuneSelf && specialBytes[b%16]&(1<<(b/16)) != 0
}

func init() {
	for _, b := range []byte(`\.+*?()|[]{}^$`) {
		specialBytes[b%16] |= 1 << (b / 16)
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
