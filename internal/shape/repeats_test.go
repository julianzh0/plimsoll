package shape_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/shape"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func repeatsSnapshot() snapshot.Snapshot {
	return snapshot.Snapshot{Repeats: snapshot.FunctionRepeats{
		Rules: conventions.Default().FunctionRules.Repeats, PerMeasure: 3, Files: 3, Found: 4, Compared: 5, Moved: 2, FromAnotherProject: 1,
		Searched: 120, SearchedFiles: 30,
		Listed: []snapshot.FunctionRepeat{
			{Path: "src/orders/save.ts", Name: "saveOrder", Occurrence: 1, Start: 40, End: 118, Tokens: 420,
				Counterpart: snapshot.RepeatCounterpart{Path: "src/users/save.ts", Name: "saveUser", Start: 12, End: 88,
					Tokens: 400, Status: snapshot.CounterpartUnchanged},
				Share: 0.914, Reverse: 0.9, Stretch: 286, StretchStart: 52, StretchEnd: 101, CodeLines: 75, Partnered: 71,
				Unpartnered:            []snapshot.LineRange{{Start: 44, End: 46}, {Start: 104, End: 104}},
				CounterpartUnpartnered: []snapshot.LineRange{{Start: 16, End: 18}}},
			{Path: "src/a.go", Name: "Second", Occurrence: 2, Start: 30, End: 60, Tokens: 200,
				Counterpart: snapshot.RepeatCounterpart{Path: "src/a.go", Name: "First", Start: 1, End: 28, Tokens: 210,
					Status: snapshot.CounterpartNew},
				Share: 0.85, Reverse: 0.82, Stretch: 160, StretchStart: 31, StretchEnd: 50, CodeLines: 28, GaveUp: true},
		}}}
}

func repeatMarks(s snapshot.Snapshot) []answer.Mark {
	var found []answer.Mark
	for _, m := range (shape.Detector{}).Detect(s).Marks {
		if answer.RuleOf(m.ID) == "function-repeat" {
			found = append(found, m)
		}
	}
	return found
}

func TestFunctionRepeatsAreCountedAndWhatWasNotComparedIsSaid(t *testing.T) {
	s := repeatsSnapshot()
	f := (shape.Detector{}).Detect(s)
	rows := map[string]answer.Statistic{}
	for _, st := range f.Statistics {
		rows[st.Name] = st
	}
	if strings.Contains(strings.Join(f.NotAnalysed, "\n"), "repeats") || strings.Contains(strings.Join(f.NotAnalysed, "\n"), "counterpart") {
		t.Errorf("Detect(nothing unread).NotAnalysed = %q, want no line of the repeat rule", f.NotAnalysed)
	}
	compared, repeating := rows["new functions compared for repeats"], rows["new functions that repeat another"]
	if compared.Value == nil || *compared.Value != 5 || !strings.Contains(compared.Note, "against 120 functions of any size in 30 product files") ||
		!strings.Contains(compared.Note, "left out as new: 2 moved from a function the change removed, 1 in a file from another project") ||
		!strings.Contains(compared.Note, "left out on both sides: test, demo, example, vendored and generated code, folders with their own licence") {
		t.Errorf("Detect rows: compared = %+v", compared)
	}
	if repeating.Value == nil || *repeating.Value != 4 || repeating.Note != "2 listed (at most three per measure, the most alike first: by the lower of the two shares)" {
		t.Errorf("Detect rows: repeating = %+v", repeating)
	}
	if want := "Function repeats: `report` lists 2 of the 4 new functions that repeat another (the most alike first); the other two are counted, not listed."; !slices.Contains(f.Summaries, want) {
		t.Errorf("Detect summaries = %q, want the cut list said", f.Summaries)
	}

	none := (shape.Detector{}).Detect(snapshot.Snapshot{Repeats: snapshot.FunctionRepeats{Rules: s.Repeats.Rules, Files: 2}})
	if !slices.ContainsFunc(none.Statistics, func(st answer.Statistic) bool {
		return st.Name == "new functions compared for repeats" && st.Value != nil && *st.Value == 0
	}) {
		t.Errorf("Detect(two changed files, no candidate) rows = %+v, want 0 compared", none.Statistics)
	}

	s.Repeats = snapshot.FunctionRepeats{Rules: s.Repeats.Rules, Unavailable: []string{"src/old.go"}, Unwalked: []string{"src/big.go"}, Searched: 120, Unreadable: 3}
	notAnalysed := strings.Join((shape.Detector{}).Detect(s).NotAnalysed, "\n")
	for _, want := range []string{"`src/old.go`", "`src/big.go`", "3 of the 123 functions searched for a counterpart could not be read by the parser and were not compared"} {
		if !strings.Contains(notAnalysed, want) {
			t.Errorf("Detect(unavailable, unwalked).NotAnalysed = %q, want %s named", notAnalysed, want)
		}
	}
}

