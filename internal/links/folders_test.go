package links

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// folders is a tree of placed files: "a/x>b/y" is a/x importing b/y, a bare "a/x" a file without an import.
func (c languageCase) folders(edges ...string) map[string]string {
	targets := map[string][]string{}
	var order []string
	add := func(file string) {
		if _, known := targets[file]; !known {
			targets[file] = nil
			order = append(order, file)
		}
	}
	placed := func(at string) string { return c.placed(path.Dir(at), path.Base(at)) }
	for _, edge := range edges {
		from, to, imports := strings.Cut(edge, ">")
		add(placed(from))
		if imports {
			add(placed(to))
			targets[placed(from)] = append(targets[placed(from)], placed(to))
		}
	}
	files := map[string]string{}
	for _, file := range order {
		files[file] = c.importing(file, targets[file])
	}
	return c.tree(files)
}

func TestFolderCycleMarksInEachLanguage(t *testing.T) {
	base := map[string]string{"src/a/x.ts": "import \"../b/y.ts\";\n", "src/b/y.ts": "export const y = 1;\n",
		"src/a/d.ts": "import \"./w.ts\";\n", "src/a/w.ts": "export const w = 1;\n"}
	now := map[string]string{"src/a/x.ts": base["src/a/x.ts"], "src/b/y.ts": base["src/b/y.ts"],
		"src/b/d.ts": "import \"../a/w.ts\";\n", "src/a/w.ts": base["src/a/w.ts"]}
	moved := snapshottest.Files(base, now)
	moved.Changes = []snapshot.Change{{Path: "src/b/d.ts", From: "src/a/d.ts", Added: 1, Removed: 1}}
	if marks := marksOf(detect(t, moved), ruleFolderCycle); len(marks) != 1 || marks[0].ID != "links.folder-cycle:src/a" ||
		marks[0].Change != answer.Touched || marks[0].Measured.Value != 2 {
		t.Errorf("a folder cycle a moved file made: marks %+v, want one touched of 2 folders", marks)
	}
	eachLanguage(t, func(t *testing.T, name string, c languageCase) {
		cycle := []string{"a/x>b/y", "b/z>a/w"}
		open := []string{"a/x>b/y", "b/z", "a/w"}
		with := func(edges []string, more ...string) []string { return append(slices.Clone(edges), more...) }
		before := func(v float64) *float64 { return &v }
		cases := []struct {
			name      string
			base, now []string
			first     string
			change    answer.Change
			value     float64
			atBase    *float64
		}{
			{"new", open, cycle, "a", answer.New, 2, nil},
			{"standing", cycle, cycle, "a", answer.Standing, 2, before(2)},
			{"grown", cycle, with(cycle, "b/y>c/v", "c/u>a/w"), "a", answer.Grown, 3, before(2)},
			{"resolved", cycle, open, "a", answer.Resolved, 0, before(2)},
			{"joined", with(cycle, "c/x>d/y", "d/z>c/w"), with(cycle, "c/x>d/y", "d/z>c/w", "b/y>c/w", "d/y>a/w"), "a", answer.Grown, 4, before(2)},
			{"hexagonal", []string{"adapters/db>domain/order", "domain/policy", "adapters/http"},
				[]string{"adapters/db>domain/order", "domain/policy>adapters/http"}, "adapters", answer.New, 2, nil},
			{"deeper", []string{"a/deep/x>b/y", "b/z", "a/w"}, []string{"a/deep/x>b/y", "b/z>a/w"}, "a", answer.New, 2, nil},
		}
		for _, tc := range cases {
			marks := marksOf(detect(t, snapshottest.Files(c.folders(tc.base...), c.folders(tc.now...))), ruleFolderCycle)
			id := "links.folder-cycle:" + c.under + "/" + tc.first
			if len(marks) != 1 || marks[0].ID != id || marks[0].Change != tc.change || marks[0].Measured.Value != tc.value ||
				(tc.atBase == nil) != (marks[0].Measured.Before == nil) || tc.atBase != nil && *marks[0].Measured.Before != *tc.atBase {
				t.Errorf("%s: folder-cycle marks %+v, want one %s %s of %v", tc.name, marks, id, tc.change, tc.value)
				continue
			}
			m := marks[0]
			if m.Subject.Kind != "folders" || !slices.IsSorted(m.Subject.Paths) || m.Measured.Unit != "folders" || m.Measured.Limit != nil ||
				m.Measured.LimitSource != "the Acyclic Dependencies Principle (Robert C. Martin)" || m.Hint != folderHint || m.Confidence != answer.Candidate {
				t.Errorf("%s: mark %+v lacks a field of the rule", tc.name, m)
			}
		}
		zero, one := 0.0, 1.0
		newCycle := detect(t, snapshottest.Files(c.folders(open...), c.folders(cycle...)))
		if row, _ := statisticNamed(newCycle, "folder cycles"); !sameStatistic(row, answer.Statistic{Name: "folder cycles", Value: &one, Before: &zero, Unit: "cycles",
			Note: "2 folders, among 2 imports between sibling folders"}) || !slices.Contains(newCycle.Notes, folderCyclesNote) {
			t.Errorf("a new cycle: row %+v, notes %q", row, newCycle.Notes)
		}
		acyclic := detect(t, same(c.folders("a/x>b/y", "c/u>b/y")))
		if row, _ := statisticNamed(acyclic, "folder cycles"); !sameStatistic(row, answer.Statistic{Name: "folder cycles", Value: &zero, Before: &zero, Unit: "cycles",
			Note: "among 2 imports between sibling folders"}) {
			t.Errorf("no cycle: row %+v", row)
		}
		if _, has := statisticNamed(detect(t, same(c.fan(8, c.nowhere))), "folder cycles"); has {
			t.Errorf("a language over the limit: a folder-cycles row")
		}
		mutual := detect(t, snapshottest.Files(c.folders("a/x", "b/y"), c.folders("a/x>b/y", "b/y>a/x")))
		rings := 0
		for _, m := range mutual.Marks {
			if answer.RuleOf(m.ID) == ruleCycle {
				rings++
			}
		}
		if folders := len(marksOf(mutual, ruleFolderCycle)); c.noRings != "" && (folders != 1 || rings != 0) || c.noRings == "" && (folders != 0 || rings != 1) {
			t.Errorf("two files importing each other: %d folder-cycle and %d ring marks", folders, rings)
		}
	})
}

