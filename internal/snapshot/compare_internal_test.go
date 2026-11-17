package snapshot

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// without is the lines of content with the ranges taken out.
func without(content string, ranges []LineRange) []string {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var kept []string
	for i, l := range lines {
		if !slices.ContainsFunc(ranges, func(r LineRange) bool { return r.Start <= i+1 && i+1 <= r.End }) {
			kept = append(kept, l)
		}
	}
	return kept
}

func TestCompareLinesFindsWhatWasAddedAndRemoved(t *testing.T) {
	cases := []struct {
		base, now      string
		added, removed []LineRange
	}{
		{"a\nb\nc\n", "a\nx\nb\nc\ny\n", []LineRange{{2, 2}, {5, 5}}, nil},
		{"a\nb\nc\n", "a\nc\n", nil, []LineRange{{2, 2}}},
		{"a\nb\nc\n", "a\nx\nc\n", []LineRange{{2, 2}}, []LineRange{{2, 2}}},
		{"", "a\nb\n", []LineRange{{1, 2}}, nil},
		{"a\nb\n", "", nil, []LineRange{{1, 2}}},
		{"a\nb\n", "a\r\nb\r\n", []LineRange{{1, 2}}, []LineRange{{1, 2}}},
	}
	rng := rand.New(rand.NewPCG(7, 11))
	random := func() string {
		var b strings.Builder
		for range rng.IntN(40) {
			fmt.Fprintf(&b, "line %d\n", rng.IntN(6))
		}
		return b.String()
	}
	for _, tc := range cases {
		got := compareLines([]byte(tc.base), []byte(tc.now))
		if !slices.Equal(got.added, tc.added) || !slices.Equal(got.removed, tc.removed) {
			t.Errorf("compareLines(%q, %q) = added %v, removed %v; want %v, %v", tc.base, tc.now, got.added, got.removed, tc.added, tc.removed)
		}
	}
	moved := compareLines([]byte("a\nb\nc\nd\n"), []byte("c\nd\na\nb\n"))
	if lineTotal(moved.added) != 2 || lineTotal(moved.removed) != 2 {
		t.Errorf("compareLines(a block moved down) = added %v, removed %v; want two lines each", moved.added, moved.removed)
	}
	for i := range 50 {
		base, now := random(), random()
		got := compareLines([]byte(base), []byte(now))
		if !slices.Equal(without(base, got.removed), without(now, got.added)) {
			t.Errorf("compareLines(random pair %d) = added %v, removed %v; what stays differs:\n%q\n%q", i, got.added, got.removed, base, now)
		}
	}
	var a, b bytes.Buffer
	a.WriteString("first\n")
	b.WriteString("first\n")
	for i := range 7998 {
		fmt.Fprintf(&a, "base %d\n", i)
		fmt.Fprintf(&b, "now %d\n", i)
	}
	a.WriteString("last\n")
	b.WriteString("last\n")
	past := compareLines(a.Bytes(), b.Bytes())
	if !slices.Equal(past.removed, []LineRange{{2, 7999}}) || !slices.Equal(past.added, []LineRange{{2, 7999}}) {
		t.Errorf("compareLines(a pair past %d steps) = added %v, removed %v; want [{2 7999}] both", compareSteps, past.added, past.removed)
	}
}

func TestAlikeIsTwiceTheSharedLinesOverAll(t *testing.T) {
	lines := func(s ...string) [][]byte {
		var out [][]byte
		for _, l := range s {
			out = append(out, []byte(l))
		}
		return out
	}
	cases := []struct {
		a, b [][]byte
		want float64
	}{
		{lines("a", "b"), lines("a", "b"), 1},
		{lines("a", "b"), lines("c", "d"), 0},
		{lines("a", "b", "c", "d"), lines("a", "x", "c", "d"), 0.75},
		{nil, nil, 0},
	}
	for _, tc := range cases {
		if got := alike(tc.a, tc.b); got != tc.want {
			t.Errorf("alike(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func lineTotal(ranges []LineRange) int {
	n := 0
	for _, r := range ranges {
		n += r.End - r.Start + 1
	}
	return n
}
