package snapshot_test

import (
	"fmt"
	"io/fs"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// repeats lists the pairs the rule found: "<path> <name> <- <counterpart path> <counterpart name> <status>".
func repeats(t *testing.T, base, now map[string]string) ([]string, snapshot.FunctionRepeats) {
	t.Helper()
	found := snapshot.Build(snapshottest.Files(base, now), conventions.Default(), snapshottest.Readings(t)).Repeats
	var lines []string
	for _, r := range found.Listed {
		lines = append(lines, fmt.Sprintf("%s %s <- %s %s %s", r.Path, r.Name, r.Counterpart.Path, r.Counterpart.Name, r.Counterpart.Status))
	}
	return lines, found
}

// TestBuildFindsRepeatsInEveryLanguage copies each language's sample file: the copy beside the original repeats it,
// and the copy that replaced it moved it. The rule's sizes are lowered to the samples' small functions.
func TestBuildFindsRepeatsInEveryLanguage(t *testing.T) {
	rules := conventions.Default()
	rules.FunctionRules.Repeats.MinTokens, rules.FunctionRules.Repeats.Stretch = 15, 15
	for _, path := range []string{"src/a.go", "src/a.py", "src/a.ts", "src/a.tsx", "src/a.rs", "src/a.dart", "src/A.java", "src/a.kt", "src/A.cs", "src/a.swift", "src/a.c", "src/a.cpp"} {
		raw, err := fs.ReadFile(os.DirFS("testdata/functions/now"), path)
		if err != nil {
			t.Fatal(err)
		}
		content, copied := string(raw), "src/copy/"+path[len("src/"):]
		build := func(base, now map[string]string) snapshot.FunctionRepeats {
			return snapshot.Build(snapshottest.Files(base, now), rules, snapshottest.Readings(t)).Repeats
		}
		beside := build(map[string]string{path: content}, map[string]string{path: content, copied: content})
		if len(beside.Listed) == 0 || beside.Listed[0].Path != copied || beside.Listed[0].Counterpart.Path != path {
			t.Errorf("Build(%s copied beside it).Repeats = %+v, want its functions repeating the original", path, beside.Listed)
		}
		moved := build(map[string]string{path: content}, map[string]string{copied: content})
		if len(moved.Listed) != 0 || moved.Moved == 0 {
			t.Errorf("Build(%s moved to %s).Repeats = %d listed, %d moved; want none listed, its functions moved", path, copied, len(moved.Listed), moved.Moved)
		}
	}
}

// TestRepeatsFromKeptReadingsEqualFreshOnes asks a run of questions on one set of kept readings: no candidate, a copy,
// an edit of the copy, and a header whose language flips with the head tree's majority; each answer equals a fresh one.
func TestRepeatsFromKeptReadingsEqualFreshOnes(t *testing.T) {
	header := "int count(int *rows, int n) {\n  int total = 0;\n  for (int i = 0; i < n; i++) {\n    if (rows[i] > 0) { total += rows[i]; }\n    else if (rows[i] < -100) { return -1; }\n  }\n  return total;\n}\n"
	rules := conventions.Default()
	rules.FunctionRules.Repeats.MinTokens, rules.FunctionRules.Repeats.Stretch = 15, 15
	base := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header}
	questions := []map[string]string{
		{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header, "pkg/z.go": snapshottest.GoFile(snapshottest.GoFiles("Other"))},
		{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header, "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))},
		{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header, "pkg/b.go": snapshottest.GoFile(strings.Replace(snapshottest.GoColumns("Copied"), "v*2", "v*7", 1))},
		{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header, "inc/b.h": header, "src/x.c": "int x(void) { return 1; }\n"},
		{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "inc/a.h": header, "inc/b.h": header, "src/x.cpp": "int x() { return 1; }\n", "src/y.cpp": "int y() { return 2; }\n"},
	}
	kept := snapshottest.Readings(t)
	for i, now := range questions {
		got := snapshot.Build(snapshottest.Files(base, now), rules, kept).Repeats
		fresh := snapshot.Build(snapshottest.Files(base, now), rules, snapshottest.Readings(t)).Repeats
		if fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", fresh) {
			t.Errorf("question %d: Build with kept readings = %+v\nwith fresh ones = %+v", i, got, fresh)
		}
		// the same question twice at the same time on readings both share, none kept yet
		shared := snapshottest.Readings(t)
		var both [2]snapshot.FunctionRepeats
		var wg sync.WaitGroup
		for k := range both {
			wg.Go(func() { both[k] = snapshot.Build(snapshottest.Files(base, now), rules, shared).Repeats })
		}
		wg.Wait()
		if fmt.Sprintf("%+v", both[0]) != fmt.Sprintf("%+v", fresh) || fmt.Sprintf("%+v", both[1]) != fmt.Sprintf("%+v", fresh) {
			t.Errorf("question %d: two Builds at the same time = %+v and %+v\nwant %+v", i, both[0], both[1], fresh)
		}
		if (i == 0) != (len(got.Listed) == 0) {
			t.Errorf("question %d: Build listed %d pairs, want pairs in every question but the first", i, len(got.Listed))
		}
	}
}

