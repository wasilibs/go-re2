package experimental

import (
	"fmt"
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