func TestFolderCyclesCountOnlyWhatTheRuleSays(t *testing.T) {
	ts := languageCases()["typescript"]
	imp := func(targets ...string) string {
		var lines []string
		for _, target := range targets {
			lines = append(lines, `import "`+target+`";`)
		}
		return strings.Join(lines, "\n") + "\nexport const v = 1;\n"
	}
	cycle := ts.folders("a/x>b/y", "b/z>a/w")
	twoLanguages := map[string]string{
		"src/app/__init__.py": "", "src/a/x.py": "import b.y\n", "src/b/y.py": "x = 1\n",
		"src/b/z.ts": imp("../a/w.ts"), "src/a/w.ts": imp(),
	}
	overLimit := clone(twoLanguages)
	overLimit["src/b/z.ts"] = imp("../a/w.ts", "./gone.ts")
	renamed := snapshottest.Files(cycle, ts.folders("k/x>b/y", "b/z>k/w"))
	renamed.Changes = []snapshot.Change{{Path: "src/k/x.ts", From: "src/a/x.ts"}, {Path: "src/k/w.ts", From: "src/a/w.ts"}, {Path: "src/b/z.ts"}}
	lib := map[string]string{}
	var moves []snapshot.Change
	for p, content := range cycle {
		moved := "lib/" + strings.TrimPrefix(p, "src/")
		lib[moved] = content
		moves = append(moves, snapshot.Change{Path: moved, From: p})
	}
	parentMoved := snapshottest.Files(cycle, lib)
	parentMoved.Changes = moves
	typeOnlyNow := ts.folders("adapters/db>domain/order", "adapters/http")
	typeOnlyNow["src/domain/order.ts"] = "import type { H } from \"../adapters/http.ts\";\nexport const v = 1;\n"
	testCode := ts.folders("a/x>b/y", "a/w")
	testCode["src/b/y.test.ts"] = imp("../a/w.ts") + "test(\"x\", () => {});\n"
	cases := []struct {
		name   string
		source snapshot.Source
		want   string // the folder-cycle mark's identifier and change, "" for none
		rings  int
	}{
		{"siblings told apart", same(ts.folders("domain/a>other/o", "other/p>domain-events/e")), "", 0},
		{"a file in the parent", same(map[string]string{"src/main.ts": imp("./a/x.ts"), "src/a/x.ts": imp(), "src/a/y.ts": imp("../main.ts")}), "", 0},
		{"a ring beside", same(ts.folders("a/x>b/y", "b/y>a/x", "b/z>a/w")), "", 1},
		{"a ring and a cycle", same(ts.folders("a/x>b/y", "b/y>a/x", "a/u>b/v", "b/z>a/w")), "links.folder-cycle:src/a standing", 1},
		{"type-only closes it", snapshottest.Files(ts.folders("adapters/db>domain/order"), typeOnlyNow), "links.folder-cycle:src/adapters new", 0},
		{"test code", same(testCode), "", 0},
		{"a renamed member", renamed, "links.folder-cycle:src/b standing", 0},
		{"a renamed parent", parentMoved, "links.folder-cycle:lib/a standing", 0},
		{"the top level", snapshottest.Files(nil, map[string]string{"a/x.ts": imp("../b/y.ts"), "b/y.ts": imp(), "b/z.ts": imp("../a/w.ts"), "a/w.ts": imp()}), "links.folder-cycle:a new", 0},
		{"two languages", snapshottest.Files(nil, twoLanguages), "links.folder-cycle:src/a new", 0},
		{"a language over the limit", snapshottest.Files(nil, overLimit), "", 0},
	}
	moved := snapshottest.Files(ts.folders("c/x>d/y", "d/z>c/w", "c/q", "a/p>b/n", "b/o"), ts.folders("c/x>d/y", "d/z>c/w", "a/q", "a/p>b/n", "b/o>a/p"))
	moved.Changes = []snapshot.Change{{Path: "src/a/q.ts", From: "src/c/q.ts"}, {Path: "src/b/o.ts"}}
	var changes []string
	for _, m := range marksOf(detect(t, moved), ruleFolderCycle) {
		changes = append(changes, m.ID+" "+string(m.Change))
	}
	if want := []string{"links.folder-cycle:src/a new", "links.folder-cycle:src/c standing"}; !slices.Equal(changes, want) {
		t.Errorf("one file moved into a folder that existed at the base: marks %q, want %q", changes, want)
	}
	if row, _ := statisticNamed(detect(t, same(ring("a>b"))), "folder cycles"); row.Note != "no import between sibling folders of product code" {
		t.Errorf("all product files in one folder: row %+v", row)
	}
	for _, tc := range cases {
		f := detect(t, tc.source)
		var got []string
		rings := 0
		for _, m := range f.Marks {
			switch answer.RuleOf(m.ID) {
			case ruleFolderCycle:
				got = append(got, m.ID+" "+string(m.Change))
			case ruleCycle:
				rings++
			}
		}
		var want []string
		if tc.want != "" {
			want = []string{tc.want}
		}
		if !slices.Equal(got, want) || rings != tc.rings {
			t.Errorf("%s: folder-cycle marks %q, %d ring marks; want %q and %d", tc.name, got, rings, want, tc.rings)
		}
	}
}
