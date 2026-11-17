package links

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestAddedCodeRowsGroupNamesByFileAndReason(t *testing.T) {
	now := map[string]string{
		"src/x.ts": "export function alpha() {}\nexport function beta() {}\nexport function delta() {}\n" +
			"export function epsilon() {}\nexport class Gamma {}\nconst secret = \"hunter2-value\";\n",
		"tests/x.test.ts": "alpha();\nbeta();\n",
		"docs/guide.md":   "Call epsilon. TOPSECRET\n",
		"docs/other.md":   "epsilon\n",
	}
	f := detect(t, snapshottest.Files(nil, now))
	var notes []string
	for _, s := range f.Statistics {
		if s.Name == addedCodeRow {
			notes = append(notes, s.Note)
		}
	}
	want := []string{
		"`delta` (function), `Gamma` (type); no use in the repository's code files",
		"`epsilon` (function); no use in the repository's code files, named only in `docs/guide.md` and 1 more document",
		"`alpha`, `beta` (functions); used only by tests",
	}
	if !slices.Equal(notes, want) {
		t.Errorf("added code rows = %q, want %q", notes, want)
	}
	if !slices.Contains(f.Notes, additionsNote) {
		t.Errorf("Notes = %q, want the added code note", f.Notes)
	}
	if summary := "Added code without a non-test use: 5 names in 1 file; `report` lists them."; !slices.Equal(f.Summaries, []string{summary}) {
		t.Errorf("Summaries = %q, want %q", f.Summaries, summary)
	}
	for _, s := range f.Statistics {
		if strings.Contains(s.Note, "hunter2") || strings.Contains(s.Note, "TOPSECRET") {
			t.Errorf("row %+v shows content", s)
		}
	}
	for _, m := range f.Marks {
		if strings.Contains(m.ID, "alpha") || strings.Contains(m.ID, "delta") {
			t.Errorf("mark %s: added code is never a mark", m.ID)
		}
	}
	unchanged := detect(t, same(now))
	if slices.ContainsFunc(unchanged.Statistics, func(s answer.Statistic) bool { return s.Name == addedCodeRow }) ||
		slices.Contains(unchanged.Notes, additionsNote) || len(unchanged.Summaries) != 0 {
		t.Errorf("an unchanged tree: rows, note or summary %+v, want none", unchanged)
	}
	one := detect(t, snapshottest.Files(nil, map[string]string{"src/lonely.ts": "export function lonelyHelper() {}\n"}))
	if summary := "Added code without a non-test use: 1 name in 1 file; `report` lists it."; !slices.Equal(one.Summaries, []string{summary}) {
		t.Errorf("one name: Summaries = %q, want %q", one.Summaries, summary)
	}
}
