package answer

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func sample() snapshot.Snapshot {
	ratio := 2.5
	return snapshot.Snapshot{
		AskedBase: "main", Base: "1a2b3c4d5e" + strings.Repeat("0", 30), Head: "9f8e7d6c5b" + strings.Repeat("0", 30),
		Modified: true,
		Footprint: snapshot.Footprint{
			Commits: 5, Files: 5, Product: snapshot.Lines{Added: 120, Removed: 80}, Test: snapshot.Lines{Added: 300, Removed: 10},
			Docs: snapshot.Lines{Added: 40}, TestPerProduct: &ratio,
		},
		History:    snapshot.History{Read: 12, Usable: 11},
		Unverified: []snapshot.Extension{{Suffix: ".sh", Files: 2, Lines: 7}},
		Untracked:  3, Missing: []string{"data/big.bin"}, Unparsed: []string{"src/big.ts", "src/huge.ts"}, SyntaxErrors: []string{"src/a.py"},
		Areas: conventions.Default().Areas,
	}
}

func fileMark(path string, change Change) Mark {
	return Mark{
		ID: ID("shape", "file-size", path), Change: change, Confidence: Exact, Subject: Subject{Kind: "file", Paths: []string{path}},
		Hint: "count the file's lines at the base and now.",
	}
}

func findings() Findings {
	return Findings{
		Marks: []Mark{
			fileMark("src/a.ts", New),
			{ID: "shape.test-ratio:repository", Change: Grown, Confidence: Exact, Subject: Subject{Kind: "repository"}, Hint: "the hint."},
			fileMark("src/b.ts", Standing),
		},
		NotAnalysed: []string{"no `.env.example`, so no settings count"},
	}
}

func wording() Wording {
	return Wording{"shape": {
		Mark:     func(m Mark) string { return "sentence of " + m.ID },
		Standing: func(items []Mark) string { return fmt.Sprintf("%d standing", len(items)) },
	}}
}

func TestStatusWithMarksReadsWordForWord(t *testing.T) {
	want := "plimsoll 0.1.0 status: base `main` 1a2b3c4d5e, head 9f8e7d6c5b (working tree modified)\n" +
		"Areas: 1 worse (size 2); not analysed: dependencies, wiring, functions, settings, process\n" +
		"1 new, 1 grown, 0 resolved\n" +
		"Footprint of the change since the base: 5 commits, 5 files; product +120 -80, test +300 -10, documentation +40 -0; 2.5 test lines added per product line added.\n" +
		"\n" +
		"1. [new, exact] sentence of shape.file-size:src/a.ts\n" +
		"   verify: count the file's lines at the base and now. id: shape.file-size:src/a.ts\n" +
		"2. [grown, exact] sentence of shape.test-ratio:repository\n" +
		"   verify: the hint. id: shape.test-ratio:repository\n" +
		"\n" +
		"Standing (already at the base, not grown): 1 standing.\n" +
		"History: 12 commits read, 11 usable.\n" +
		"Detectors run: shape.\n" +
		"Not analysed: no `.env.example`, so no settings count; not verified for `.sh` (7 lines in 2 files); 3 untracked files; content not in this partial clone, so its lines and history are not counted: 1 file (`data/big.bin`); the parser could not read 2 files, so their imports, test code and settings reads are not counted (a file may need at most 128 MiB to parse): `src/big.ts`, `src/huge.ts` and 1 more in `report`.\n"
	got := Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}), wording())
	if got != want {
		t.Fatalf("Render(status) =\n%s\nwant\n%s", got, want)
	}
}