// TestBuildMarksTheOutermostOfNestedRepeats copies a function with a function inside it: both repeat their originals,
// only the outer one is listed.
func TestBuildMarksTheOutermostOfNestedRepeats(t *testing.T) {
	rules := conventions.Default()
	rules.FunctionRules.Repeats.MinTokens, rules.FunctionRules.Repeats.Stretch = 15, 15
	outer := func(name string) string {
		return "export function " + name + "(x: number): number {\n  function inner(y: number): number {\n    const z = y * 2 + 1;\n" +
			"    if (z > 10) {\n      return z - 3;\n    }\n    return z + 4;\n  }\n  return inner(x) + inner(x + 1);\n}\n"
	}
	base := map[string]string{"src/a.ts": outer("first")}
	now := map[string]string{"src/a.ts": outer("first"), "src/b.ts": outer("second")}
	got := snapshot.Build(snapshottest.Files(base, now), rules, snapshottest.Readings(t)).Repeats
	if len(got.Listed) != 1 || got.Listed[0].Name != "second" {
		t.Errorf("Build(a nested copy).Repeats = %+v, want only src/b.ts second listed", got.Listed)
	}
}

// TestRepeatLimitsHoldAtTheirValues sets each limit to a measured pair's own number, where it still marks, and one step
// past it, where it does not.
func TestRepeatLimitsHoldAtTheirValues(t *testing.T) {
	// the copy differs from its original in a few lines, so share and stretch are below 1 and its length
	edited := strings.NewReplacer("v*2", "v*2 + float64(n)", "v*3", "math.Abs(v*3)").Replace(snapshottest.GoColumns("Copied"))
	base := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))}
	now := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "pkg/b.go": snapshottest.GoFile(edited)}
	build := func(rules conventions.Rules) snapshot.FunctionRepeats {
		return snapshot.Build(snapshottest.Files(base, now), rules, snapshottest.Readings(t)).Repeats
	}
	measured := build(conventions.Default())
	if len(measured.Listed) != 1 {
		t.Fatalf("Build(an edited copy) = %+v, want one pair", measured.Listed)
	}
	p := measured.Listed[0]
	limits := []struct {
		name string
		set  func(r *conventions.RepeatRules, past bool)
	}{
		{"tokens", func(r *conventions.RepeatRules, past bool) { r.MinTokens = p.Tokens + b2i(past) }},
		{"lines", func(r *conventions.RepeatRules, past bool) { r.MinLines = p.End - p.Start + 1 + b2i(past) }},
		{"share", func(r *conventions.RepeatRules, past bool) { r.Share = p.Share + 1e-9*float64(b2i(past)) }},
		{"stretch", func(r *conventions.RepeatRules, past bool) { r.Stretch = p.Stretch + b2i(past) }},
	}
	for _, l := range limits {
		for _, past := range []bool{false, true} {
			rules := conventions.Default()
			l.set(&rules.FunctionRules.Repeats, past)
			if got := len(build(rules).Listed); got != 1-b2i(past) {
				t.Errorf("Build(%s limit at the pair's value, past it: %v) listed %d, want %d", l.name, past, got, 1-b2i(past))
			}
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestBuildComparesNoRepeatsWithoutTheCodeItNeeds(t *testing.T) {
	leftOut := map[string]string{"pkg/a_test.go": "package p\n", "vendor/lib/x.go": "package lib\n"}
	base := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))}
	now := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))}
	maps.Copy(base, leftOut)
	maps.Copy(now, leftOut)
	for _, tt := range []struct {
		unavailable []string
		listed      int
	}{
		{[]string{"pkg/old.go"}, 0},      // a changed code file whose base the clone lacks: nothing is compared
		{[]string{"data/big.txt"}, 1},    // no code: the rule runs
		{[]string{"pkg/a_test.go"}, 1},   // test code, which the rule leaves out on both sides: it runs
		{[]string{"vendor/lib/x.go"}, 1}, // vendored code, left out too
	} {
		source := snapshottest.Files(base, now)
		source.Unavailable = tt.unavailable
		got := snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Repeats
		if len(got.Listed) != tt.listed || (tt.listed == 0) != (len(got.Unavailable) > 0) {
			t.Errorf("Build(unavailable %q).Repeats = %d listed, unavailable %q; want %d listed", tt.unavailable, len(got.Listed), got.Unavailable, tt.listed)
		}
	}
}

