package answer_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/links"
	"github.com/julianzh0/plimsoll/internal/shape"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

func questions(t testing.TB, source snapshot.Source, err error) (answer.Questions, *bool) {
	t.Helper()
	called := false
	return answer.Questions{
		Take: func(context.Context, string, string) (snapshot.Snapshot, error) {
			called = true
			if err != nil {
				return snapshot.Snapshot{}, err
			}
			return snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)), nil
		},
		Detectors: []answer.Detector{shape.Detector{}},
		Version:   "0.1.0",
	}, &called
}

func grown() snapshot.Source {
	source := snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Lines(900)}, map[string]string{"src/a.ts": snapshottest.Lines(1620)})
	source.AskedBase, source.Base, source.Head = "main", strings.Repeat("a", 40), strings.Repeat("b", 40)
	return source
}

func TestQuestionsAnswerWithARealDetector(t *testing.T) {
	q, _ := questions(t, grown(), nil)
	status, err := q.Status(context.Background(), "/repository", "main")
	if err != nil || status.Question != answer.Status || status.Version != "0.1.0" || len(status.Detectors) != 1 ||
		len(status.Marks) != 0 || len(status.ReportOnly) != 1 || status.ReportOnly[0].ID != "shape.file-size:src/a.ts" || status.Sections != nil {
		t.Fatalf("Status = %+v, %v; want one size mark, in report only, and no sections", status, err)
	}
	report, err := q.Report(context.Background(), "/repository", "main", nil)
	if err != nil || report.Question != answer.Report || len(report.Marks) != 0 || len(report.ReportOnly) != 1 || len(report.Sections) != 1 ||
		report.Sections[0].Statistics[0].Name != "product lines" {
		t.Fatalf("Report = %+v, %v; want the mark and one shape section", report, err)
	}
	if text := q.Text(status); !strings.HasPrefix(text, "plimsoll 0.1.0 status: base `") {
		t.Errorf("Text(status) = %q, want the header first", text)
	}
}

func TestQuestionsRefuseInOnePlainSentence(t *testing.T) {
	q, called := questions(t, grown(), nil)
	_, err := q.Status(context.Background(), "relative/path", "main")
	if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != "`repository` must be an absolute path." || *called {
		t.Errorf("Status(relative path) = %v, take called %v; want the absolute-path refusal before reading", err, *called)
	}
	file := ".plimsoll.toml"
	cases := []struct {
		refusal snapshot.Refusal
		want    string
	}{
		{snapshot.Refusal{Kind: snapshot.NotARepository, Detail: "nope"}, "`repository` does not name a folder inside a git working tree."},
		{snapshot.Refusal{Kind: snapshot.NoCommit, Detail: "nope"}, "The repository has no commit yet, so there is nothing to compare."},
		{snapshot.Refusal{Kind: snapshot.UnknownBase, Detail: "nope"}, "Unknown base `nope`: git finds no such commit in this repository."},
		{snapshot.Refusal{Kind: snapshot.NoCommonHistory, Detail: "nope"}, "Base `nope` shares no history with the head commit."},
		{snapshot.Refusal{Kind: snapshot.GitFailed, Detail: "nope"}, "git could not read the repository: `nope`."},
		{snapshot.Refusal{Kind: snapshot.NoBase, File: file}, "The question names no base, and no tracked `.plimsoll.toml` declares a default base."},
		{snapshot.Refusal{Kind: snapshot.NoBase, Detail: "`base` is not text", File: file},
			"The question names no base, and the default base of `.plimsoll.toml` cannot be used: `base` is not text."},
		{snapshot.Refusal{Kind: snapshot.UnknownDeclaredBase, Detail: "main", File: file},
			"Unknown base `main`, the default base `.plimsoll.toml` declares: git finds no branch, remote branch or tag of this name in this repository."},
	}
	for _, c := range cases {
		q, _ := questions(t, snapshot.Source{}, &c.refusal)
		_, err := q.Report(context.Background(), "/repository", "nope", nil)
		if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != c.want {
			t.Errorf("Report(refusal %+v) = %v, want %q", c.refusal, err, c.want)
		}
	}
	boom := errors.New("boom")
	q, _ = questions(t, snapshot.Source{}, boom)
	if _, err := q.Status(context.Background(), "/repository", "main"); !errors.Is(err, boom) {
		t.Errorf("Status(other error) = %v, want it returned as it is", err)
	}
}

var forbidden = regexp.MustCompile(`(?i)\b(should|must|remove|delete|fix|refactor|consider|no code reads|never read|unused|dead|healthy|clean|score|grade|coupling|coupled|story|slice|closure record|disposition)\b`)

