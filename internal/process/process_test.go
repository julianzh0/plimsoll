package process

import (
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func detect(t *testing.T, source snapshot.Source) (snapshot.Snapshot, answer.Findings) {
	t.Helper()
	s := snapshot.Build(source, conventions.Default())
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
	if _, found := row(f, "documentation churn since the base"); found || len(marksOf(f, ruleDocChurn)) != 0 || len(f.NotAnalysed) != 0 {
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
		"`docs/x.md` is tracked and lies under a planning or review path; it was not tracked at the base.": planning("docs/x.md", answer.New),
		"`docs/x.md` is tracked and lies under a planning or review path; it was tracked at the base too.": planning("docs/x.md", answer.Standing),
		"`docs/x.md` is no longer tracked; it lay under a planning or review path at the base.":            planning("docs/x.md", answer.Resolved),
		"Documentation churn since the base is 4,120 lines, above product plus test churn of 310 lines.":   doc,
	}
	one := 1.0
	small := doc
	small.Measured = answer.Measured{Value: 2, Limit: &one}
	cases["Documentation churn since the base is 2 lines, above product plus test churn of 1 line."] = small
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
	source := snapshottest.Files(map[string]string{"docs/stories/c.md": "x\n"}, map[string]string{"docs/stories/d.md": "x\n"})
	source.Changes = []snapshot.Change{{Path: "docs/stories/d.md", From: "docs/stories/c.md"}}
	s, f := detect(t, source)
	evidence := Detector{}.Explain(s, marksOf(f, rulePlanning)["docs/stories/d.md"])
	if !slices.Contains(evidence, "tracked at the base: yes, as `docs/stories/c.md`") || !strings.HasPrefix(evidence[0], "tracked now: yes") {
		t.Errorf("evidence of a renamed planning file = %q", evidence)
	}
}

func TestALimitChangedSinceTheBaseIsMarked(t *testing.T) {
	files := func(limits string) map[string]string {
		f := map[string]string{"src/a.ts": "a\n"}
		if limits != "" {
			f[".plimsoll.toml"] = "[limits]\n" + limits
		}
		return f
	}
	cases := []struct {
		name, base, now string
		sentences       []string
	}{
		{"raised from the default", "", "file_lines = 1500\n",
			[]string{"This change raised the file-size limit from 1,000 to 1,500 lines in `.plimsoll.toml` (plimsoll's default: 1,000)."}},
		{"lowered, and one removed", "test_ratio = 3\nshared_commits = 8\n", "shared_commits = 3\n", []string{
			"This change lowered the test-ratio limit from 3.0 to 2.0 in `.plimsoll.toml` (plimsoll's default: 2.0).",
			"This change lowered the co-change limit from 8 to 3 shared commits in `.plimsoll.toml` (plimsoll's default: 5).",
		}},
		{"unchanged", "file_lines = 1500\n", "file_lines = 1500\n", nil},
		{"no file", "", "", nil},
	}
	for _, tc := range cases {
		s, f := detect(t, snapshottest.Files(files(tc.base), files(tc.now)))
		var got []string
		for _, m := range f.Marks {
			if answer.RuleOf(m.ID) == ruleLimitChange {
				if m.Change != answer.New || m.Confidence != answer.Exact || len(Detector{}.Explain(s, m)) == 0 {
					t.Errorf("%s: mark %+v, want a new exact mark with evidence", tc.name, m)
				}
				got = append(got, Detector{}.Wording(m))
			}
		}
		if !slices.Equal(got, tc.sentences) {
			t.Errorf("%s: limit-change sentences %q, want %q", tc.name, got, tc.sentences)
		}
	}
	_, f := detect(t, snapshottest.Files(nil, files("file_lines = 1500\ntest_ratio = 2.5\nmax_depth = 3\n")))
	note := "limits set by `.plimsoll.toml`: the file-size limit 1,500 lines (plimsoll's default: 1,000), the test-ratio limit 2.5 (plimsoll's default: 2.0)"
	problem := "`.plimsoll.toml`: `max_depth` is no limit plimsoll knows; a limit it does not set keeps plimsoll's default"
	if !slices.Contains(f.Notes, note) || !slices.Contains(f.NotAnalysed, problem) {
		t.Errorf("notes %q, not analysed %q; want %q and %q", f.Notes, f.NotAnalysed, note, problem)
	}
}