func TestAnswerWithoutMarksReadsWordForWord(t *testing.T) {
	commit := "1a2b3c4d5e" + strings.Repeat("0", 30)
	s := snapshot.Snapshot{AskedBase: "main", Base: commit, Head: commit, History: snapshot.History{Read: 12, Usable: 11},
		Areas: conventions.Default().Areas}
	ran := Findings{RanRules: []string{"shape.test-ratio", "shape.dependencies", "links.cycle", "shape.function-complexity", "process.planning-file"},
		Radius: &Radius{State: RadiusCounted, Paths: []string{}}}
	want := "plimsoll 0.1.0 status: base `main` 1a2b3c4d5e, head 1a2b3c4d5e\n" +
		"Areas: none worse; not analysed: settings\n" +
		"Blast radius: 0 files outside the change use what it altered or removed\n" +
		"0 new, 0 grown, 0 resolved\n" +
		"Footprint of the change since the base: 0 commits, 0 files; product +0 -0, test +0 -0, documentation +0 -0; no product line added.\n" +
		"\n" +
		"No new or grown marks since `main` (0 commits, 0 files compared).\n" +
		"Standing (already at the base, not grown): none.\n" +
		"History: 12 commits read, 11 usable.\n" +
		"Detectors run: shape.\n" +
		"Not analysed: none.\n"
	if got := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", ran}}), wording()); got != want {
		t.Fatalf("Render(no marks) =\n%s\nwant\n%s", got, want)
	}
}

func reportFindings() Findings {
	value := func(v float64) *float64 { return &v }
	f := findings()
	f.Marks = append(f.Marks, fileMark("src/c.ts", Resolved))
	f.Statistics = []Statistic{
		{Name: "product lines", Value: value(19465), Before: value(19345), Unit: "lines"},
		{Name: "test ratio", Value: value(3.8), Before: value(3.7), Unit: "ratio"},
		{Name: "largest code file", Subject: "src/a.ts", Value: value(1620), Before: value(1480), Unit: "lines"},
		{Name: "largest code file", Subject: "src/new.ts", Value: value(900), Unit: "lines"},
		{Name: "runtime dependencies", Subject: "package.json", Value: value(6), Before: value(5), Note: "express, ioredis, pg"},
	}
	f.Notes = []string{"test code recognized by: `tests/`"}
	return f
}

func TestReportListsEverythingInAFixedLayout(t *testing.T) {
	report := Render(Assemble(Report, "0.1.0", sample(), []Result{{"shape", reportFindings()}}), wording())
	wantTail := "\n" +
		"Resolved:\n" +
		"- sentence of shape.file-size:src/c.ts\n" +
		"  verify: count the file's lines at the base and now. id: shape.file-size:src/c.ts\n" +
		"Touched but not created by this change: none.\n" +
		"\n" +
		"shape\n" +
		"  product lines: 19,465 lines (net +120)\n" +
		"  test ratio: 3.8 (+0.1)\n" +
		"  largest code file `src/a.ts`: 1,620 lines (net +140)\n" +
		"  largest code file `src/new.ts`: 900 lines (not at the base)\n" +
		"  runtime dependencies `package.json`: 6 (net +1); express, ioredis, pg\n" +
		"  test code recognized by: `tests/`\n" +
		"\n" +
		"Standing (already at the base, not grown):\n" +
		"- sentence of shape.file-size:src/b.ts id: shape.file-size:src/b.ts\n" +
		"History: 12 commits read, 11 usable.\n" +
		"Detectors run: shape.\n" +
		"Not analysed:\n" +
		"- shape: no `.env.example`, so no settings count\n" +
		"- repository: not verified for `.sh` (7 lines in 2 files)\n" +
		"- repository: 3 untracked files\n" +
		"- repository: content not in this partial clone, so its lines and history are not counted: 1 file (`data/big.bin`)\n" +
		"- repository: the parser could not read 2 files, so their imports, test code and settings reads are not counted (a file may need at most 128 MiB to parse): `src/big.ts`, `src/huge.ts`\n" +
		"- repository: 1 changed file holds a syntax error, so an import after the error may be missed: `src/a.py`\n"
	if !strings.HasSuffix(report, wantTail) {
		t.Fatalf("Render(report) =\n%s\nwant it to end with\n%s", report, wantTail)
	}
	lines := strings.Split(report, "\n")
	status := strings.Split(Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}), wording()), "\n")
	if !strings.Contains(lines[0], "report:") || !strings.HasPrefix(lines[1], "Areas: ") || lines[3] != status[3] {
		t.Errorf("Render(report) header = %q, areas %q, footprint %q; want report, its areas and the footprint of status %q", lines[0], lines[1], lines[3], status[3])
	}
}

