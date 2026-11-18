package process

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func detect(t *testing.T, source snapshot.Source) (snapshot.Snapshot, answer.Findings) {
	t.Helper()
	s := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t))
	return s, Detector{}.Detect(s)
}

func marksOf(f answer.Findings, rule string) map[string]answer.Mark {
	found := map[string]answer.Mark{}
	for _, m := range f.Marks {
		if answer.RuleOf(m.ID) == rule {
			key := ""
			if len(m.Subject.Paths) > 0 {
				key = m.Subject.Paths[0]
			}
			found[key] = m
		}
	}
	return found
}

func row(f answer.Findings, name string) (answer.Statistic, bool) {
	index := slices.IndexFunc(f.Statistics, func(s answer.Statistic) bool { return s.Name == name })
	if index < 0 {
		return answer.Statistic{}, false
	}
	return f.Statistics[index], true
}

func TestPlanningFileMarks(t *testing.T) {
	source := snapshottest.Files(
		map[string]string{"docs/stories/a.md": "x\n", "docs/plans/b.md": "x\n", "docs/stories/c.md": "x\n"},
		map[string]string{"docs/review-packet-7.md": "x\n", "docs/stories/a.md": "x\n", "docs/stories/d.md": "x\n", "docs/guide/e.md": "x\n"},
	)
	source.Changes = []snapshot.Change{{Path: "docs/review-packet-7.md", Added: 1}, {Path: "docs/plans/b.md", Removed: 1},
		{Path: "docs/stories/d.md", From: "docs/stories/c.md"}, {Path: "docs/guide/e.md", Added: 1}}
	_, f := detect(t, source)
	marks := marksOf(f, rulePlanning)
	want := map[string]answer.Change{
		"docs/review-packet-7.md": answer.New, "docs/stories/a.md": answer.Standing,
		"docs/plans/b.md": answer.Resolved, "docs/stories/d.md": answer.Standing,
	}
	if len(marks) != len(want) {
		t.Fatalf("planning marks = %v, want %v", marks, want)
	}
	for path, change := range want {
		if marks[path].Change != change || marks[path].Confidence != answer.Exact {
			t.Errorf("planning mark of %s = %+v, want %s and exact", path, marks[path], change)
		}
	}
	if marks["docs/plans/b.md"].Measured.Value != 0 {
		t.Errorf("a planning file gone: value %v, want 0", marks["docs/plans/b.md"].Measured.Value)
	}
	if r, _ := row(f, "tracked planning and review files"); *r.Value != 3 || *r.Before != 3 {
		t.Errorf("the planning row = %+v, want 3 now and 3 at the base", r)
	}
	if !slices.Contains(f.Notes, "planning and review paths: "+conventions.Default().PlanningRule()) {
		t.Errorf("notes = %q, want the planning rule", f.Notes)
	}
}

func churn(docs, product, tests int, since bool) snapshot.Source {
	files := []snapshot.FileChange{{Path: "docs/a.md", Added: docs}}
	if product > 0 {
		files = append(files, snapshot.FileChange{Path: "src/a.ts", Added: product})
	}
	if tests > 0 {
		files = append(files, snapshot.FileChange{Path: "tests/a.test.ts", Added: tests})
	}
	source := snapshot.Source{Commits: []snapshot.Commit{{ID: "c1", Files: files}, {ID: "c0", Files: []snapshot.FileChange{{Path: "src/b.ts", Added: 1000}}}}}
	if since {
		source.Since = []string{"c1"}
	}
	return source
}

func TestDocChurnMark(t *testing.T) {
	_, f := detect(t, churn(400, 60, 40, true))
	marks := marksOf(f, ruleDocChurn)
	if len(marks) != 1 || marks[""].Change != answer.New || marks[""].Measured.Value != 400 || *marks[""].Measured.Limit != 100 {
		t.Errorf("docs 400 against code 100: marks %+v, want one new mark", marks)
	}
	if _, f := detect(t, churn(50, 100, 0, true)); len(marksOf(f, ruleDocChurn)) != 0 {
		t.Errorf("docs 50 against code 100: a mark, want none")
	}
	_, f = detect(t, churn(50, 0, 0, true))
	if len(marksOf(f, ruleDocChurn)) != 0 || !slices.Contains(f.NotAnalysed, "no product or test churn since the base to compare documentation churn with") {
		t.Errorf("docs without code churn: marks %v, not analysed %q", marksOf(f, ruleDocChurn), f.NotAnalysed)
	}
	records := snapshottest.Files(nil, map[string]string{"src/a.ts": "x\n", "docs/superpowers/plans/a.md": "x\n"})
	records.Commits = []snapshot.Commit{{ID: "c1", Files: []snapshot.FileChange{{Path: "docs/superpowers/plans/a.md", Added: 400}, {Path: "src/a.ts", Added: 100}}}}
	records.Since = []string{"c1"}
	if _, f := detect(t, records); len(marksOf(f, ruleDocChurn)) != 0 {
		t.Errorf("planning-record churn above code churn: a doc-churn mark, want none: records are no documentation")
	}
	_, f = detect(t, churn(400, 60, 40, false))
	if _, found := row(f, "documentation churn since the base, summed over its commits"); found || len(marksOf(f, ruleDocChurn)) != 0 || len(f.NotAnalysed) != 0 {
		t.Errorf("no commit since the base: rows %v, not analysed %q; want neither", f.Statistics, f.NotAnalysed)
	}
}