func sampleAnswers(t *testing.T) (answer.Questions, []answer.Answer) {
	t.Helper()
	files := func(n int) map[string]string {
		return map[string]string{"src/a.ts": snapshottest.Lines(n), "tests/a.ts": snapshottest.Lines(n * 3 / 2)}
	}
	sources := []snapshot.Source{
		snapshottest.Files(files(10), files(10)),
		grown(),
		snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Lines(1600)}, nil),
		snapshottest.Files(map[string]string{"src/a.ts": "x\n", "tests/a.ts": snapshottest.Lines(19)}, map[string]string{"src/a.ts": "x\n", "tests/a.ts": snapshottest.Lines(25)}),
		snapshottest.Files(map[string]string{"package.json": `{"dependencies": {}}`, ".env.example": "ALPHA=1\n"},
			map[string]string{"package.json": `{"dependencies": {"pg": "8"}}`, ".env.example": "ALPHA=1\nBETA=2\n"}),
	}
	sources = append(sources, detectorSamples()...)
	var all []answer.Answer
	var q answer.Questions
	for _, source := range sources {
		q, _ = questions(t, source, nil)
		q.Detectors = everyDetector()
		for _, ask := range []func(context.Context, string, string) (answer.Answer, error){q.Status, func(ctx context.Context, repository, base string) (answer.Answer, error) {
			return q.Report(ctx, repository, base, nil)
		}} {
			a, err := ask(context.Background(), "/repository", "main")
			if err != nil {
				t.Fatal(err)
			}
			all = append(all, a)
		}
	}
	return q, all
}

func TestNoAnswerUsesForbiddenWords(t *testing.T) {
	q, answers := sampleAnswers(t)
	for _, a := range answers {
		if found := forbidden.FindString(q.Text(a)); found != "" {
			t.Errorf("the %s text holds %q:\n%s", a.Question, found, q.Text(a))
		}
	}
}

func TestSampleAnswersAreTheSameFromTheirOwnJSON(t *testing.T) {
	q, answers := sampleAnswers(t)
	for _, a := range answers {
		data, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		var again answer.Answer
		if err := json.Unmarshal(data, &again); err != nil {
			t.Fatal(err)
		}
		if q.Text(again) != q.Text(a) {
			t.Errorf("the %s text differs when rendered from its own JSON", a.Question)
		}
	}
}

func TestExplainAnswersForOneItem(t *testing.T) {
	source := snapshottest.Files(
		map[string]string{"src/new.ts": "x\n", "src/gone.ts": snapshottest.Lines(1600), "src/old.ts": snapshottest.Lines(1600)},
		map[string]string{"src/new.ts": snapshottest.Lines(1600), "src/old.ts": snapshottest.Lines(1600)},
	)
	q, _ := questions(t, source, nil)
	report, err := q.Report(context.Background(), "/repository", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"shape.file-size:src/new.ts", "shape.file-size:src/gone.ts", "shape.file-size:src/old.ts"} {
		a, err := q.Explain(context.Background(), "/repository", "main", id)
		items := len(a.Marks) + len(a.ReportOnly) + len(a.Resolved) + len(a.Standing)
		if err != nil || a.Question != answer.Explain || items != 1 || a.Footprint != nil || a.Sections != nil || a.Radius != nil ||
			a.Evidence == nil || a.Evidence.Mark != id || a.Counts != report.Counts {
			t.Errorf("Explain(%s) = %+v, %v; want that item alone with its evidence and the report's counts", id, a, err)
		}
	}
}

func TestExplainOfALongMarkEndsWithATruncationLine(t *testing.T) {
	files := map[string]string{"src/a.ts": snapshottest.Lines(10)}
	for i := range 300 {
		files[fmt.Sprintf("tests/t%03d.ts", i)] = "x\n"
	}
	q, _ := questions(t, snapshottest.Files(nil, files), nil)
	a, err := q.Explain(context.Background(), "/repository", "main", "shape.test-ratio:repository")
	if err != nil || len(a.Evidence.Lines) != 180 || a.Evidence.Omitted == 0 {
		t.Fatalf("Explain(ratio with 300 test files) = %+v, %v; want 180 lines and some omitted", a.Evidence, err)
	}
	text := q.Text(a)
	if lines := strings.Count(text, "\n"); lines > 200 || !strings.HasSuffix(text, fmt.Sprintf("... %d more evidence lines not shown.\n", a.Evidence.Omitted)) {
		t.Errorf("Text(explain) has %d lines and ends %q; want at most 200 and the truncation line", lines, text[len(text)-60:])
	}
}

func TestExplainRefusesAnUnknownMark(t *testing.T) {
	q, _ := questions(t, grown(), nil)
	_, err := q.Explain(context.Background(), "/repository", "main", "shape.file-size:nope.ts")
	want := "Unknown mark `shape.file-size:nope.ts`: nothing with this identifier exists against this base now. `report` lists the current identifiers."
	if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != want {
		t.Errorf("Explain(unknown mark) = %v, want %q", err, want)
	}
}

