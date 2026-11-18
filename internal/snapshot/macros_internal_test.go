package snapshot

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

func TestWithBlankedKeepsCleanFunctionsAndLosesNone(t *testing.T) {
	clean := function{name: "clean", start: 0, end: 10, complexity: 3}
	fixed := function{name: "fixed", start: 20, end: 30, broken: true}
	stuck := function{name: "stuck", start: 40, end: 50, broken: true}
	got := withRewritten([]function{clean, fixed, stuck}, []function{
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
		t.Errorf("withRewritten(clean, fixed, stuck) = %+v, want clean unchanged (3), fixed read from the copy (5), stuck kept unreadable", got)
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
	// a name that only starts like an annotation stays; a foreach's template arguments keep their commas
	for content, want := range map[string]string{
		"int _Internal_init(_In_ int a);\n":        "int _Internal_init(     int a);\n",
		"  foreach (QPair<int, int> p, pairs) {\n": "  for     (QPair<int, int> p: pairs) {\n",
		"  Q_FOREACH (const QString &s, l) {\n":    "  for       (const QString &s: l) {\n",
	} {
		if got := string(rewriteMacros(cpp.Macros, []byte(content))); got != want {
			t.Errorf("rewriteMacros(%q) = %q, want %q", content, got, want)
		}
	}
}

func TestRewriteMacrosTakesTimeThatGrowsWithTheFile(t *testing.T) {
	var cpp conventions.Language
	for _, l := range conventions.Default().Languages {
		if l.Name == "cpp" {
			cpp = l
		}
	}
	// a long run of macro names is rewritten in passes that do not grow with the run
	content := []byte(strings.Repeat("A_B ", 16000) + "int f(int a) {\n  return a;\n}\n")
	done := make(chan []byte, 1)
	go func() { done <- rewriteMacros(cpp.Macros, content) }()
	select {
	case got := <-done:
		if !strings.HasPrefix(string(got), strings.Repeat(" ", 64000)+"int f") {
			t.Errorf("rewriteMacros(16,000 macro names) = %.40q…, want them blanked", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("rewriteMacros(16,000 macro names) did not finish within 10 s")
	}
}
