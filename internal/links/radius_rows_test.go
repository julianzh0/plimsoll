package links

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// countedRadius counts the radius of a walk's result over Go alone, listing at most three paths; known are the files whose
// imports are read.
func countedRadius(r snapshot.Radius, known ...string) (*answer.Radius, []answer.Statistic, []string) {
	s := snapshot.Snapshot{Radius: r}
	s.Limits.RadiusPaths = conventions.Limit{Value: 3}
	g := graph{known: map[string]bool{}}
	for _, k := range known {
		g.known[k] = true
	}
	return blastRadius(s, []snapshot.LanguageImports{{Language: "go"}}, g)
}

func notes(rows []answer.Statistic) []string {
	var n []string
	for _, r := range rows {
		n = append(n, r.Name+": "+r.Note)
	}
	return n
}

// Every state of a changed file, every role of a user and every reason a number is a lower bound, with the exact
// structured radius, rows and "not analysed" entries.
func TestTheRadiusOfEveryKindOfChangedFileAndUser(t *testing.T) {
	r, rows, notAnalysed := countedRadius(snapshot.Radius{
		Changed: []snapshot.ChangedFile{
			{Path: "a.go", Language: "go", State: snapshot.ByName},
			{Path: "b.go", Language: "go", State: snapshot.AsAWhole},
			{BasePath: "c.go", Language: "go", State: snapshot.Deleted},
			{Path: "d.go", Language: "go", State: snapshot.NothingAltered},
			{Path: "e.ts", Language: "typescript", State: snapshot.ByName}, // its links are not used
			{Path: "g.go", Language: "go", State: snapshot.NotCounted, Reason: snapshot.ReasonGenerated},
			{Path: "i.go", Language: "go", State: snapshot.NotCounted, Reason: snapshot.ReasonIgnored},
		},
		Users: []snapshot.User{
			{Path: "p1.go", Role: snapshot.Product, ByName: []string{"go"}, Outside: []string{"go"}},
			{Path: "p2.go", Role: snapshot.Product, Whole: []string{"go"}},
			{Path: "t_test.go", Role: snapshot.Test, ByName: []string{"go"}, Outside: []string{"go"}},
			{Path: "v.go", Role: snapshot.Product, ByName: []string{"go"}}, // the name only in its tests
			{Path: "o.go", Role: snapshot.Other, Whole: []string{"go"}},
			{Path: "x.ts", Role: snapshot.Product, ByName: []string{"typescript"}}, // reached from a gated language only
		},
		Touched: []snapshot.User{
			{Path: "a.go", Role: snapshot.Product, ByName: []string{"go"}, Outside: []string{"go"}},
			{Path: "e.ts", Role: snapshot.Product, ByName: []string{"typescript"}},
			{Path: "q.go", Role: snapshot.Product, ByName: []string{"go"}}, // its imports are not read
		},
	}, "p1.go", "p2.go", "t_test.go", "v.go", "o.go", "x.ts", "a.go", "e.ts")
	want := answer.Radius{State: answer.RadiusAtLeast, Files: 5, ByName: 3, AsWhole: 2, Product: 2, Tests: 2, Other: 1,
		InChange: 1, Paths: []string{"p1.go", "p2.go", "t_test.go"}, Omitted: 2, Folders: []answer.FolderCount{},
		Changed: answer.ChangedFiles{All: 7, ByName: 1, AsWhole: 1, Deleted: 1, NothingAltered: 1, NotCounted: 3}}
	if !reflect.DeepEqual(*r, want) {
		t.Errorf("radius = %+v\nwant     %+v", *r, want)
	}
	if want := []string{
		"blast radius: at least 5 files (2 product, 2 test, 1 other); 2 of them count because a changed file counts as a " +
			"whole; 7 changed code files: 1 altered names, 1 as a whole, 1 deleted, 1 altered no name, 3 not counted; " +
			"1 more user is a file the change touched",
		"blast radius files: `p1.go`, `p2.go`, `t_test.go` and 2 more (0 product, 1 test, 1 other)",
	}; !slices.Equal(notes(rows), want) {
		t.Errorf("rows = %q\nwant   %q", notes(rows), want)
	}
	if want := []string{
		"the blast radius leaves out 1 changed code file (generated): `g.go`",
		"the blast radius leaves out 1 changed code file (" + snapshot.ReasonIgnored + "): `i.go`",
	}; !slices.Equal(notAnalysed, want) {
		t.Errorf("not analysed = %q, want %q", notAnalysed, want)
	}
}

