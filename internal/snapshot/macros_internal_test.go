package snapshot

import (
	"slices"
	"testing"
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