// TestBuildComparesNoFunctionOfAFileTheParserCannotRead puts a file over the parser's memory cap at the base or now:
// its old functions cannot be told from new ones, nor its gone ones from kept ones, so it is no candidate, no
// counterpart and no source of moved code, and it is named.
func TestBuildComparesNoFunctionOfAFileTheParserCannotRead(t *testing.T) {
	var big strings.Builder
	for i := range 150_000 { // measured: the Go grammar needs more than its 128 MiB for this shape from 100,000 lines
		fmt.Fprintf(&big, "var v%d = %d\n", i, i)
	}
	original, copied := snapshottest.GoFile(snapshottest.GoColumns("Original")), snapshottest.GoFile(snapshottest.GoColumns("Copied"))
	for _, tt := range []struct {
		name      string
		base, now map[string]string
	}{
		{"a candidate whose base the parser cannot read", map[string]string{"pkg/a.go": original + big.String(), "pkg/c.go": copied},
			map[string]string{"pkg/a.go": original, "pkg/c.go": copied}},
		{"a counterpart whose base the parser cannot read", map[string]string{"pkg/a.go": original + big.String()},
			map[string]string{"pkg/a.go": original, "pkg/c.go": copied}},
		{"a file the parser cannot read now", map[string]string{"pkg/a.go": original},
			map[string]string{"pkg/a.go": original + big.String(), "pkg/c.go": copied}},
	} {
		got := snapshot.Build(snapshottest.Files(tt.base, tt.now), conventions.Default(), snapshottest.Readings(t)).Repeats
		if len(got.Listed) != 0 || got.Moved != 0 || !slices.Contains(got.Unwalked, "pkg/a.go") {
			t.Errorf("Build(%s).Repeats = %d listed, %d moved, unwalked %q; want none listed, none moved, pkg/a.go unwalked",
				tt.name, len(got.Listed), got.Moved, got.Unwalked)
		}
	}
}

// methods is a Go file of two methods named Run: first one of type A with the given code, then one of type B
// holding the code of GoColumns.
func methods(first string) string {
	return snapshottest.GoFile(strings.Replace(first, "func Run(", "func (A) Run(", 1), strings.Replace(snapshottest.GoColumns("Run"), "func Run(", "func (B) Run(", 1))
}