func TestProcessRunsWithoutALanguageEntry(t *testing.T) {
	_, f := detect(t, snapshottest.Files(map[string]string{"bin/run.sh": "x\n"}, map[string]string{"bin/run.sh": "x\n", "docs/stories/a.md": "x\n"}))
	if marks := marksOf(f, rulePlanning); marks["docs/stories/a.md"].Change != answer.New {
		t.Errorf("a repository of shell scripts: planning marks %v, want the new file", marks)
	}
}

func TestProcessWordingStandingAndEvidence(t *testing.T) {
	planning := func(path string, change answer.Change) answer.Mark {
		return answer.Mark{ID: answer.ID(name, rulePlanning, path), Change: change, Subject: answer.Subject{Kind: "file", Paths: []string{path}}}
	}
	limit := 310.0
	doc := answer.Mark{ID: answer.ID(name, ruleDocChurn, "repository"), Change: answer.New, Subject: answer.Subject{Kind: "repository"},
		Measured: answer.Measured{Value: 4120, Limit: &limit}}
	cases := map[string]answer.Mark{
		"`docs/x.md` is a planning or review record this change added to the repository (plimsoll's rule: repositories hold no planning records); keep it outside the repository.": planning("docs/x.md", answer.New),
		"`docs/x.md` is tracked and lies under a planning or review path; it was tracked at the base too.":                                                                         planning("docs/x.md", answer.Standing),
		"`docs/x.md` is no longer tracked; it lay under a planning or review path at the base.":                                                                                    planning("docs/x.md", answer.Resolved),
		"Documentation churn since the base, summed over its commits, is 4,120 lines, above product plus test churn of 310 lines; the footprint counts the net change instead.":    doc,
	}
	one := 1.0
	small := doc
	small.Measured = answer.Measured{Value: 2, Limit: &one}
	cases["Documentation churn since the base, summed over its commits, is 2 lines, above product plus test churn of 1 line; the footprint counts the net change instead."] = small
	for want, m := range cases {
		if got := (Detector{}).Wording(m); got != want {
			t.Errorf("Wording(%s %s) = %q, want %q", m.ID, m.Change, got, want)
		}
	}
	items := []answer.Mark{planning("a", answer.Standing), planning("b", answer.Standing), planning("c", answer.Standing)}
	if got := (Detector{}).Standing(items); got != "3 tracked planning or review files" {
		t.Errorf("Standing(3) = %q", got)
	}
	if got := (Detector{}).Standing(items[:1]); got != "1 tracked planning or review file" {
		t.Errorf("Standing(1) = %q", got)
	}
	if got := (Detector{}).Standing([]answer.Mark{doc}); got != "documentation churn above code churn" {
		t.Errorf("Standing(doc churn) = %q", got)
	}
	source := snapshottest.Files(map[string]string{"docs/stories/c.md": "x\n"}, map[string]string{"docs/stories/d.md": "x\n"})
	source.Changes = []snapshot.Change{{Path: "docs/stories/d.md", From: "docs/stories/c.md"}}
	s, f := detect(t, source)
	evidence := Detector{}.Explain(s, marksOf(f, rulePlanning)["docs/stories/d.md"])
	if !slices.Contains(evidence, "tracked at the base: yes, as `docs/stories/c.md`") || !strings.HasPrefix(evidence[0], "tracked now: yes") {
		t.Errorf("evidence of a renamed planning file = %q", evidence)
	}
}