func TestReportGroupsALongRunOfStandingItems(t *testing.T) {
	var f Findings
	for _, path := range []string{"docs/a/x1.md", "docs/a/x2.md", "docs/b/y.md", "s1.ts", "src/s2.ts", "src/s3.ts", "src/s4.ts", "docs/b/deep/z.md", "src/s5.ts"} {
		f.Marks = append(f.Marks, fileMark(path, Standing))
	}
	f.Marks = append(f.Marks, Mark{ID: "shape.test-ratio:repository", Change: Standing, Confidence: Exact, Subject: Subject{Kind: "repository"}, Hint: "h."})
	for i := range 7 {
		name := fmt.Sprintf("S%d", i)
		f.Marks = append(f.Marks, Mark{ID: ID("shape", "named", name), Change: Standing, Confidence: Candidate, Subject: Subject{Kind: "setting", Names: []string{name}}, Hint: "h."})
	}
	report := Render(Assemble(Report, "0.1.0", sample(), []Result{{"shape", f}}), wording())
	start := strings.Index(report, "Standing (already at the base, not grown):\n")
	end := strings.Index(report, "History:")
	want := "Standing (already at the base, not grown):\n" +
		"- sentence of shape.file-size:docs/a/x1.md id: shape.file-size:docs/a/x1.md\n" +
		"- sentence of shape.file-size:docs/a/x2.md id: shape.file-size:docs/a/x2.md\n" +
		"- sentence of shape.file-size:docs/b/y.md id: shape.file-size:docs/b/y.md\n" +
		"- sentence of shape.file-size:s1.ts id: shape.file-size:s1.ts\n" +
		"- sentence of shape.file-size:src/s2.ts id: shape.file-size:src/s2.ts\n" +
		"- 4 more `shape.file-size` items, in `src/` 3, `docs/b/` 1\n" +
		"- sentence of shape.test-ratio:repository id: shape.test-ratio:repository\n" +
		"- sentence of shape.named:S0 id: shape.named:S0\n" +
		"- sentence of shape.named:S1 id: shape.named:S1\n" +
		"- sentence of shape.named:S2 id: shape.named:S2\n" +
		"- sentence of shape.named:S3 id: shape.named:S3\n" +
		"- sentence of shape.named:S4 id: shape.named:S4\n" +
		"- 2 more `shape.named` items\n"
	if start < 0 || end < start || report[start:end] != want {
		t.Fatalf("Render(report) standing =\n%s\nwant\n%s", report[max(start, 0):max(end, 0)], want)
	}
	if a := Assemble(Report, "0.1.0", sample(), []Result{{"shape", f}}); len(a.Standing) != 17 {
		t.Errorf("Assemble kept %d standing marks, want all 17", len(a.Standing))
	}
}

func TestStatusShowsFiveMarksAndStaysInsideFortyLines(t *testing.T) {
	var f Findings
	for i := range 12 {
		f.Marks = append(f.Marks, fileMark(fmt.Sprintf("src/new%02d.ts", i), New))
	}
	for i := range 10 {
		f.Marks = append(f.Marks, fileMark(fmt.Sprintf("src/gone%02d.ts", i), Resolved), fileMark(fmt.Sprintf("src/old%02d.ts", i), Standing))
	}
	for i := range 30 {
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("entry %d", i))
	}
	a := Assemble(Status, "0.1.0", sample(), []Result{{"shape", f}})
	if len(a.Marks) != 5 || a.More != 7 || a.Counts != (Counts{New: 12, Resolved: 10}) {
		t.Fatalf("Assemble(status) = %d marks, %d more, counts %+v; want 5, 7, {12 0 10}", len(a.Marks), a.More, a.Counts)
	}
	text := Render(a, wording())
	if lines := strings.Count(text, "\n"); lines > 40 {
		t.Errorf("Render(status) has %d lines, want at most 40", lines)
	}
	for _, want := range []string{"7 more: `report` lists them.", "and 7 more in `report`", "and 30 more in `report`"} {
		if !strings.Contains(text, want) {
			t.Errorf("Render(status) lacks %q:\n%s", want, text)
		}
	}
}

