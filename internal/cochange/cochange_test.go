package cochange

import (
	"fmt"
	"slices"
	"testing"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

const (
	a = "src/a.ts"
	b = "tests/fakes/b.ts"
)

// sample is a repository whose files are tracked now and whose history is base commits, then commits since.
type sample struct {
	files       map[string]string
	base, since [][]string
}

func (s sample) build() snapshot.Snapshot {
	source := snapshottest.Files(nil, s.files)
	for i, paths := range s.since {
		id := fmt.Sprintf("5%09d", len(s.since)-i)
		source.Commits = append(source.Commits, snapshottest.Commit(id, paths...))
		source.Since = append(source.Since, id)
	}
	for i, paths := range s.base {
		source.Commits = append(source.Commits, snapshottest.Commit(fmt.Sprintf("b%09d", len(s.base)-i), paths...))
	}
	return snapshot.Build(source, conventions.Default())
}

func files(paths ...string) map[string]string {
	found := map[string]string{a: "x\n", b: "x\n"}
	for _, p := range paths {
		found[p] = "x\n"
	}
	return found
}

func times(n int, paths ...string) [][]string {
	var commits [][]string
	for range n {
		commits = append(commits, paths)
	}
	return commits
}

func wide(n int, paths ...string) []string {
	found := slices.Clone(paths)
	for i := len(paths); i < n; i++ {
		found = append(found, fmt.Sprintf("lib/f%d.ts", i))
	}
	return found
}

// check compares what every mark carries.
func check(t *testing.T, m answer.Mark, x, y string, change answer.Change, value, before, total float64) {
	t.Helper()
	limit := 5.0
	want := answer.Mark{
		ID: "co-change.pair:" + x + "+" + y, Change: change, Subject: answer.Subject{Kind: "files", Paths: []string{x, y}},
		Measured: answer.Measured{
			Value: value, Before: &before, Limit: &limit, LimitSource: "at least 5 shared commits", Total: &total, Unit: "shared commits",
		},
		Confidence: answer.Exact, Hint: "`explain` lists the shared commits by name.",
	}
	if m.ID != want.ID || m.Change != want.Change || !slices.Equal(m.Subject.Paths, want.Subject.Paths) || m.Subject.Kind != "files" ||
		m.Confidence != want.Confidence || m.Hint != want.Hint || m.Measured.Value != value || m.Measured.Unit != want.Measured.Unit ||
		m.Measured.LimitSource != want.Measured.LimitSource || m.Measured.Before == nil || *m.Measured.Before != before ||
		m.Measured.Limit == nil || *m.Measured.Limit != limit || m.Measured.Total == nil || *m.Measured.Total != total {
		t.Errorf("mark = %+v %+v, want %+v %+v", m, m.Measured, want, want.Measured)
	}
}

func marks(s sample) []answer.Mark { return Detector{}.Detect(s.build()).Marks }

func TestPairMarks(t *testing.T) {
	cases := map[string]struct {
		s                    sample
		change               answer.Change
		value, before, total float64
	}{
		"new":      {sample{files: files(), base: times(4, a, b), since: times(1, a, b)}, answer.New, 5, 4, 5},
		"grown":    {sample{files: files(), base: times(5, a, b), since: times(1, a, b)}, answer.Grown, 6, 5, 6},
		"standing": {sample{files: files(), base: times(5, a, b)}, answer.Standing, 5, 5, 5},
		"a alone":  {sample{files: files(), base: append(times(5, a, b), times(2, a)...)}, answer.Standing, 5, 5, 7},
		"20 paths": {sample{files: files(), base: append(times(4, a, b), wide(20, a, b))}, answer.Standing, 5, 5, 5},
	}
	for name, c := range cases {
		found := marks(c.s)
		if len(found) != 1 {
			t.Errorf("%s: marks = %+v, want one", name, found)
			continue
		}
		check(t, found[0], a, b, c.change, c.value, c.before, c.total)
	}
	if found := marks(sample{files: files(), base: append(times(4, a, b), wide(21, a, b))}); len(found) != 0 {
		t.Errorf("the fifth shared commit touches 21 paths: marks = %+v, want none", found)
	}
	for _, other := range []string{"scripts/x.ts", "src/c.ts"} {
		found := marks(sample{files: files(other), base: times(6, a, other)})
		if len(found) != 1 || found[0].Subject.Paths[1] != other && found[0].Subject.Paths[0] != other {
			t.Errorf("a with %s: marks = %+v, want one", other, found)
		}
	}
	none := map[string]sample{
		"a test file":      {files: map[string]string{a: "x\n", "tests/a.test.ts": "test(\"x\", () => {})\n"}, base: times(6, a, "tests/a.test.ts")},
		"two test helpers": {files: files("tests/support/c.ts"), base: times(6, b, "tests/support/c.ts")},
		"not code":         {files: map[string]string{a: "x\n", "package.json": "{}\n", "README.md": "x\n"}, base: append(times(6, a, "package.json"), times(6, a, "README.md")...)},
		"gone":             {files: files(), base: times(6, a, "src/gone.ts")},
		"a record":         {files: files("docs/qualification/run.ts"), base: times(6, a, "docs/qualification/run.ts")},
	}
	for name, s := range none {
		if found := marks(s); len(found) != 0 {
			t.Errorf("%s: marks = %+v, want none", name, found)
		}
	}
}

func rowsNamed(f answer.Findings, name string) []answer.Statistic {
	var found []answer.Statistic
	for _, s := range f.Statistics {
		if s.Name == name {
			found = append(found, s)
		}
	}
	return found
}

func TestTopPairsAndTheirCountAreStatistics(t *testing.T) {
	c, d, e := "src/c.ts", "src/d.ts", "src/e.ts"
	history := slices.Concat(times(9, a, b), times(3, a), times(7, d, e), times(7, c, d), times(5, c, e))
	f := Detector{}.Detect(sample{files: files(c, d, e), base: history}.build())
	count := rowsNamed(f, "pairs with at least 5 shared commits")
	if len(count) != 1 || *count[0].Value != 4 || *count[0].Before != 4 || count[0].Unit != "pairs" {
		t.Errorf("count rows = %+v, want one with 4 pairs", count)
	}
	pairs := rowsNamed(f, "pair")
	subjects := []string{}
	for _, p := range pairs {
		subjects = append(subjects, p.Subject)
	}
	if want := []string{"src/a.ts + tests/fakes/b.ts", "src/c.ts + src/d.ts", "src/d.ts + src/e.ts"}; !slices.Equal(subjects, want) {
		t.Errorf("pair rows = %q, want %q", subjects, want)
	}
	if first := pairs[0]; *first.Value != 9 || *first.Before != 9 || first.Unit != "shared commits" || first.Note != "of the 12 that touched either" {
		t.Errorf("first pair row = %+v", first)
	}
	f = Detector{}.Detect(sample{files: files(), base: times(2, a, b)}.build())
	if count := rowsNamed(f, "pairs with at least 5 shared commits"); len(count) != 1 || *count[0].Value != 0 || len(rowsNamed(f, "pair")) != 0 {
		t.Errorf("no pair at the limit: statistics %+v", f.Statistics)
	}
	if len(f.Notes) != 1 || f.Notes[0] != "pairs: two code files tracked now, at least one product code, neither a file that defines tests, not both in one folder where a language imports folders; planning, review and measurement paths left out" {
		t.Errorf("notes = %q", f.Notes)
	}
}

func TestHistoryWithoutAUsableCommitIsNotAnalysed(t *testing.T) {
	f := Detector{}.Detect(sample{files: files(), base: times(6, wide(21, a, b)...)}.build())
	if len(f.Marks) != 0 || len(f.Statistics) != 0 || !slices.Equal(f.NotAnalysed, []string{"no usable commit in the history, so no pairs"}) {
		t.Errorf("no usable commit: %+v", f)
	}
}

func TestPairsRunWithoutALanguageEntry(t *testing.T) {
	found := marks(sample{files: map[string]string{"bin/a.sh": "x\n", "bin/b.sh": "x\n"}, base: times(5, "bin/a.sh", "bin/b.sh")})
	if len(found) != 1 || found[0].ID != "co-change.pair:bin/a.sh+bin/b.sh" {
		t.Errorf("shell scripts: marks = %+v", found)
	}
}

func TestNoPairsInsideOnePackageFolder(t *testing.T) {
	goFiles := map[string]string{"go.mod": "module example.com/m\n", "internal/s/one.go": "package s\n", "internal/s/two.go": "package s\n", "internal/u/u.go": "package u\n"}
	if found := marks(sample{files: goFiles, base: times(6, "internal/s/one.go", "internal/s/two.go")}); len(found) != 0 {
		t.Errorf("two files of one Go package: marks %+v, want none: a package changes together", found)
	}
	if found := marks(sample{files: goFiles, base: times(6, "internal/s/one.go", "internal/u/u.go")}); len(found) != 1 {
		t.Errorf("files of two Go packages: marks %+v, want one", found)
	}
	if found := marks(sample{files: files("src/c.ts"), base: times(6, a, "src/c.ts")}); len(found) != 1 {
		t.Errorf("two TypeScript files of one folder: marks %+v, want one: a file names one file", found)
	}
	jvm := map[string]string{
		"src/main/java/com/acme/p/A.java": "package com.acme.p;\n", "src/main/java/com/acme/p/B.java": "package com.acme.p;\n",
		"src/main/java/com/acme/p/C.kt": "package com.acme.p\n", "src/test/java/com/acme/p/FakeA.java": "package com.acme.p;\n",
	}
	class, fake := "src/main/java/com/acme/p/A.java", "src/test/java/com/acme/p/FakeA.java"
	for _, tc := range []struct {
		name  string
		other string
		want  int
	}{
		{"two Java files of one folder", "src/main/java/com/acme/p/B.java", 0},
		{"a Java and a Kotlin file of one folder", "src/main/java/com/acme/p/C.kt", 0},
		{"a class and its fake in another source set", fake, 1},
	} {
		if found := marks(sample{files: jvm, base: times(6, class, tc.other)}); len(found) != tc.want {
			t.Errorf("%s: marks %+v, want %d", tc.name, found, tc.want)
		}
	}
	cs := map[string]string{"src/Acme/Core/A.cs": "namespace Acme.Core;\n", "src/Acme/Core/B.cs": "namespace Acme.Core;\n", "src/Acme/Store/S.cs": "namespace Acme.Store;\n"}
	if found := marks(sample{files: cs, base: times(6, "src/Acme/Core/A.cs", "src/Acme/Core/B.cs")}); len(found) != 0 {
		t.Errorf("two C# files of one folder: marks %+v, want none", found)
	}
	if found := marks(sample{files: cs, base: times(6, "src/Acme/Core/A.cs", "src/Acme/Store/S.cs")}); len(found) != 1 {
		t.Errorf("C# files of two folders: marks %+v, want one", found)
	}
}
