package snapshot

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

func TestWithBlankedKeepsCleanFunctionsAndLosesNone(t *testing.T) {
	clean := function{name: "clean", start: 0, end: 10, complexity: 3}
	fixed := function{name: "fixed", start: 20, end: 30, broken: true}
	stuck := function{name: "stuck", start: 40, end: 50, broken: true}
	got := withBlanked([]function{clean, fixed, stuck}, []function{
		{name: "clean", start: 0, end: 10, complexity: 9},  // the copy reads the clean one otherwise: the written one stays
		{name: "fixed", start: 22, end: 30, complexity: 5}, // takes the place of the unreadable one it overlaps
	})
	names := func(fs []function) []string {
		var out []string
		for _, f := range fs {
			out = append(out, f.name)
		}
		return out
	}
	if !slices.Equal(names(got), []string{"clean", "fixed", "stuck"}) || got[0].complexity != 3 || got[1].broken || got[1].complexity != 5 || !got[2].broken {
		t.Errorf("withBlanked(clean, fixed, stuck) = %+v, want clean unchanged (3), fixed read from the copy (5), stuck kept unreadable", got)
	}
}

func TestRewriteMacrosKeepsOffsetsAndRewritesWholeNames(t *testing.T) {
	var cpp conventions.Language
	for _, l := range conventions.Default().Languages {
		if l.Name == "cpp" {
			cpp = l
		}
	}
	content := "MY_API_EXPORT\nMY_INLINE int f(_In_reads_(n,\n m) int a) {\n  LIST_FOREACH(x, &list, link)\n    a++;\n  return a;\n}\n"
	want := "             \n          int f(" + strings.Repeat(" ", 13) + "\n    int a) {\n  while       (x, &list, link)\n    a++;\n  return a;\n}\n"
	if got := string(rewriteMacros(cpp.Macros, []byte(content))); got != want {
		t.Errorf("rewriteMacros(%q) = %q, want %q", content, got, want)
	}
}