func TestStandingLeavesOutASubjectShownAsAMark(t *testing.T) {
	subject := Subject{Kind: "repository"}
	f := Findings{Marks: []Mark{
		{ID: "shape.test-ratio:repository", Change: Grown, Subject: subject},
		{ID: "shape.other:repository", Change: Standing, Subject: subject},
	}}
	if a := Assemble(Status, "0.1.0", sample(), []Result{{"shape", f}}); len(a.Standing) != 0 {
		t.Errorf("Assemble.Standing = %v, want the item left out", a.Standing)
	}
}

func TestTextIsTheSameWhenRenderedFromItsOwnJSON(t *testing.T) {
	answers := []Answer{
		Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}),
		Assemble(Report, "0.1.0", sample(), []Result{{"shape", reportFindings()}}),
		Assemble(Status, "0.1.0", snapshot.Snapshot{}, []Result{{"shape", Findings{}}}),
	}
	for _, a := range answers {
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var again Answer
		if err := json.Unmarshal(data, &again); err != nil {
			t.Fatal(err)
		}
		if Render(again, wording()) != Render(a, wording()) {
			t.Errorf("Render from JSON differs for the %s answer", a.Question)
		}
		if strings.Contains(string(data), "null") {
			t.Errorf("the %s answer's JSON holds null: %s", a.Question, data)
		}
	}
}

func TestMergeBaseAndSingularsAreSaid(t *testing.T) {
	s := sample()
	s.MergeBase = true
	s.Footprint = snapshot.Footprint{Commits: 1, Files: 1}
	text := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", Findings{}}}), wording())
	for _, want := range []string{"base `main` (merge base with the head) 1a2b3c4d5e", "1 commit, 1 file;", "(1 commit, 1 file compared)"} {
		if !strings.Contains(text, want) {
			t.Errorf("Render lacks %q:\n%s", want, text)
		}
	}
	plain, err := json.Marshal(Assemble(Status, "0.1.0", s, []Result{{"shape", Findings{}}}))
	if err != nil || strings.Contains(string(plain), "declaredIn") {
		t.Errorf("a base the question names: JSON %s, %v; want no declaredIn", plain, err)
	}
	s.BaseDeclared, s.OwnFile = true, ".plimsoll.toml"
	declared := Assemble(Status, "0.1.0", s, []Result{{"shape", Findings{}}})
	if text := Render(declared, wording()); !strings.Contains(text, "base `main` (declared in `.plimsoll.toml`; merge base with the head) 1a2b3c4d5e") {
		t.Errorf("Render(declared, merge base) lacks the declared header:\n%s", text)
	}
	if data, err := json.Marshal(declared); err != nil || !strings.Contains(string(data), `"declaredIn":".plimsoll.toml"`) {
		t.Errorf("JSON of a declared base = %s, %v; want declaredIn", data, err)
	}
	s.MergeBase = false
	if text := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", Findings{}}}), wording()); !strings.Contains(text, "base `main` (declared in `.plimsoll.toml`) 1a2b3c4d5e") {
		t.Errorf("Render(declared) lacks the declared header:\n%s", text)
	}
	s.BaseDeclared, s.MergeBase = false, true
	one := 1.0
	zero := 0.0
	report := Assemble(Report, "0.1.0", s, []Result{{"shape", Findings{Statistics: []Statistic{{Name: "product lines", Value: &one, Before: &zero, Unit: "lines"}}}}})
	if text := Render(report, wording()); !strings.Contains(text, "  product lines: 1 line (net +1)\n") {
		t.Errorf("Render(one line) lacks the singular:\n%s", text)
	}
}

