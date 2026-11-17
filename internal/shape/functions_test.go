package shape

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func TestDetectMarksFunctionsAChangeMadeComplexOrDeeper(t *testing.T) {
	fn := snapshottest.Function
	rules := conventions.Default()
	one := detect(t, snapshottest.Files(nil, map[string]string{"src/sync.ts": fn("syncAll", 7, 0)}), rules)
	got := marksOf(one, ruleComplexity)
	wantHint := "read lines 1–17 (17 of them added by this change): it nests 7 levels deep at line 8; counted: 1 for each if, loop, switch, catch and conditional expression plus 1 per level it is nested in; 1 for each else, else-if and run of one boolean operator; functions written inside it and markup are not counted."
	if len(got) != 1 || got[0].ID != "shape.function-complexity:src/sync.ts:syncAll" || got[0].Change != answer.New || got[0].Confidence != answer.Candidate ||
		got[0].Subject.Kind != "function" || !slices.Equal(got[0].Subject.Paths, []string{"src/sync.ts"}) || !slices.Equal(got[0].Subject.Names, []string{"syncAll"}) ||
		got[0].Measured.Value != 28 || got[0].Measured.Before != nil || *got[0].Measured.Limit != 25 || got[0].Measured.Unit != "cognitive complexity" || got[0].Hint != wantHint {
		t.Errorf("Detect(a new complex function) complexity marks = %+v, want one new candidate of 28 with hint %q", got, wantHint)
	}
	if row, found := statistic(one, "functions over the complexity limit", ""); !found || row.Note != "1 listed" || len(one.Summaries) != 0 {
		t.Errorf("Detect(one function listed) = row %+v, summaries %q; want note \"1 listed\" and no summary", row, one.Summaries)
	}

	twice := func(text string) string {
		var out, run []string
		for _, line := range strings.SplitAfter(text, "\n") {
			if strings.HasPrefix(line, "  if (a === ") && strings.HasSuffix(line, "}\n") {
				run = append(run, line)
				continue
			}
			out = append(out, run...)
			out = append(out, run...)
			run = nil
			out = append(out, line)
		}
		return strings.Join(out, "")
	}
	repeated := marksOf(detect(t, snapshottest.Files(map[string]string{"src/a.ts": fn("grow", 5, 8)}, map[string]string{"src/a.ts": twice(fn("grow", 5, 8))}), rules), ruleComplexity)
	if want := "read lines 1–29 (8 of them added by this change): it nests 5 levels deep at line 6, and lines 12–15 are repeated at lines 20–23; "; len(repeated) != 1 || !strings.HasPrefix(repeated[0].Hint, want) {
		t.Errorf("Detect(a pasted block) complexity marks = %+v, want a hint starting %q", repeated, want)
	}

	walk := marksOf(detect(t, snapshottest.Files(map[string]string{"src/a.ts": fn("walk", 3, 0)}, map[string]string{"src/a.ts": fn("walk", 4, 0)}), rules), ruleNesting)
	if len(walk) != 1 || walk[0].ID != "shape.function-nesting:src/a.ts:walk" || walk[0].Change != answer.New || walk[0].Measured.Value != 4 ||
		walk[0].Measured.Before == nil || *walk[0].Measured.Before != 3 || *walk[0].Measured.Limit != 4 || walk[0].Measured.Unit != "levels" ||
		walk[0].Hint != "read lines 1–11 and count the control structures around line 5." {
		t.Errorf("Detect(nesting 3 to 4) nesting marks = %+v, want one new item of 4 from 3", walk)
	}
	deeper := marksOf(detect(t, snapshottest.Files(map[string]string{"src/a.ts": fn("walk", 4, 0)}, map[string]string{"src/a.ts": fn("walk", 5, 0)}), rules), ruleNesting)
	if len(deeper) != 1 || deeper[0].Change != answer.Grown {
		t.Errorf("Detect(nesting 4 to 5) nesting marks = %+v, want one grown item", deeper)
	}

	runs := marksOf(detect(t, snapshottest.Files(nil, map[string]string{"src/run.ts": fn("run", 7, 0) + fn("run", 7, 0)}), rules), ruleComplexity)
	var ids []string
	for _, m := range runs {
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"shape.function-complexity:src/run.ts:run", "shape.function-complexity:src/run.ts:run#2"}) {
		t.Errorf("Detect(two functions named run) ids = %q, want run and run#2", ids)
	}

	four := detect(t, snapshottest.Files(nil, map[string]string{"src/four.ts": fn("a", 7, 0) + "\n" + fn("b", 8, 0) + "\n" + fn("c", 9, 0) + "\n" + fn("d", 7, 1)}), rules)
	rows := map[string]string{
		"functions measured":                  "in 1 changed product file",
		"functions over the complexity limit": "3 listed (at most three per measure, the largest rise first)",
		"functions nested deeper":             "1 listed",
	}
	for row, note := range rows {
		if s, found := statistic(four, row, ""); !found || s.Note != note {
			t.Errorf("Detect(four over the limit) row %q = %+v, want note %q", row, s, note)
		}
	}
	// the value is what crossed; the note says how many are listed
	if s, found := statistic(four, "functions over the complexity limit", ""); !found || s.Value == nil || *s.Value != 4 {
		t.Errorf("Detect(four over the limit) row \"functions over the complexity limit\" = %+v, want value 4", s)
	}
	summary := "Function limits: 3 of 4 functions over the complexity limit are listed (at most three per measure, the largest rise first); `report` counts the rest."
	if len(marksOf(four, ruleComplexity)) != 3 || len(marksOf(four, ruleNesting)) != 1 || !slices.Contains(four.Summaries, summary) {
		t.Errorf("Detect(four over the limit) = %d complexity, %d nesting, summaries %q; want 3, 1 and %q",
			len(marksOf(four, ruleComplexity)), len(marksOf(four, ruleNesting)), four.Summaries, summary)
	}

	copied := detect(t, snapshottest.Files(map[string]string{"src/old.ts": fn("seven", 7, 0)}, map[string]string{"src/old.ts": fn("seven", 7, 0), "src/new.ts": fn("seven", 7, 0)}), rules)
	if s, _ := statistic(copied, "functions left out as code that existed before", ""); s.Note != "1: 1 found at the base (`src/new.ts` `seven` in `src/old.ts`)" {
		t.Errorf("Detect(a copy) left-out row = %+v, want the copy named", s)
	}
	moved := detect(t, snapshottest.Files(map[string]string{"src/a.ts": fn("all", 7, 3)}, map[string]string{"src/a.ts": fn("all", 0, 3), "src/b.ts": fn("part", 7, 0)}), rules)
	if s, _ := statistic(moved, "functions left out as code that existed before", ""); s.Note != "1: 1 moved within the change" {
		t.Errorf("Detect(moved code) left-out row = %+v, want one moved", s)
	}

	var big strings.Builder
	big.WriteString("package main\n")
	for i := range 150_000 {
		fmt.Fprintf(&big, "var v%d = %d\n", i, i)
	}
	broken := fn("grow", 5, 4)
	first := strings.Index(broken, "\n") + 1
	var ten strings.Builder
	for i := 1; i <= 20; i++ {
		ten.WriteString(fn(fmt.Sprintf("c%02d", i), 8, 0))
	}
	notAnalysed := []struct {
		base, now map[string]string
		want      string
	}{
		{map[string]string{"src/a.ts": broken[:first] + "    const = 1 +;\n" + broken[first:]}, map[string]string{"src/a.ts": fn("grow", 6, 6)},
			"1 function could not be read by the parser, now or at the base (a macro or a syntax its grammar does not know), and is not measured: in `src/a.ts`"},
		{map[string]string{"src/a.ts": broken[:first] + "    const = 1 +;\n" + broken[first:] + strings.Replace(broken[:first], "grow", "keep", 1) + "    const = 1 +;\n" + broken[first:]},
			map[string]string{"src/a.ts": fn("grow", 6, 6) + fn("keep", 6, 6)},
			"2 functions could not be read by the parser, now or at the base (a macro or a syntax its grammar does not know), and are not measured: 2 in `src/a.ts`"},
		{map[string]string{"src/a.ts": broken[:first] + "    const = 1 +;\n" + broken[first:] + strings.Replace(broken[:first], "grow", "keep", 1) + "    const = 1 +;\n" + broken[first:],
			"src/b.ts": broken[:first] + "    const = 1 +;\n" + broken[first:]},
			map[string]string{"src/a.ts": fn("grow", 6, 6) + fn("keep", 6, 6), "src/b.ts": fn("grow", 6, 6)},
			"3 functions could not be read by the parser, now or at the base (a macro or a syntax its grammar does not know), and are not measured: most in `src/a.ts` (2)"},
		{nil, map[string]string{"src/min.ts": strings.ReplaceAll(fn("seven", 7, 0), "\n", " ")},
			"1 changed file has more than 110 bytes a line on average, so its functions were not measured: `src/min.ts`"},
		{nil, map[string]string{"big.go": big.String()}, "the parser could not read the functions of 1 changed file: `big.go`"},
		{nil, map[string]string{"big.c": strings.ReplaceAll(strings.ReplaceAll(big.String(), "package main\n", ""), "var v", "int v")},
			"the parser could not read the functions of 1 changed file: `big.c`"},
		{map[string]string{"src/lib.ts": ten.String()}, map[string]string{"src/lib.ts": ten.String(), "src/new.ts": ten.String() + fn("fresh", 7, 0)},
			"1 function over a function limit was not compared with existing code (at most 10 per measure are), so it is not listed"},
	}
	for _, tc := range notAnalysed {
		if got := detect(t, snapshottest.Files(tc.base, tc.now), rules); !slices.Contains(got.NotAnalysed, tc.want) {
			t.Errorf("Detect(%v).NotAnalysed = %q, want it to hold %q", tc.now, got.NotAnalysed, tc.want)
		}
	}
}
