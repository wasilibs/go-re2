package experimental

import (
	"fmt"
	"github.com/wasilibs/go-re2"
	"reflect"
	"strings"
	"testing"
)

func TestCompileLatin1(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		want    bool
	}{
		{
			pattern: `\xac\xed\x00\x05`,
			input:   "\xac\xed\x00\x05t\x00\x04test",
			want:    true,
		},
		{
			pattern: `\xac\xed\x00\x05`,
			input:   "\xac\xed\x00t\x00\x04test",
			want:    false,
		},
		// Make sure flags are parsed
		{
			pattern: `(?sm)\xac\xed\x00\x05`,
			input:   "\xac\xed\x00\x05t\x00\x04test",
			want:    true,
		},
		{
			pattern: `(?sm)\xac\xed\x00\x05`,
			input:   "\xac\xed\x00t\x00\x04test",
			want:    false,
		},
		// Unicode character classes don't work but matching bytes still does.
		{
			pattern: "ハロー",
			input:   "ハローワールド",
			want:    true,
		},
		{
			pattern: "ハロー",
			input:   "グッバイワールド",
			want:    false,
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(fmt.Sprintf("%s/%s", tt.pattern, tt.input), func(t *testing.T) {
			re := MustCompileLatin1(tt.pattern)
			if re.MatchString(tt.input) != tt.want {
				t.Errorf("MatchString(%q) = %v, want %v", tt.input, !tt.want, tt.want)
			}
		})
	}
}

var goodRe = []string{
	``,
	`.`,
	`^.$`,
	`a`,
	`a*`,
	`a+`,
	`a?`,
	`a|b`,
	`a*|b*`,
	`(a*|b)(c*|d)`,
	`[a-z]`,
	`[a-abc-c\-\]\[]`,
	`[a-z]+`,
	`[abc]`,
	`[^1234]`,
	`[^\n]`,
	`\!\\`,
}

type stringError struct {
	re  string
	err string
}

var badSet = []stringError{
	{`*`, "no argument for repetition operator: *"},
	{`+`, "no argument for repetition operator: +"},
	{`?`, "no argument for repetition operator: ?"},
	{`(abc`, "missing ): (abc"},
	{`abc)`, "unexpected ): abc)"},
	{`x[a-z`, "missing ]: [a-z"},
	{`[z-a]`, "invalid character class range: z-a"},
	{`abc\`, "trailing \\"},
	{`a**`, "bad repetition operator: **"},
	{`a*+`, "bad repetition operator: *+"},
	{`\x`, "invalid escape sequence: \\x"},
}

func TestGoodSetCompile(t *testing.T) {
	compileSetTest(t, goodRe, "")
}

func compileSetTest(t *testing.T, exprs []string, error string) *Set {
	set, err := CompileSet(exprs)
	if error == "" && err != nil {
		t.Error("compiling `", exprs, "`; unexpected error: ", err.Error())
	}
	if error != "" && err == nil {
		t.Error("compiling `", exprs, "`; missing error")
	} else if error != "" && !strings.Contains(err.Error(), error) {
		t.Error("compiling `", exprs, "`; wrong error: ", err.Error(), "; want ", error)
	}
	return set
}

func TestBadCompileSet(t *testing.T) {
	for i := 0; i < len(badSet); i++ {
		compileSetTest(t, []string{badSet[i].re}, badSet[i].err)
	}
}

type SetTest struct {
	expr    []string
	matches map[string][]int
}

var setTests = []SetTest{
	{
		expr: []string{"abc", "\\d+"},
		matches: map[string][]int{
			"abc":    {0},
			"123":    {1},
			"abc123": {0, 1},
			"def":    {},
		},
	},
	{
		expr: []string{"[a-c]+", "(d)(e){0}(f)"},
		matches: map[string][]int{
			"a234v": {0},
			"df":    {1},
			"abcdf": {0, 1},
			"def":   {},
		},
	},
}

func setMatchTest(t *testing.T, set *Set, matchStr string, matchedIds []int) {
	m := set.Match([]byte(matchStr), 10)
	if !reflect.DeepEqual(m, matchedIds) {
		t.Errorf("Match failure on %s: %v should be %v", matchStr, m, matchedIds)
	}
}

func TestSetMatch(t *testing.T) {
	for _, test := range setTests {
		set := compileSetTest(t, test.expr, "")
		if set == nil {
			return
		}
		for matchStr, matchedIds := range test.matches {
			setMatchTest(t, set, matchStr, matchedIds)
		}
	}
}

func BenchmarkSetMatchWithFindSubmatch(b *testing.B) {
	b.Run("set match", func(b *testing.B) {
		set, err := CompileSet(goodRe)
		if err != nil {
			panic(err)
		}
		for i := 0; i < b.N; i++ {
			set.Match([]byte("abcd123"), 20)
		}
	})
	b.Run("findSubmatch", func(b *testing.B) {
		re, err := re2.Compile("(" + strings.Join(goodRe, ")|(") + ")")
		if err != nil {
			panic(err)
		}
		for i := 0; i < b.N; i++ {
			re.FindAllStringSubmatchIndex("abcd123", 20)
		}
	})
}

func ExampleCompileSet() {
	exprs := []string{"abc", "\\d+"}
	set, err := CompileSet(exprs)
	if err != nil {
		panic(err)
	}
	fmt.Println(set.Match([]byte("abcd"), len(exprs)))
	fmt.Println(set.Match([]byte("123"), len(exprs)))
	fmt.Println(set.Match([]byte("abc123"), len(exprs)))
	fmt.Println(set.Match([]byte("def"), len(exprs)))
	// Output:
	// [0]
	// [1]
	// [0 1]
	// []
}