// TestAnAnswerShortensButNeverHides renders a large answer of two detectors in every change kind and checks that each
// mark the text leaves out is counted: the numbers always add up.
func TestAnAnswerShortensButNeverHides(t *testing.T) {
	w := Wording{}
	var results []Result
	total := map[Change]int{}
	for _, detector := range []string{"shape", "links"} {
		w[detector] = Phrases{
			Mark:     func(m Mark) string { return "sentence of " + m.ID },
			Standing: func(items []Mark) string { return fmt.Sprintf("%d standing of %s", len(items), detector) },
		}
		var f Findings
		for _, rule := range []string{"one", "two"} {
			for i, change := range []Change{New, Grown, Resolved, Standing} {
				for n := range 4 + 3*i {
					p := fmt.Sprintf("dir%d/sub%d/%s/f%d.ts", n%3, n%2, change, n)
					f.Marks = append(f.Marks, Mark{ID: ID(detector, rule, p), Change: change, Confidence: Exact, Subject: Subject{Kind: "file", Paths: []string{p}}, Hint: "h."})
					total[change]++
				}
			}
		}
		for n := range 9 {
			f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("gap %d of %s", n, detector))
		}
		results = append(results, Result{detector, f})
	}
	moreLine := regexp.MustCompile("(?m)^- (\\d+) more `[^`]+` items")
	counted := func(text string, re *regexp.Regexp) int {
		n := 0
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			v, _ := strconv.Atoi(m[1])
			n += v
		}
		return n
	}

	reportAnswer := Assemble(Report, "0.1.0", sample(), results)
	if got := len(reportAnswer.Marks) + len(reportAnswer.Resolved) + len(reportAnswer.Standing); got != total[New]+total[Grown]+total[Resolved]+total[Standing] {
		t.Errorf("report holds %d marks, want every one", got)
	}
	report := Render(reportAnswer, w)
	standingListed := strings.Count(report[strings.Index(report, "Standing (already"):], "- sentence of ")
	if got := standingListed + counted(report, moreLine); got != total[Standing] {
		t.Errorf("report text: %d standing listed + counted, want %d", got, total[Standing])
	}
	for _, m := range append(slices.Clone(reportAnswer.Marks), reportAnswer.Resolved...) {
		if !strings.Contains(report, "id: "+m.ID+"\n") {
			t.Errorf("report text leaves out %s", m.ID)
		}
	}

	statusAnswer := Assemble(Status, "0.1.0", sample(), results)
	status := Render(statusAnswer, w)
	if got := len(statusAnswer.Marks) + statusAnswer.More; got != total[New]+total[Grown] || !strings.Contains(status, fmt.Sprintf("%d more: `report` lists them.", statusAnswer.More)) {
		t.Errorf("status: %d marks + %d more, want %d new and grown counted", len(statusAnswer.Marks), statusAnswer.More, total[New]+total[Grown])
	}
	resolvedLine := status[strings.Index(status, "Resolved: "):]
	resolvedLine = resolvedLine[:strings.Index(resolvedLine, "\n")]
	if got := strings.Count(resolvedLine, "; ") + 1 + counted(resolvedLine, regexp.MustCompile(` and (\d+) more in`)); got != total[Resolved] {
		t.Errorf("status resolved line %q counts %d, want %d", resolvedLine, got, total[Resolved])
	}
	if got := counted(status, regexp.MustCompile(`(\d+) standing of`)); got != total[Standing] {
		t.Errorf("status standing line counts %d, want %d", got, total[Standing])
	}
	gaps := status[strings.Index(status, "Not analysed: "):]
	if got := strings.Count(gaps, "gap ") + counted(gaps, regexp.MustCompile(` and (\d+) more in`)); got != len(statusAnswer.NotAnalysed) {
		t.Errorf("status not-analysed line counts %d, want %d", got, len(statusAnswer.NotAnalysed))
	}
}

func TestStatusCountsTheMarksItDoesNotShow(t *testing.T) {
	var facts Findings
	for i := range 3 {
		facts.Marks = append(facts.Marks, fileMark(fmt.Sprintf("src/new%02d.ts", i), New))
	}
	plain := Assemble(Status, "0.1.0", sample(), []Result{{"shape", Findings{Marks: slices.Concat(facts.Marks, facts.Marks)}}})
	if text := Render(plain, wording()); !strings.Contains(text, "1 more: `report` lists them.") {
		t.Errorf("Render(status) without context marks = %s, want the plain line", text)
	}
}

