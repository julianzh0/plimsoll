package answer_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func headlineMark(id string, change answer.Change) answer.Mark {
	return answer.Mark{ID: id, Change: change, Subject: answer.Subject{Kind: "file", Paths: []string{"a"}}, Confidence: answer.Exact,
		Measured: answer.Measured{Value: 1, LimitSource: "a rule", Unit: "lines"}, Hint: "read it."}
}

func assembleHeadline(q answer.Question, marks []answer.Mark, ran ...string) answer.Answer {
	rules := conventions.Default()
	s := snapshot.Snapshot{Areas: rules.Areas, ReportOnly: rules.ReportOnly}
	return answer.Assemble(q, "0", s, []answer.Result{{Detector: "all", Findings: answer.Findings{Marks: marks, RanRules: ran}}})
}

func areaOf(h *answer.Headline, name string) answer.HeadlineArea {
	for _, a := range h.Areas {
		if a.Name == name {
			return a
		}
	}
	return answer.HeadlineArea{}
}

func TestTheHeadlineCountsAreasWorseAndBetterFromDecidingKindsOnly(t *testing.T) {
	marks := []answer.Mark{
		headlineMark("shape.function-complexity:a.go:f", answer.New),
		headlineMark("shape.function-complexity:a.go:g", answer.Grown),
		headlineMark("shape.dependencies:go.mod", answer.New),
		headlineMark("shape.file-size:a.go", answer.New),             // report-only: never moves size
		headlineMark("shape.test-ratio:repository", answer.Resolved), // makes size better
		headlineMark("co-change.pair:a+b", answer.Resolved),          // report-only resolved: wiring stays as it is
		headlineMark("process.doc-churn:repository", answer.Resolved),
		headlineMark("settings.defined-twice:X", answer.New), // new and resolved in one area: it fails, not better
		headlineMark("settings.defined-twice:Y", answer.Resolved),
	}
	ran := []string{"shape.test-ratio", "shape.dependencies", "shape.function-complexity", "links.cycle", "settings.defined-twice", "process.planning-file"}
	status, report := assembleHeadline(answer.Status, marks, ran...), assembleHeadline(answer.Report, marks, ran...)
	h := status.Headline
	if h == nil || h.Worse != 3 || h.Better != 1 {
		t.Fatalf("headline = %+v, want 3 worse (functions, dependencies, settings) and 1 better (size)", h)
	}
	var names []string
	for _, a := range h.Areas {
		names = append(names, a.Name)
	}
	if want := []string{"size", "dependencies", "wiring", "functions", "settings", "process"}; !slices.Equal(names, want) {
		t.Errorf("areas %q, want %q", names, want)
	}
	if f := areaOf(h, "functions"); f.State != answer.AreaFail || !slices.Equal(f.Marks, []string{"shape.function-complexity:a.go:f", "shape.function-complexity:a.go:g"}) {
		t.Errorf("functions = %+v, want failing on both marks", f)
	}
	if size := areaOf(h, "size"); size.State != answer.AreaPass || !slices.Equal(size.Resolved, []string{"shape.test-ratio:repository"}) || len(size.Marks) != 0 {
		t.Errorf("size = %+v, want passing and better through the resolved ratio; the report-only file size moves nothing", size)
	}
	if wiring := areaOf(h, "wiring"); wiring.State != answer.AreaPass || len(wiring.Resolved) != 0 {
		t.Errorf("wiring = %+v, want passing with no resolved item: a resolved pair is report-only", wiring)
	}
	if settings := areaOf(h, "settings"); settings.State != answer.AreaFail {
		t.Errorf("settings = %+v, want failing: new and resolved in one area fails", settings)
	}
	if report.Headline == nil || !reflect.DeepEqual(*status.Headline, *report.Headline) {
		t.Errorf("status and report headlines differ: %+v and %+v", status.Headline, report.Headline)
	}
}

func TestAnAreaWhoseDecidingRulesDidNotRunIsNotAnalysed(t *testing.T) {
	h := assembleHeadline(answer.Status, nil, "shape.test-ratio", "process.planning-file").Headline
	for _, a := range h.Areas {
		want := answer.AreaNotAnalysed
		if a.Name == "size" || a.Name == "process" {
			want = answer.AreaPass
		}
		if a.State != want {
			t.Errorf("%s = %s, want %s", a.Name, a.State, want)
		}
	}
	if h := assembleHeadline(answer.Status, nil, "links.never").Headline; areaOf(h, "wiring").State != answer.AreaPass {
		t.Errorf("wiring with a checked declared rule = %s, want pass", areaOf(h, "wiring").State)
	}
}

func TestTheHeadlineListsEveryFailingMarkBeforeStatusCutsThemToFive(t *testing.T) {
	var marks []answer.Mark
	for _, id := range []string{"a.go:f", "a.go:g", "a.go:h", "b.go:f", "b.go:g"} {
		marks = append(marks, headlineMark("shape.function-complexity:"+id, answer.New))
	}
	marks = append(marks, headlineMark("shape.dependencies:go.mod", answer.New), headlineMark("links.cycle:a.go", answer.New),
		headlineMark("links.never:a->b", answer.New))
	status := assembleHeadline(answer.Status, marks)
	report := assembleHeadline(answer.Report, marks)
	if len(status.Marks) != 5 || status.More != 3 {
		t.Fatalf("status shows %d marks and %d more, want 5 and 3", len(status.Marks), status.More)
	}
	listed := 0
	for _, a := range status.Headline.Areas {
		for _, id := range a.Marks {
			listed++
			if !slices.ContainsFunc(report.Marks, func(m answer.Mark) bool { return m.ID == id }) {
				t.Errorf("headline identifier %s is no mark of report", id)
			}
		}
	}
	if listed != 8 || status.Headline.Worse != 3 {
		t.Errorf("headline lists %d identifiers over %d failing areas, want 8 over 3", listed, status.Headline.Worse)
	}
}