func TestReportTakesNamedFiles(t *testing.T) {
	q, called := questions(t, snapshottest.Files(nil, map[string]string{"src/a.ts": "import \"./b.ts\";\n", "src/b.ts": "export const b = 1;\n"}), nil)
	q.Detectors = []answer.Detector{shape.Detector{}, links.Detector{}}
	named := func(a answer.Answer) []string {
		var subjects []string
		for _, section := range a.Sections {
			for _, s := range section.Statistics {
				if section.Detector == "links" && s.Name == "named file" {
					subjects = append(subjects, s.Subject)
				}
			}
		}
		return subjects
	}
	report, err := q.Report(context.Background(), "/repository", "main", []string{"src/b.ts", "./src/a.ts", "src/b.ts"})
	if err != nil || !slices.Equal(named(report), []string{"src/a.ts", "src/b.ts"}) {
		t.Errorf("Report(files) named rows = %q, %v; want src/a.ts and src/b.ts once each", named(report), err)
	}
	for _, files := range [][]string{nil, {}} {
		if report, _ := q.Report(context.Background(), "/repository", "main", files); len(named(report)) != 0 {
			t.Errorf("Report(%q) has named rows %q, want none", files, named(report))
		}
	}
	for _, files := range [][]string{{"/abs/x.ts"}, {"../x.ts"}} {
		*called = false
		_, err := q.Report(context.Background(), "/repository", "main", files)
		refusal, ok := errors.AsType[*answer.Refusal](err)
		if !ok || refusal.Sentence != "Every entry of `files` must be a path relative to the repository's top folder." || *called {
			t.Errorf("Report(%q) = %v, read %v; want the refusal before the repository is read", files, err, *called)
		}
	}
	if report, err := q.Report(context.Background(), "/repository", "main", []string{"src/", ""}); err != nil || len(named(report)) != 2 {
		t.Errorf("Report(src/, empty) = %q, %v; want two named rows", named(report), err)
	}
}

func TestExplainSaysAnItemIsAcknowledged(t *testing.T) {
	source := snapshottest.Files(
		map[string]string{"src/new.ts": "x\n", "src/old.ts": snapshottest.Lines(1600)},
		map[string]string{"src/new.ts": snapshottest.Lines(1600), "src/old.ts": snapshottest.Lines(1600),
			".plimsoll.toml": "[acknowledged]\n\"shape.file-size:src/old.ts\" = \"one parser, one file\"\n\"shape.file-size:src/gone.ts\" = \"x\"\n"},
	)
	q, _ := questions(t, source, nil)
	a, err := q.Explain(context.Background(), "/repository", "main", "shape.file-size:src/old.ts")
	if err != nil || len(a.Acknowledged) != 1 || len(a.Standing) != 0 || a.AcknowledgedIn != ".plimsoll.toml" ||
		!strings.Contains(q.Text(a), "acknowledged in `.plimsoll.toml`: `report` lists it under \"Acknowledged\"") {
		t.Errorf("Explain(acknowledged) = %+v, %v; want the item under acknowledged, named so in the text", a, err)
	}
	other, err := q.Explain(context.Background(), "/repository", "main", "shape.file-size:src/new.ts")
	if err != nil || len(other.Acknowledged) != 0 || other.AcknowledgedIn != "" {
		t.Errorf("Explain(another item) = acknowledged %v in %q, %v; want none", other.Acknowledged, other.AcknowledgedIn, err)
	}
	if _, err := q.Explain(context.Background(), "/repository", "main", "shape.file-size:src/gone.ts"); err == nil {
		t.Error("Explain(a stale entry's identifier) = no error, want it refused as unknown")
	}
}

// radiusDetector gives a blast radius and the size marks of shape, as the link detector gives a radius.
type radiusDetector struct{ shape.Detector }

func (d radiusDetector) Detect(s snapshot.Snapshot) answer.Findings {
	f := d.Detector.Detect(s)
	f.Radius = &answer.Radius{State: answer.RadiusCounted, Files: 1, ByName: 1, Product: 1, Paths: []string{"src/b.ts"}}
	return f
}

func TestStatusAndReportCarryTheBlastRadiusAndExplainLeavesItOut(t *testing.T) {
	q, _ := questions(t, grown(), nil)
	q.Detectors = []answer.Detector{radiusDetector{}}
	status, errStatus := q.Status(context.Background(), "/repository", "main")
	report, errReport := q.Report(context.Background(), "/repository", "main", nil)
	explained, errExplain := q.Explain(context.Background(), "/repository", "main", "shape.file-size:src/a.ts")
	if errors.Join(errStatus, errReport, errExplain) != nil || status.Radius == nil || report.Radius == nil || status.Radius.Files != 1 ||
		explained.Radius != nil {
		t.Errorf("radius of status, report, explain = %+v, %+v, %+v; want the radius in status and report only", status.Radius, report.Radius, explained.Radius)
	}
}
