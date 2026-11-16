package links

import (
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func testImport(change answer.Change, value float64, before *float64) answer.Mark {
	return answer.Mark{
		ID: answer.ID(name, ruleTestImport, "src/a.ts"), Change: change,
		Subject: answer.Subject{Kind: "file", Paths: []string{"src/a.ts"}}, Measured: answer.Measured{Value: value, Before: before},
	}
}

func TestLinksWordingStandingAndEvidence(t *testing.T) {
	zero, two := 0.0, 2.0
	for m, want := range map[*answer.Mark]string{
		new(testImport(answer.New, 1, &zero)):     "`src/a.ts` is product code and has 1 import of test code; it had none at the base.",
		new(testImport(answer.New, 2, nil)):       "`src/a.ts` is product code and has 2 imports of test code; it did not exist at the base.",
		new(testImport(answer.Grown, 3, &two)):    "`src/a.ts` is product code and has 3 imports of test code, 2 at the base.",
		new(testImport(answer.Standing, 3, &two)): "`src/a.ts` is product code and has 3 imports of test code, 2 at the base.",
		new(testImport(answer.Resolved, 0, &two)): "No import of test code found in `src/a.ts` now; it had 2 at the base.",
	} {
		if got := (Detector{}).Wording(*m); got != want {
			t.Errorf("Wording(%s, %v) = %q, want %q", m.Change, m.Measured.Value, got, want)
		}
	}
	one := testImport(answer.Standing, 1, &zero)
	if got := (Detector{}).Standing([]answer.Mark{one}); got != "1 product file with imports of test code" {
		t.Errorf("Standing(1) = %q", got)
	}
	if got := (Detector{}).Standing([]answer.Mark{one, one}); got != "2 product files with imports of test code" {
		t.Errorf("Standing(2) = %q", got)
	}

	cycle := func(change answer.Change, value float64, before *float64) answer.Mark {
		return answer.Mark{ID: answer.ID(name, ruleCycle, "src/a.ts"), Change: change,
			Subject: answer.Subject{Kind: "files", Paths: []string{"src/a.ts", "src/b.ts", "src/c.ts"}}, Measured: answer.Measured{Value: value, Before: before}}
	}
	three := 3.0
	for m, want := range map[*answer.Mark]string{
		new(cycle(answer.New, 3, nil)):         "3 files import each other in a ring, the first `src/a.ts`; none of them was in a ring at the base.",
		new(cycle(answer.Grown, 4, &three)):    "The ring of `src/a.ts` has 4 files, 3 at the base.",
		new(cycle(answer.Standing, 4, &three)): "The ring of `src/a.ts` has 4 files, 3 at the base.",
		new(cycle(answer.Resolved, 0, &three)): "The ring of `src/a.ts` had 3 files at the base; no ring found among them now.",
	} {
		if got := (Detector{}).Wording(*m); got != want {
			t.Errorf("Wording(cycle %s, %v) = %q, want %q", m.Change, m.Measured.Value, got, want)
		}
	}
	ringItem := cycle(answer.Standing, 3, &three)
	if got := (Detector{}).Standing([]answer.Mark{one, ringItem, ringItem}); got != "1 product file with imports of test code; 2 import rings" {
		t.Errorf("Standing(test-import, two rings) = %q", got)
	}
	folder := func(change answer.Change, value float64, before *float64, paths ...string) answer.Mark {
		return answer.Mark{ID: answer.ID(name, ruleFolderCycle, paths[0]), Change: change,
			Subject: answer.Subject{Kind: "folders", Paths: paths}, Measured: answer.Measured{Value: value, Before: before}}
	}
	for m, want := range map[*answer.Mark]string{
		new(folder(answer.New, 2, nil, "src/a", "src/b")):                "2 folders under `src/` depend on each other in a cycle, the first `src/a/`; none of them was in a folder cycle at the base.",
		new(folder(answer.New, 2, nil, "a", "b")):                        "2 folders at the root depend on each other in a cycle, the first `a/`; none of them was in a folder cycle at the base.",
		new(folder(answer.Grown, 3, &two, "src/a", "src/b", "src/c")):    "The folder cycle of `src/a/` has 3 folders, 2 at the base.",
		new(folder(answer.Standing, 3, &two, "src/a", "src/b", "src/c")): "The folder cycle of `src/a/` has 3 folders, 2 at the base.",
		new(folder(answer.Resolved, 0, &two, "src/a", "src/b")):          "The folder cycle of `src/a/` had 2 folders at the base; no cycle found among them now.",
	} {
		if got := (Detector{}).Wording(*m); got != want {
			t.Errorf("Wording(folder cycle %s, %v) = %q, want %q", m.Change, m.Measured.Value, got, want)
		}
	}
	folderItem := folder(answer.Standing, 2, &two, "src/a", "src/b")
	if got := (Detector{}).Standing([]answer.Mark{one, cycle(answer.Standing, 3, &three), folderItem, folderItem}); got != "1 product file with imports of test code; 1 import ring; 2 folder cycles" {
		t.Errorf("Standing(with folder cycles) = %q", got)
	}
	tsCase, goCase := languageCases()["typescript"], languageCases()["go"]
	explainFolders := func(base, now map[string]string) []string {
		s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
		return (Detector{}).Explain(s, marksOf(Detector{}.Detect(s), ruleFolderCycle)[0])
	}
	open := tsCase.folders("a/x>b/y", "a/u>b/v", "b/z", "a/w")
	closed := tsCase.folders("a/x>b/y", "a/u>b/v", "b/z>a/w")
	if got, want := explainFolders(open, closed), []string{
		"cycle of 2 folders under `src/`: `src/a/`, `src/b/`; at the base: none", "`src/b/` has 1 import of `src/a/`", "`src/a/` has 2 imports of `src/b/`",
		"`src/b/z.ts` line 1 imports `src/a/w.ts`", "`src/a/u.ts` line 1 imports `src/b/v.ts`", "`src/a/x.ts` line 1 imports `src/b/y.ts`",
		"imports between two files of one import ring are left out",
	}; !slices.Equal(got, want) {
		t.Errorf("Explain(new folder cycle) =\n%q\nwant\n%q", got, want)
	}
	if got := explainFolders(closed, open); len(got) < 3 || got[0] != "no cycle among these folders now; at the base: a cycle of 2 folders under `src/`" ||
		!slices.Contains(got, "at the base `src/b/` had 1 import of `src/a/`") || !slices.Contains(got, "at the base `src/b/z.ts` line 1 imported `src/a/w.ts`") {
		t.Errorf("Explain(resolved folder cycle) = %q", got)
	}
	typeOnlyCycle := tsCase.folders("adapters/db>domain/order", "adapters/http")
	typeOnlyCycle["src/domain/order.ts"] = "import type { H } from \"../adapters/http.ts\";\nexport const v = 1;\n"
	if got := explainFolders(tsCase.folders("adapters/db>domain/order"), typeOnlyCycle); !slices.ContainsFunc(got, func(l string) bool { return strings.HasSuffix(l, ", type-only") }) {
		t.Errorf("Explain(a cycle closed by a type-only import) = %q, want its line marked type-only", got)
	}
	if got := explainFolders(goCase.folders("a/x>b/y", "b/z", "a/w"), goCase.folders("a/x>b/y", "b/z>a/w")); !slices.Contains(got, "`internal/b/z/z.go` line 4 imports the folder `internal/a/w`") {
		t.Errorf("Explain(go folder cycle) = %q", got)
	}
	explainRing := func(base, now map[string]string) []string {
		s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
		return (Detector{}).Explain(s, marksOf(Detector{}.Detect(s), ruleCycle)[0])
	}
	if got, want := explainRing(ring("a>b", "b>c"), ring("a>b", "b>c", "c>a")), []string{
		"ring of 3 files; at the base: none", "`src/a.ts` line 1 imports `src/b.ts`", "`src/b.ts` line 1 imports `src/c.ts`",
		"`src/c.ts` line 1 imports `src/a.ts`", "type-only imports are left out",
	}; !slices.Equal(got, want) {
		t.Errorf("Explain(new ring) = %q, want %q", got, want)
	}
	if got := explainRing(ring("b>c", "c>d", "d>b"), ring("b>c", "c>d", "d>b", "b>a", "a>c")); len(got) == 0 || got[0] != "ring of 4 files; at the base: 3" {
		t.Errorf("Explain(grown ring) = %q", got)
	}
	if got := explainRing(ring("a>b", "b>c", "c>a"), ring("a>b", "b>c")); len(got) < 2 ||
		got[0] != "no ring among these files now; at the base: a ring of 3 files" || got[1] != "at the base `src/a.ts` line 1 imported `src/b.ts`" {
		t.Errorf("Explain(resolved ring) = %q", got)
	}

	ts := languageCases()["typescript"]
	a, db, clock := ts.product("a"), ts.support("db"), ts.support("clock")
	base, now := ts.imports(a, db), ts.imports(a, db, clock)
	s := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t))
	grown := marksOf(Detector{}.Detect(s), ruleTestImport)[0]
	want := []string{
		"`src/a.ts` line 1 imports `tests/support/db.ts` (test infrastructure)",
		"`src/a.ts` line 2 imports `tests/support/clock.ts` (test infrastructure)",
		"at the base: 1 import of test code", "searched: the import lines of `src/a.ts`",
	}
	if got := (Detector{}).Explain(s, grown); !slices.Equal(got, want) {
		t.Errorf("Explain(grown) = %q, want %q", got, want)
	}
	typeOnly := ts.imports("", db)
	typeOnly[a] = `import type { T } from "../tests/support/db.ts";` + "\n"
	s = snapshot.Build(snapshottest.Files(ts.tree(nil), typeOnly), conventions.Default(), snapshottest.Readings(t))
	if got := (Detector{}).Explain(s, marksOf(Detector{}.Detect(s), ruleTestImport)[0]); len(got) == 0 ||
		got[0] != "`src/a.ts` line 1 imports `tests/support/db.ts` (test infrastructure), type-only" {
		t.Errorf("Explain(type-only) = %q", got)
	}

	golang := languageCases()["go"]
	goNow := golang.imports(golang.product("a"), golang.support("db"))
	s = snapshot.Build(snapshottest.Files(golang.tree(nil), goNow), conventions.Default(), snapshottest.Readings(t))
	if got := (Detector{}).Explain(s, marksOf(Detector{}.Detect(s), ruleTestImport)[0]); len(got) == 0 ||
		got[0] != "`internal/a/a.go` line 4 imports the folder `internal/test/db` (1 file of test code)" {
		t.Errorf("Explain(go) = %q", got)
	}
}