func TestTouchedItemsAreCountedInStatusAndListedInReport(t *testing.T) {
	f := findings()
	f.Marks = append(f.Marks, fileMark("src/t.ts", Touched), fileMark("src/u.ts", Touched))
	status := Assemble(Status, "0.1.0", sample(), []Result{{"shape", f}})
	if status.Counts != (Counts{New: 1, Grown: 1}) || len(status.Marks) != 2 || status.More != 0 || len(status.Touched) != 2 {
		t.Fatalf("Assemble(status) = counts %+v, %d marks, %d more, %d touched; want {1 1 0}, 2, 0, 2",
			status.Counts, len(status.Marks), status.More, len(status.Touched))
	}
	line := "\nTouched but not created by this change (`report` lists them): 2 standing.\nStanding (already at the base, not grown): 1 standing.\n"
	if text := Render(status, wording()); !strings.Contains(text, line) {
		t.Errorf("Render(status) =\n%s\nwant it to hold %q", text, line)
	}
	block := "Touched but not created by this change:\n" +
		"- sentence of shape.file-size:src/t.ts\n  verify: count the file's lines at the base and now. id: shape.file-size:src/t.ts\n" +
		"- sentence of shape.file-size:src/u.ts\n  verify: count the file's lines at the base and now. id: shape.file-size:src/u.ts\n\nshape\n"
	report := Assemble(Report, "0.1.0", sample(), []Result{{"shape", f}})
	if text := Render(report, wording()); !strings.Contains(text, block) {
		t.Errorf("Render(report) =\n%s\nwant it to hold %q", text, block)
	}
	if none := Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}), wording()); strings.Contains(none, "Touched") {
		t.Errorf("Render(status without a touched item) names one:\n%s", none)
	}
}

func TestReportOnlyMarksAreCountedInStatusAndListedInReport(t *testing.T) {
	s := sample()
	s.ReportOnly = []string{"shape.test-ratio"}
	status := Assemble(Status, "0.1.0", s, []Result{{"shape", findings()}})
	if status.Counts != (Counts{New: 1}) || len(status.Marks) != 1 || len(status.ReportOnly) != 1 ||
		status.ReportOnly[0].Change != Grown {
		t.Fatalf("Assemble(status) = counts %+v, %d marks, report only %+v; want {1 0 0}, 1, the grown mark",
			status.Counts, len(status.Marks), status.ReportOnly)
	}
	if text := Render(status, wording()); !strings.Contains(text, "\n1 new, 0 grown, 0 resolved; 1 more in `report` only\n") {
		t.Errorf("Render(status) =\n%s\nwant the counts to name the report-only mark", text)
	}
	explained := status
	explained.Question = Explain
	if text := Render(explained, wording()); strings.Contains(text, " grown, ") {
		t.Errorf("Render(explain) =\n%s\nwant no counts of the whole answer", text)
	}
	line := "\nIn `report` only (not counted here): 1 standing.\n"
	if text := Render(status, wording()); !strings.Contains(text, line) {
		t.Errorf("Render(status) =\n%s\nwant it to hold %q", text, line)
	}
	block := "In `report` only (not counted in `status`):\n- sentence of shape.test-ratio:repository\n"
	if text := Render(Assemble(Report, "0.1.0", s, []Result{{"shape", findings()}}), wording()); !strings.Contains(text, block) {
		t.Errorf("Render(report) =\n%s\nwant it to hold %q", text, block)
	}
	if text := Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}), wording()); strings.Contains(text, "only") {
		t.Errorf("Render(status without a report-only kind) =\n%s\nwant no report-only line", text)
	}
}

