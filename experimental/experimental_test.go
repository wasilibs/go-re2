package experimental

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/wasilibs/go-re2"
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
	{`*`, "error parsing regexp: no argument for repetition operator: *"},
	{`+`, "error parsing regexp: no argument for repetition operator: +"},
	{`?`, "error parsing regexp: no argument for repetition operator: ?"},
	{`(abc`, "error parsing regexp: missing ): (abc"},
	{`abc)`, "error parsing regexp: unexpected ): abc)"},
	{`x[a-z`, "error parsing regexp: missing ]: [a-z"},
	{`[z-a]`, "error parsing regexp: invalid character class range: z-a"},
	{`abc\`, "error parsing regexp: trailing \\"},
	{`a**`, "error parsing regexp: bad repetition operator: **"},
	{`a*+`, "error parsing regexp: bad repetition operator: *+"},
	{`\x`, "error parsing regexp: invalid escape sequence: \\x"},
	{strings.Repeat(`)\pL`, 27000), "error parsing regexp: unexpected ): " + strings.Repeat(`)\pL`, 27000)},
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

func TestGoodSetCompile(t *testing.T) {
	compileSetTest(t, goodRe, "")
}

func TestBadCompileSet(t *testing.T) {
	for i := 0; i < len(badSet); i++ {
		compileSetTest(t, []string{badSet[i].re}, badSet[i].err)
	}
}

type SetTest struct {
	exprs   []string
	matches string
	matched [4][]int
}

var setTests = []SetTest{
	{
		exprs:   []string{`(d)(e){0}(f)`, `[a-c]+`, `abc`, `\d+`},
		matches: "x",
		matched: [4][]int{
			nil, nil, nil, nil,
		},
	},
	{
		exprs:   []string{`(d)(e){0}(f)`, `[a-c]+`, `abc`, `\d+`},
		matches: "123",
		matched: [4][]int{
			nil, {3}, {3}, {3},
		},
	},
	{
		exprs:   []string{`(d)(e){0}(f)`, `[a-c]+`, `abc`, `\d+`},
		matches: "df123abc",
		matched: [4][]int{
			nil, {0}, {0, 3}, {0, 1, 2, 3},
		},
	},
	{
		exprs:   []string{`(d)(e){0}(f)`, `[a-c]+`, `abc`, `\d+`, `d{4}-\d{2}-\d{2}$`, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`, `1[3-9]\d{9}`, `\.[a-zA-Z0-9]+$`, `<!--[\s\S]*?-->`},
		matches: "abcdef123</html><!-- test -->13988889181demo@gmail.com",
		matched: [4][]int{
			nil, {1}, {1, 2}, {1, 2, 3, 5, 6, 7, 8},
		},
	},
	{
		exprs:   []string{`(d)(e){0}(f)`, `[a-c]+`, `abc`, `\d+`, `d{4}-\d{2}-\d{2}$`, `[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`, `1[3-9]\d{9}`, `\.[a-zA-Z0-9]+$`, `<!--[\s\S]*?-->`},
		matches: "df12313988889181demo@gmail.com",
		matched: [4][]int{
			nil, {0}, {0, 3}, {0, 1, 3, 5, 6, 7},
		},
	},
}

func setFindAllTest(t *testing.T, set *Set, matchStr string, matchNum int, matchedIds []int) {
	m := set.FindAll([]byte(matchStr), matchNum)
	sort.Ints(m)
	if !reflect.DeepEqual(m, matchedIds) {
		t.Errorf("Match failure on %s: %v should be %v", matchStr, m, matchedIds)
	}
}

func setFindAllStringTest(t *testing.T, set *Set, matchStr string, matchNum int, matchedIds []int) {
	m := set.FindAllString(matchStr, matchNum)
	sort.Ints(m)
	if !reflect.DeepEqual(m, matchedIds) {
		t.Errorf("Match failure on %s: %v should be %v", matchStr, m, matchedIds)
	}
}

func TestSetFindAll(t *testing.T) {
	for _, test := range setTests {
		set := compileSetTest(t, test.exprs, "")
		if set == nil {
			return
		}
		setFindAllTest(t, set, test.matches, 0, test.matched[0])
		setFindAllTest(t, set, test.matches, 1, test.matched[1])
		setFindAllTest(t, set, test.matches, 2, test.matched[2])
		setFindAllTest(t, set, test.matches, 7, test.matched[3])
		setFindAllTest(t, set, test.matches, 20, test.matched[3])
	}
}

func TestSetFindAllString(t *testing.T) {
	for _, test := range setTests {
		set := compileSetTest(t, test.exprs, "")
		if set == nil {
			return
		}
		setFindAllStringTest(t, set, test.matches, 0, test.matched[0])
		setFindAllStringTest(t, set, test.matches, 1, test.matched[1])
		setFindAllStringTest(t, set, test.matches, 2, test.matched[2])
		setFindAllStringTest(t, set, test.matches, 7, test.matched[3])
		setFindAllStringTest(t, set, test.matches, 20, test.matched[3])
	}
}

func BenchmarkSet(b *testing.B) {
	b.Run("findAll", func(b *testing.B) {
		set, err := CompileSet(goodRe)
		if err != nil {
			panic(err)
		}
		for i := 0; i < b.N; i++ {
			set.FindAll([]byte("abcdef123</html><!-- test -->13988889181demo@gmail.com"), 20)
		}
	})
}

func BenchmarkSetMatchWithFindSubmatch(b *testing.B) {
	b.Run("set match", func(b *testing.B) {
		set, err := CompileSet(goodRe)
		if err != nil {
			panic(err)
		}
		for i := 0; i < b.N; i++ {
			set.FindAll([]byte("abcd123"), 20)
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
	fmt.Println(set.FindAll([]byte("abcd"), len(exprs)))
	fmt.Println(set.FindAll([]byte("123"), len(exprs)))
	fmt.Println(set.FindAll([]byte("abc123"), len(exprs)))
	fmt.Println(set.FindAll([]byte("def"), len(exprs)))
	// Output:
	// [0]
	// [1]
	// [0 1]
	// []
}