// The words of the number for each state: exact, a lower bound with and without files, not analysed, no code change.
func TestTheRadiusRowSaysWhatTheNumberIs(t *testing.T) {
	for _, tc := range []struct {
		label string
		r     snapshot.Radius
		known []string
		want  []string
	}{
		{"exact", snapshot.Radius{
			Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.ByName}},
			Users:   []snapshot.User{{Path: "u.go", Role: snapshot.Product, ByName: []string{"go"}, Outside: []string{"go"}}},
		}, []string{"u.go"}, []string{
			"blast radius: 1 file (1 product, 0 test); 1 changed code file: 1 altered names",
			"blast radius files: `u.go`",
		}},
		{"a lower bound without files", snapshot.Radius{
			Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.ByName}},
			Users:   []snapshot.User{{Path: "u.go", Role: snapshot.Product, ByName: []string{"go"}}},
		}, nil, []string{
			"blast radius: 0 files found, an incomplete count; 1 changed code file: 1 altered names",
		}},
		{"nothing altered", snapshot.Radius{
			Changed: []snapshot.ChangedFile{{Path: "a.go", Language: "go", State: snapshot.NothingAltered}},
		}, nil, []string{"blast radius: 0 files; 1 changed code file: 1 altered no name"}},
		{"not analysed", snapshot.Radius{
			Changed: []snapshot.ChangedFile{{Path: "g.go", Language: "go", State: snapshot.NotCounted, Reason: snapshot.ReasonGenerated}},
			Users:   []snapshot.User{{Path: "u.go", Role: snapshot.Product, ByName: []string{"go"}, Outside: []string{"go"}}},
		}, []string{"u.go"}, []string{"blast radius: not analysed; 1 changed code file: 1 not counted"}},
		{"no code change, content missing", snapshot.Radius{MissingNow: []string{"go"}}, nil, []string{
			"blast radius: 0 files found, an incomplete count; no code file changed",
		}},
		{"no code change", snapshot.Radius{}, nil, []string{"blast radius: 0 files; no code file changed"}},
	} {
		_, rows, _ := countedRadius(tc.r, tc.known...)
		if !slices.Equal(notes(rows), tc.want) {
			t.Errorf("%s: rows = %q\nwant %q", tc.label, notes(rows), tc.want)
		}
	}
}

// The importers of the change in exact words: each role, the folders with the root among them, the product share, and
// the changed packages ranked by their direct importers.
func TestTheImportersOfTheChangeInExactWords(t *testing.T) {
	pkg := func(name, imports, body string) string {
		return "package " + name + "\n\nimport \"" + goModule + "/" + imports + "\"\n\n" + body + "\n"
	}
	base := map[string]string{
		"go.mod":               "module " + goModule + "\n",
		"internal/a/a.go":      "package a\n\nfunc Run() int { return 1 }\n",
		"internal/z/z.go":      "package z\n\nfunc Zed() int { return 1 }\n",
		"internal/b/b.go":      pkg("b", "internal/z", "var B = z.Zed()"),
		"internal/b/b_test.go": pkg("b", "internal/z", "var t = z.Zed()"),
		"examples/e/e.go":      pkg("e", "internal/z", "var E = z.Zed()"),
		"internal/y/y.go":      pkg("y", "internal/a", "var Y = a.Run()"),
		"main.go":              pkg("main", "internal/b", "func main() { _ = b.B }"),
	}
	now := maps.Clone(base)
	now["internal/a/a.go"] = "package a\n\nfunc Run() int { return 2 }\n"
	now["internal/z/z.go"] = "package z\n\nfunc Zed() int { return 2 }\n"
	f := detect(t, snapshottest.Files(base, now))
	var got []string
	for _, s := range f.Statistics {
		if s.Name == "importers of the change" || s.Name == "direct importers of" {
			got = append(got, s.Name+" "+s.Subject+": "+s.Note)
		}
	}
	want := []string{
		"importers of the change : 4 files directly (2 product, 1 test, 1 other), 5 at any distance (3 product, 1 test, 1 other); " +
			"in `internal/b/` 2, the root 1, `examples/e/` 1, 1 in other folders; 3 of 5 product code files",
		"direct importers of internal/z/: 3 files; 1 product, 1 test, 1 other",
		"direct importers of internal/a/: 1 file; 1 product, 0 test",
	}
	if !slices.Equal(got, want) {
		t.Errorf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !slices.Contains(f.Notes, radiusNote) {
		t.Errorf("notes %q, want the blast radius note: a changed declaration's users are counted", f.Notes)
	}
	commented := maps.Clone(base)
	commented["internal/a/a.go"] += "// a comment\n"
	if f := detect(t, snapshottest.Files(base, commented)); slices.Contains(f.Notes, radiusNote) {
		t.Errorf("a comment alone changed: notes %q, want no blast radius note", f.Notes)
	}
}
