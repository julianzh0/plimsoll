package snapshot

import "testing"

func TestLineTextKeepsALastLineWithoutANewline(t *testing.T) {
	cases := []struct {
		content    string
		start, end int
		want       string
	}{
		{"a\nb", 2, 2, "b"},
		{"ab", 1, 1, "ab"},
		{"a\nb\nc\n", 2, 3, "b\nc\n"},
		{"a\nb\n", 1, 1, "a\n"},
	}
	for _, tc := range cases {
		if got := string(lineText([]byte(tc.content), linesOf([]byte(tc.content)), LineRange{tc.start, tc.end})); got != tc.want {
			t.Errorf("lineText(%q, %d–%d) = %q, want %q", tc.content, tc.start, tc.end, got, tc.want)
		}
	}
}