func TestStatusPrintsEachSummaryBeforeTheStandingLine(t *testing.T) {
	f := findings()
	f.Marks = append(f.Marks, fileMark("src/t.ts", Touched), fileMark("src/u.ts", Touched))
	f.Summaries = []string{"A count; `report` lists them."}
	status := Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", f}}), wording())
	if want := "2 standing.\nA count; `report` lists them.\nStanding (already"; !strings.Contains(status, want) {
		t.Errorf("Render(status) =\n%s\nwant it to hold %q", status, want)
	}
	if report := Render(Assemble(Report, "0.1.0", sample(), []Result{{"shape", f}}), wording()); strings.Contains(report, "A count") {
		t.Errorf("Render(report) =\n%s\nwant no summary", report)
	}
	data, _ := json.Marshal(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}))
	if !strings.Contains(string(data), `"summaries":[]`) {
		t.Errorf("an answer without a summary marshals as %s, want summaries []", data)
	}
}

func TestAStatusWithoutMarksSaysWhatItLeftToTheLinesBelow(t *testing.T) {
	s := sample()
	s.ReportOnly = []string{"shape.test-ratio"}
	f := findings()
	f.Marks = f.Marks[1:2] // the grown ratio alone
	text := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", f}}), wording())
	if want := "No new or grown mark to list since `main` (5 commits, 5 files compared); the lines below count what else the change did, and `report` lists it."; !strings.Contains(text, want) {
		t.Errorf("Render(status with only a report-only mark) =\n%s\nwant %q and no all-clear", text, want)
	}
}

func TestWhatComesFirstUnderNotAnalysed(t *testing.T) {
	s := sample()
	s.SkippedChanges = []string{"`single_include/` (2 files)"}
	early := Findings{NotAnalysed: []string{"one", "two", "three", "four", "five"}}
	late := Findings{NotAnalysed: []string{"six"}, NotAnalysedFirst: []string{"a declared rule checks nothing"}}
	text := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", early}, {"links", late}}), wording())
	if want := "Not analysed: changed files in folders plimsoll skips (generated, vendored or built), not counted: `single_include/` (2 files); " +
		"a declared rule checks nothing; one; two; three"; !strings.Contains(text, want) {
		t.Errorf("Render(status) =\n%s\nwant the not-analysed line to start with %q", text, want)
	}
}

func TestAcknowledgedStandingItemsAreListedApartAndStaleEntriesNamed(t *testing.T) {
	s := sample()
	s.OwnFile = ".plimsoll.toml"
	s.Acknowledged = snapshot.Acknowledged{Table: "acknowledged",
		IDs:      []string{"shape.file-size:src/b.ts", "shape.file-size:src/a.ts", "shape.file-size:src/gone.ts"},
		Problems: []string{"1 line is no `\"identifier\" = \"reason\"` entry; it is not applied"}}
	f := findings()
	f.Marks = append(f.Marks, fileMark("src/c.ts", Standing))
	a := Assemble(Report, "0.1.0", s, []Result{{"shape", f}})
	ids := func(marks []Mark) []string {
		var out []string
		for _, m := range marks {
			out = append(out, m.ID)
		}
		return out
	}
	if !slices.Equal(ids(a.Acknowledged), []string{"shape.file-size:src/b.ts"}) || !slices.Equal(ids(a.Standing), []string{"shape.file-size:src/c.ts"}) ||
		a.Acknowledged[0].Change != Standing || a.AcknowledgedIn != ".plimsoll.toml" || !slices.Contains(ids(a.Marks), "shape.file-size:src/a.ts") {
		t.Errorf("Assemble = acknowledged %v in %q, standing %v, marks %v; want the standing src/b.ts apart, src/c.ts standing, the new src/a.ts a mark as usual",
			ids(a.Acknowledged), a.AcknowledgedIn, ids(a.Standing), ids(a.Marks))
	}
	var notAnalysed []string
	for _, n := range a.NotAnalysed {
		if n.Detector == repositoryEntry && strings.HasPrefix(n.What, "`.plimsoll.toml` `[acknowledged]`: ") {
			notAnalysed = append(notAnalysed, n.What)
		}
	}
	want := []string{"`.plimsoll.toml` `[acknowledged]`: 1 line is no `\"identifier\" = \"reason\"` entry; it is not applied",
		"`.plimsoll.toml` `[acknowledged]`: `shape.file-size:src/gone.ts` matches no item of this answer; it acknowledges nothing"}
	if !slices.Equal(notAnalysed, want) {
		t.Errorf("not analysed about the table = %q, want %q", notAnalysed, want)
	}

	onlyStale := sample()
	onlyStale.OwnFile, onlyStale.Acknowledged = ".plimsoll.toml", snapshot.Acknowledged{Table: "acknowledged", IDs: []string{"shape.file-size:src/gone.ts"}}
	if stale := Assemble(Report, "0.1.0", onlyStale, []Result{{"shape", f}}); stale.Acknowledged != nil || stale.AcknowledgedIn != "" {
		t.Errorf("only a stale entry: acknowledged %v in %q; want none and no file named", stale.Acknowledged, stale.AcknowledgedIn)
	}
	none := Assemble(Report, "0.1.0", sample(), []Result{{"shape", f}})
	if none.Acknowledged != nil || none.AcknowledgedIn != "" || len(none.Standing) != 2 {
		t.Errorf("no table: acknowledged %v in %q, standing %d; want none and both standing", none.Acknowledged, none.AcknowledgedIn, len(none.Standing))
	}
}

