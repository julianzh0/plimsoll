package hotspots

import (
	"fmt"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func detect(t testing.TB, source snapshot.Source) answer.Findings {
	t.Helper()
	return Detector{}.Detect(snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)))
}

type hotspot struct {
	path            string
	changes, before float64
	note            string
}

func rows(f answer.Findings) []hotspot {
	var found []hotspot
	for _, s := range f.Statistics {
		found = append(found, hotspot{s.Subject, *s.Value, *s.Before, s.Note})
	}
	return found
}

// history builds commits c<n>..c0, newest first, from the paths each one touches.
func history(touches map[int][]string, newest int) []snapshot.Commit {
	var commits []snapshot.Commit
	for i := newest; i >= 0; i-- {
		commits = append(commits, snapshottest.Commit(fmt.Sprintf("c%d", i), touches[i]...))
	}
	return commits
}

func TestHotspotsRankChangesTimesLines(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{
		"src/big.ts": snapshottest.Lines(100), "src/busy.ts": snapshottest.Lines(10), "tests/a.test.ts": snapshottest.Lines(50),
		"src/quiet.ts": snapshottest.Lines(1000), "docs/qualification/run.ts": snapshottest.Lines(500),
	})
	touches := map[int][]string{}
	for i := 0; i <= 10; i++ {
		if i >= 3 {
			touches[i] = append(touches[i], "src/busy.ts")
		}
		if i >= 1 && i <= 3 {
			touches[i] = append(touches[i], "src/big.ts")
		}
		if i >= 4 && i <= 7 {
			touches[i] = append(touches[i], "tests/a.test.ts")
		}
		if i >= 1 && i <= 9 {
			touches[i] = append(touches[i], "docs/qualification/run.ts")
		}
	}
	touches[9] = append(touches[9], "old/big.ts")
	touches[0] = []string{"src/quiet.ts"}
	for i := range 24 {
		touches[0] = append(touches[0], fmt.Sprintf("src/other%d.ts", i))
	}
	source.Commits, source.Since = history(touches, 10), []string{"c10"}
	f := detect(t, source)
	want := []hotspot{{"src/big.ts", 3, 3, "x 100 lines"}, {"tests/a.test.ts", 4, 4, "x 50 lines"}, {"src/busy.ts", 8, 7, "x 10 lines"}}
	if got := rows(f); !slices.Equal(got, want) {
		t.Errorf("hotspots = %+v, want %+v", got, want)
	}
	for _, s := range f.Statistics {
		if s.Name != "hotspot" || s.Unit != "changes" {
			t.Errorf("row %+v, want name hotspot and unit changes", s)
		}
	}
	if len(f.Remarks) != 3 || f.Remarks[0] != (answer.Remark{Mark: "shape.file-size:src/big.ts", Text: "Also a hotspot: 3 changes x 100 lines."}) {
		t.Errorf("remarks = %+v", f.Remarks)
	}
	if len(f.Marks) != 0 || len(f.NotAnalysed) != 0 || len(f.Notes) != 1 {
		t.Errorf("marks %v, not analysed %q, notes %q; want none, none, the note", f.Marks, f.NotAnalysed, f.Notes)
	}
	ties := snapshottest.Files(nil, map[string]string{"src/tie-b.ts": snapshottest.Lines(10), "src/tie-a.ts": snapshottest.Lines(10)})
	ties.Commits = []snapshot.Commit{snapshottest.Commit("c1", "src/tie-b.ts"), snapshottest.Commit("c0", "src/tie-a.ts")}
	if got := rows(detect(t, ties)); len(got) != 2 || got[0].path != "src/tie-a.ts" {
		t.Errorf("a tie = %+v, want tie-a first", got)
	}
	five := snapshottest.Files(nil, map[string]string{"a.go": "x\n", "b.go": "x\n", "c.go": "x\n", "d.go": "x\n", "e.go": "x\n"})
	five.Commits = []snapshot.Commit{snapshottest.Commit("c0", "a.go", "b.go", "c.go", "d.go", "e.go")}
	if got := rows(detect(t, five)); len(got) != 3 {
		t.Errorf("five changed files = %d rows, want 3", len(got))
	}
	own := snapshottest.Files(nil, map[string]string{
		"a.go": "x\n", "b.go": "x\n", "c.go": "x\n", "d.go": "x\n", "e.go": "x\n", ".plimsoll.toml": "[limits]\nhotspot_files = 4\n",
	})
	own.Commits = five.Commits
	if got := rows(detect(t, own)); len(got) != 4 {
		t.Errorf("five changed files and hotspot_files = 4 = %d rows, want 4", len(got))
	}
	// The largest count a repository may set.
	for i := range own.Now {
		if own.Now[i].Path == ".plimsoll.toml" {
			own.Now[i].Content = []byte("[limits]\nhotspot_files = 9007199254740992\n")
		}
	}
	if got := rows(detect(t, own)); len(got) != 5 {
		t.Errorf("five changed files and the largest hotspot_files = %d rows, want all 5", len(got))
	}
}

func TestHistoryWithoutAUsableCommitIsNotAnalysed(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{"src/a.ts": "x\n"})
	var wide []string
	for i := range 21 {
		wide = append(wide, fmt.Sprintf("src/f%d.ts", i))
	}
	source.Commits = []snapshot.Commit{snapshottest.Commit("c2", wide...), snapshottest.Commit("c1", wide...), snapshottest.Commit("c0", wide...)}
	if f := detect(t, source); len(f.Statistics) != 0 || len(f.Remarks) != 0 || !slices.Equal(f.NotAnalysed, []string{"no usable commit in the history, so no hotspots"}) {
		t.Errorf("no usable commit: %+v", f)
	}
	source.Commits = []snapshot.Commit{snapshottest.Commit("c0", "README.md")}
	if f := detect(t, source); !slices.Equal(f.NotAnalysed, []string{"no code file tracked now changed in a usable commit, so no hotspots"}) {
		t.Errorf("only a README changed: not analysed %q", f.NotAnalysed)
	}
}

func TestHotspotsRunWithoutALanguageEntry(t *testing.T) {
	source := snapshottest.Files(nil, map[string]string{"bin/a.sh": snapshottest.Lines(3), "bin/b.sh": snapshottest.Lines(2)})
	source.Commits = []snapshot.Commit{
		snapshottest.Commit("c3", "bin/a.sh"), snapshottest.Commit("c2", "bin/a.sh"), snapshottest.Commit("c1", "bin/a.sh", "bin/b.sh"), snapshottest.Commit("c0", "bin/a.sh"),
	}
	if got := rows(detect(t, source)); len(got) != 2 || got[0] != (hotspot{"bin/a.sh", 4, 4, "x 3 lines"}) || got[1].changes != 1 {
		t.Errorf("shell scripts: hotspots %+v", got)
	}
}