func TestExplainOfARepeatShowsBothFunctionsAndCapsTheLineRanges(t *testing.T) {
	s := repeatsSnapshot()
	for i := range 12 { // more ranges than explain shows
		s.Repeats.Listed[0].Unpartnered = append(s.Repeats.Listed[0].Unpartnered, snapshot.LineRange{Start: 200 + 2*i, End: 200 + 2*i})
	}
	m := repeatMarks(s)[0]
	if !strings.Contains(m.Hint, "and 4 more") {
		t.Errorf("Hint(14 ranges) = %q, want the ranges past the cap counted", m.Hint)
	}
	got := strings.Join((shape.Detector{}).Explain(s, m), "\n")
	for _, want := range []string{
		"new in this change: lines 40–118 (420 tokens)",
		"counterpart: `src/users/save.ts` `saveUser`, lines 12–88 (400 tokens), which was there before",
		"91% of its tokens are in runs of 10 tokens the counterpart holds (the limit is 80%); 90% of the counterpart's are in its",
		"286 tokens in a row are covered, lines 52–101 (at least 150 needed)",
		"and 4 more",
		"searched: 120 functions in 30 product files",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Explain(repeat) =\n%s\nwant it to hold %q", got, want)
		}
	}
}

func TestAFunctionThatRepeatsAnotherIsMarkedWithItsCounterpart(t *testing.T) {
	marks := repeatMarks(repeatsSnapshot())
	if len(marks) != 2 {
		t.Fatalf("Detect(two repeats) = %d function-repeat marks, want 2", len(marks))
	}
	first, second := marks[0], marks[1]
	if first.ID != "shape.function-repeat:src/orders/save.ts:saveOrder" || second.ID != "shape.function-repeat:src/a.go:Second#2" {
		t.Errorf("Detect ids = %q, %q", first.ID, second.ID)
	}
	if c := first.Counterpart; c == nil || c.Path != "src/users/save.ts" || c.Name != "saveUser" || c.Status != "unchanged" {
		t.Errorf("Detect(first).Counterpart = %+v, want src/users/save.ts saveUser unchanged", c)
	}
	if first.Change != answer.New || first.Measured.Value != 91 || *first.Measured.Limit != 80 {
		t.Errorf("Detect(first) = %s %v of %v, want new 91 of 80", first.Change, first.Measured.Value, *first.Measured.Limit)
	}
	w := shape.Detector{}
	if got, want := w.Wording(first), "`src/orders/save.ts` `saveOrder` is new in this change, and 91% of its tokens are in `src/users/save.ts` `saveUser`, which was there before. Limit: 80%, plimsoll's rule, read blind on agent pull requests."; got != want {
		t.Errorf("Wording(first) =\n%s\nwant\n%s", got, want)
	}
	if got, want := w.Wording(second), "`src/a.go` `Second` is new in this change, and 85% of its tokens are in `src/a.go` `First`, also new in this change. Limit: 80%, plimsoll's rule, read blind on agent pull requests."; got != want {
		t.Errorf("Wording(second) =\n%s\nwant\n%s", got, want)
	}
	namesake := second
	namesake.Counterpart = &answer.Counterpart{Path: "src/a.go", Name: "Second", Status: "new"}
	if got, want := w.Wording(namesake), "`src/a.go` `Second` is new in this change, and 85% of its tokens are in another function of its name in the same file, also new in this change. Limit: 80%, plimsoll's rule, read blind on agent pull requests."; got != want {
		t.Errorf("Wording(a namesake in the same file) =\n%s\nwant\n%s", got, want)
	}
	if want := "compare lines 40–118 of `src/orders/save.ts` with lines 12–88 of `src/users/save.ts`: 71 of its 75 lines that hold code have a matching line there; without one: lines 44–46 and 104 here, 16–18 there; the longest covered run is 286 tokens, lines 52–101 (at least 150 needed)."; first.Hint != want {
		t.Errorf("Hint(first) =\n%s\nwant\n%s", first.Hint, want)
	}
	if !strings.Contains(second.Hint, "of the same file: the line comparison gave up") {
		t.Errorf("Hint(second) = %q, want the same file named once and the comparison's give-up said", second.Hint)
	}
}