func TestAChangeToTheRepositorysOwnFileIsMarked(t *testing.T) {
	files := func(own string) map[string]string {
		f := map[string]string{"src/a.ts": "a\n", "checks/h.ts": "a\n"}
		if own != "" {
			f[".plimsoll.toml"] = own
		}
		return f
	}
	layout := func(lines string) string { return "[layout]\n" + lines }
	cases := []struct {
		name, rule, base, now string
		sentences             []string
	}{
		{"raised from the default", ruleLimitChange, "", "[limits]\nfile_lines = 1500\n",
			[]string{"This change raised the file-size limit from 1,000 to 1,500 lines in `.plimsoll.toml` (plimsoll's default: 1,000)."}},
		{"lowered, and one removed", ruleLimitChange, "[limits]\ntest_ratio = 3\nshared_commits = 8\n", "[limits]\nshared_commits = 3\n", []string{
			"This change lowered the test-ratio limit from 3.0 to 2.0 in `.plimsoll.toml` (plimsoll's default: 2.0).",
			"This change lowered the co-change limit from 8 to 3 shared commits in `.plimsoll.toml` (plimsoll's default: 5).",
		}},
		{"a limit unchanged", ruleLimitChange, "[limits]\nfile_lines = 1500\n", "[limits]\nfile_lines = 1500\n", nil},
		{"no file", ruleLimitChange, "", "", nil},
		{"folders added and removed", ruleDeclarationChange, layout("test_folders = [\"old\", \"checks\"]\n"), layout("test_folders = [\"checks\", \"e2e\"]\n"),
			[]string{"This change altered the test folders declared in `.plimsoll.toml`: added `e2e`, removed 1; 2 declared now, 2 at the base."}},
		{"names declared in a new file", ruleDeclarationChange, "", layout("test_files = [\"*_spec.rb\", \"*IT.java\"]\n"),
			[]string{"This change altered the test files declared in `.plimsoll.toml`: added `*IT.java`, `*_spec.rb`; 2 declared now, 0 at the base."}},
		{"a key removed", ruleDeclarationChange, layout("test_folders = [\"checks\"]\n"), "[limits]\nfile_lines = 1500\n",
			[]string{"This change altered the test folders declared in `.plimsoll.toml`: removed 1; 0 declared now, 1 at the base."}},
		{"reordered", ruleDeclarationChange, layout("test_folders = [\"a\", \"b\"]\n"), layout("test_folders = [\"b\", \"a\"]\n"), nil},
		{"written another way", ruleDeclarationChange, layout("test_folders = [\"checks/\"]\n"), layout("test_folders = [\"./checks\"]\n"), nil},
		{"a value without its form added", ruleDeclarationChange, layout("test_folders = [\"checks\"]\n"), layout("test_folders = [\"checks\", \"a b\"]\n"), nil},
		{"the default base changed", ruleDeclarationChange, "[defaults]\nbase = \"main\"\n", "[defaults]\nbase = \"develop\"\n",
			[]string{"This change altered the default base declared in `.plimsoll.toml`: added `develop`, removed 1; 1 declared now, 1 at the base."}},
		{"not read at the base", ruleDeclarationChange, layout("test_folders = 3\n"), layout("test_folders = [\"checks\"]\n"), nil},
		{"six ignored files, five named", ruleDeclarationChange, "", "[ignore]\nfiles = [\"a.x\", \"b.x\", \"c.x\", \"d.x\", \"e.x\", \"f.x\"]\n",
			[]string{"This change altered the ignored files declared in `.plimsoll.toml`: added `a.x`, `b.x`, `c.x`, `d.x`, `e.x` and 1 more; 6 declared now, 0 at the base."}},
		{"seven acknowledged items, five named", ruleDeclarationChange, "[acknowledged]\n\"links.cycle:z\" = \"x\"\n",
			"[acknowledged]\n" + acknowledgedLines("a", "b", "c", "d", "e", "f", "g"),
			[]string{"This change altered the acknowledged items declared in `.plimsoll.toml`: added `links.cycle:a`, `links.cycle:b`, `links.cycle:c`, `links.cycle:d`, `links.cycle:e` and 2 more, removed 1; 7 declared now, 1 at the base."}},
		{"a table added", ruleDeclarationChange, "", "[acknowledged]\n" + acknowledgedLines("a"),
			[]string{"This change altered the acknowledged items declared in `.plimsoll.toml`: added `links.cycle:a`; 1 declared now, 0 at the base."}},
		{"a reason edited", ruleDeclarationChange, "[acknowledged]\n\"links.cycle:a\" = \"x\"\n", "[acknowledged]\n\"links.cycle:a\" = \"\"\n", nil},
	}
	for _, tc := range cases {
		s, f := detect(t, snapshottest.Files(files(tc.base), files(tc.now)))
		var got []string
		for _, m := range f.Marks {
			if answer.RuleOf(m.ID) == tc.rule {
				if m.Change != answer.New || m.Confidence != answer.Exact || len(Detector{}.Explain(s, m)) == 0 {
					t.Errorf("%s: mark %+v, want a new exact mark with evidence", tc.name, m)
				}
				got = append(got, Detector{}.Wording(m))
			}
		}
		if !slices.Equal(got, tc.sentences) {
			t.Errorf("%s: %s sentences %q, want %q", tc.name, tc.rule, got, tc.sentences)
		}
	}

	s, f := detect(t, snapshottest.Files(files(layout("test_folders = [\"old\", \"checks\"]\n")), files(layout("test_folders = [\"checks\", \"e2e\"]\n"))))
	var m answer.Mark
	for _, mark := range f.Marks {
		if answer.RuleOf(mark.ID) == ruleDeclarationChange {
			m = mark
		}
	}
	two := 2.0
	want := answer.Mark{
		ID: "process.declaration-change:test_folders", Change: answer.New, Confidence: answer.Exact,
		Subject:  answer.Subject{Kind: "file", Paths: []string{".plimsoll.toml"}, Names: []string{"e2e"}},
		Measured: answer.Measured{Value: 2, Before: &two, LimitSource: "any change", Unit: "test folders"},
		Hint:     "compare `[layout]` `test_folders` of `.plimsoll.toml` at the base and now.",
	}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("the mark of changed test folders = %+v, want %+v", m, want)
	}
	evidence := []string{
		"`.plimsoll.toml` `[layout]` `test_folders` at the base: `old`, `checks`", "`.plimsoll.toml` `[layout]` `test_folders` now: `checks`, `e2e`",
		"added: `e2e`", "removed: `old`",
	}
	if got := (Detector{}).Explain(s, m); !slices.Equal(got, evidence) {
		t.Errorf("the evidence = %q, want %q", got, evidence)
	}

	s, f = detect(t, snapshottest.Files(files("[acknowledged]\n"+acknowledgedLines("a", "z")), files("[acknowledged]\n"+acknowledgedLines("a", "b", "c"))))
	at := slices.IndexFunc(f.Marks, func(m answer.Mark) bool { return m.ID == "process.declaration-change:acknowledged" })
	tableEvidence := []string{"`.plimsoll.toml` `[acknowledged]` at the base: 2 entries", "`.plimsoll.toml` `[acknowledged]` now: 3 entries",
		"added: `links.cycle:b`", "added: `links.cycle:c`", "removed: `links.cycle:z`"}
	if at < 0 || f.Marks[at].Hint != "compare the `[acknowledged]` table of `.plimsoll.toml` at the base and now." || !slices.Equal((Detector{}).Explain(s, f.Marks[at]), tableEvidence) {
		t.Errorf("the acknowledged table changed: marks %+v; want its own mark with the table's hint and evidence %q", f.Marks, tableEvidence)
	}

	_, f = detect(t, snapshottest.Files(files(layout("test_folders = 3\n")), files(layout("test_folders = [\"checks\"]\n"))))
	if entry := "`.plimsoll.toml`: `test_folders` could not be read at the base, so a change to it is not marked"; !slices.Contains(f.NotAnalysed, entry) {
		t.Errorf("a key not read at the base: not analysed %q, want %q", f.NotAnalysed, entry)
	}
	binary := snapshottest.Files(files(layout("test_folders = [\"old\"]\n")), files(layout("test_folders = [\"checks\"]\n")))
	for i := range binary.Before {
		if binary.Before[i].Path == ".plimsoll.toml" {
			binary.Before[i].Content, binary.Before[i].Binary = nil, true
		}
	}
	_, f = detect(t, binary)
	if entry := "`.plimsoll.toml` could not be read at the base, so a change to its declarations is not marked"; !slices.Contains(f.NotAnalysed, entry) ||
		slices.ContainsFunc(f.Marks, func(m answer.Mark) bool { return answer.RuleOf(m.ID) == ruleDeclarationChange }) {
		t.Errorf("the file not read at the base: marks %+v, not analysed %q; want no declaration mark and %q", f.Marks, f.NotAnalysed, entry)
	}

	_, f = detect(t, snapshottest.Files(nil, files("[defaults]\nbase = \"main\"\n")))
	if note := "default base declared in `.plimsoll.toml` (`[defaults]` `base`): `main`"; !slices.Contains(f.Notes, note) {
		t.Errorf("a declared base: notes %q, want %q", f.Notes, note)
	}
	_, f = detect(t, snapshottest.Files(nil, files("[limits]\nfile_lines = 1500\ntest_ratio = 2.5\nmax_depth = 3\n")))
	note := "limits set by `.plimsoll.toml`: the file-size limit 1,500 lines (plimsoll's default: 1,000), the test-ratio limit 2.5 (plimsoll's default: 2.0)"
	problem := "`.plimsoll.toml`: `max_depth` is no limit plimsoll knows; what it does not set keeps plimsoll's default"
	if !slices.Contains(f.Notes, note) || !slices.Contains(f.NotAnalysed, problem) {
		t.Errorf("notes %q, not analysed %q; want %q and %q", f.Notes, f.NotAnalysed, note, problem)
	}
}

// acknowledgedLines are entries of the acknowledged table for the import rings of the given files.
func acknowledgedLines(files ...string) string {
	var lines strings.Builder
	for _, f := range files {
		fmt.Fprintf(&lines, "\"links.cycle:%s\" = \"one ring on purpose\"\n", f)
	}
	return lines.String()
}