func TestAcknowledgedItemsAreCountedInStatusAndListedInReport(t *testing.T) {
	s := sample()
	s.OwnFile = ".plimsoll.toml"
	s.Acknowledged = snapshot.Acknowledged{Table: "acknowledged", IDs: []string{"shape.file-size:src/b.ts"}}
	status := Render(Assemble(Status, "0.1.0", s, []Result{{"shape", findings()}}), wording())
	if !strings.Contains(status, "Standing (already at the base, not grown): none.\n1 standing item acknowledged in `.plimsoll.toml`; `report` lists them.\n") {
		t.Errorf("Render(status) =\n%s\nwant standing none and the acknowledged line after it", status)
	}
	report := Render(Assemble(Report, "0.1.0", s, []Result{{"shape", findings()}}), wording())
	if !strings.Contains(report, "Standing (already at the base, not grown): none.\nAcknowledged in `.plimsoll.toml` (standing, not listed above):\n- sentence of shape.file-size:src/b.ts id: shape.file-size:src/b.ts\n") {
		t.Errorf("Render(report) =\n%s\nwant the acknowledged list after the standing one", report)
	}
	if plain := Render(Assemble(Status, "0.1.0", sample(), []Result{{"shape", findings()}}), wording()); strings.Contains(plain, "acknowledged") {
		t.Errorf("Render(status without a table) =\n%s\nwant no word about acknowledged items", plain)
	}
}

func TestTheHeadlineBlockNamesWorseBetterAndNotAnalysedAreasAndTheRadius(t *testing.T) {
	h := &Headline{Worse: 2, Better: 1, Areas: []HeadlineArea{
		{Name: "size", State: AreaPass, Resolved: []string{"shape.test-ratio:repository"}},
		{Name: "dependencies", State: AreaFail, Marks: []string{"shape.dependencies:go.mod"}},
		{Name: "wiring", State: AreaNotAnalysed},
		{Name: "functions", State: AreaFail, Marks: []string{"a", "b"}},
		{Name: "settings", State: AreaPass},
	}}
	for _, tc := range []struct {
		radius *Radius
		want   string
	}{
		{&Radius{State: RadiusCounted, Files: 50, Product: 25, Tests: 24, Other: 1},
			"Blast radius: 50 files outside the change use what it altered or removed (25 product, 24 test, 1 other)"},
		{&Radius{State: RadiusAtLeast, Files: 1, Product: 1}, "Blast radius: at least 1 file outside the change uses what it altered or removed (1 product, 0 test)"},
		{&Radius{State: RadiusAtLeast}, "Blast radius: an incomplete count: no file outside the change found that uses what it altered or removed (see Not analysed)"},
		{&Radius{State: RadiusNotAnalysed}, "Blast radius: not analysed (see Not analysed)"},
	} {
		var text strings.Builder
		headlineBlock(&text, h, tc.radius)
		want := "Areas: 2 worse (dependencies 1, functions 2); 1 better (size); not analysed: wiring\n" + tc.want + "\n"
		if text.String() != want {
			t.Errorf("block =\n%s\nwant\n%s", text.String(), want)
		}
	}
}