// TestBuildCountsWhatItCompared checks the counts beside the pairs: two new functions compared, both repeat, and the
// later one has a second counterpart that also met the rule.
func TestBuildCountsWhatItCompared(t *testing.T) {
	base := map[string]string{"pkg/z.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))}
	now := map[string]string{"pkg/z.go": base["pkg/z.go"], "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("NewOne")),
		"pkg/c.go": snapshottest.GoFile(snapshottest.GoColumns("NewTwo"))}
	_, got := repeats(t, base, now)
	if got.Files != 2 || got.Compared != 2 || got.Found != 2 || len(got.Listed) != 2 || got.Listed[0].Others != 0 || got.Listed[1].Others != 1 {
		t.Errorf("Build(two copies of one function).Repeats = files %d, compared %d, found %d, listed %+v; want 2, 2, 2, others 0 and 1",
			got.Files, got.Compared, got.Found, got.Listed)
	}
}

// edited is GoColumns with a few lines changed: it still repeats GoColumns, below a share of 1.
func edited(name string) string {
	return strings.NewReplacer("v*2", "v*2 + float64(n)", "v*3", "math.Abs(v*3)").Replace(snapshottest.GoColumns(name))
}

func TestBuildFindsAFunctionThatRepeatsAnother(t *testing.T) {
	foreign := "// Copyright (c) Meta Platforms, Inc. and affiliates.\n"
	original := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))}
	with := func(extra map[string]string) map[string]string {
		m := map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))}
		maps.Copy(m, extra)
		return m
	}
	tests := []struct {
		name      string
		base, now map[string]string
		want      []string
	}{
		{"a copy in a new file", original, with(map[string]string{"pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))}),
			[]string{"pkg/b.go Copied <- pkg/a.go Original unchanged"}},
		{"a function moved and renamed", map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"), snapshottest.GoFiles("ToMove"))},
			with(map[string]string{"pkg/c.go": snapshottest.GoFile(snapshottest.GoFiles("Moved"))}), nil},
		{"a copy added beside its original", original,
			map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original"), snapshottest.GoColumns("Copied"))},
			[]string{"pkg/a.go Copied <- pkg/a.go Original unchanged"}},
		{"a counterpart that was there wins over a new one before it", map[string]string{"pkg/z.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))},
			map[string]string{"pkg/z.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("NewOne")),
				"pkg/c.go": snapshottest.GoFile(snapshottest.GoColumns("NewTwo"))},
			[]string{"pkg/b.go NewOne <- pkg/z.go Original unchanged", "pkg/c.go NewTwo <- pkg/z.go Original unchanged"}},
		{"the fuller of two counterparts that were there", map[string]string{"pkg/a.go": snapshottest.GoFile(edited("Near")), "pkg/z.go": snapshottest.GoFile(snapshottest.GoColumns("Original"))},
			map[string]string{"pkg/a.go": snapshottest.GoFile(edited("Near")), "pkg/z.go": snapshottest.GoFile(snapshottest.GoColumns("Original")),
				"pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))},
			[]string{"pkg/b.go Copied <- pkg/z.go Original unchanged"}},
		{"a copy in test support", original, with(map[string]string{"pkg/testing/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))}), nil},
		{"a copy in vendored code", original, with(map[string]string{"3rdparty/lib/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))}), nil},
		{"two like functions added together", nil, map[string]string{"pkg/x.go": snapshottest.GoFile(snapshottest.GoColumns("First")), "pkg/y.go": snapshottest.GoFile(snapshottest.GoColumns("Second"))},
			[]string{"pkg/y.go Second <- pkg/x.go First new"}},
		{"a counterpart that was there wins", original,
			with(map[string]string{"pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("NewOne")), "pkg/c.go": snapshottest.GoFile(snapshottest.GoColumns("NewTwo"))}),
			[]string{"pkg/b.go NewOne <- pkg/a.go Original unchanged", "pkg/c.go NewTwo <- pkg/a.go Original unchanged"}},
		{"a copy in a folder with its own licence", original,
			with(map[string]string{"pkg/lib/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied")), "pkg/lib/LICENSE": "zlib\n"}), nil},
		{"a copy whose file came from another project", original,
			with(map[string]string{"pkg/b.go": foreign + snapshottest.GoFile(snapshottest.GoColumns("Copied"))}), nil},
		{"a counterpart whose file came from another project", map[string]string{"pkg/a.go": foreign + snapshottest.GoFile(snapshottest.GoColumns("Original"))},
			map[string]string{"pkg/a.go": foreign + snapshottest.GoFile(snapshottest.GoColumns("Original")), "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))},
			[]string{"pkg/b.go Copied <- pkg/a.go Original unchanged"}},
		{"a counterpart after a namesake the change edited", map[string]string{"pkg/a.go": methods(snapshottest.GoFiles("Run"))},
			map[string]string{"pkg/a.go": methods(strings.Replace(snapshottest.GoFiles("Run"), "keep(", "!keep(", 1)), "pkg/b.go": snapshottest.GoFile(snapshottest.GoColumns("Copied"))},
			[]string{"pkg/b.go Copied <- pkg/a.go Run unchanged"}},
		{"a function the change rewrote", map[string]string{"pkg/a.go": snapshottest.GoFile(snapshottest.GoColumns("Original")), "pkg/b.go": snapshottest.GoFile(snapshottest.GoFiles("Rewritten"))},
			with(map[string]string{"pkg/b.go": snapshottest.GoFile(strings.Replace(snapshottest.GoColumns("Original"), "Original", "Rewritten", 1))}), nil},
		{"a long switch beside a short one", map[string]string{"pkg/s.go": snapshottest.GoFile(snapshottest.GoCases("isHook", 8))},
			map[string]string{"pkg/s.go": snapshottest.GoFile(snapshottest.GoCases("isHook", 8)), "pkg/e.go": snapshottest.GoFile(snapshottest.GoCases("isEntity", 60))}, nil},
	}
	for _, tt := range tests {
		got, _ := repeats(t, tt.base, tt.now)
		if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
			t.Errorf("Build(%s).Repeats = %q, want %q", tt.name, got, tt.want)
		}
	}
}
