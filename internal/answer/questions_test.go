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

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/links"
	"github.com/juzhxng/plimsoll/internal/shape"
	"github.com/juzhxng/plimsoll/internal/snapshot"
	"github.com/juzhxng/plimsoll/internal/snapshot/snapshottest"
)

func questions(source snapshot.Source, err error) (answer.Questions, *bool) {
	called := false
	return answer.Questions{
		Take: func(context.Context, string, string) (snapshot.Snapshot, error) {
			called = true
			if err != nil {
				return snapshot.Snapshot{}, err
			}
			return snapshot.Build(source, conventions.Default()), nil
		},
		Detectors: []answer.Detector{shape.Detector{}},
		Version:   "0.1.0",
	}, &called
}

func grown() snapshot.Source {
	source := snapshottest.Files(map[string]string{"src/a.ts": snapshottest.Lines(1480)}, map[string]string{"src/a.ts": snapshottest.Lines(1620)})
	source.AskedBase, source.Base, source.Head = "main", strings.Repeat("a", 40), strings.Repeat("b", 40)
	return source
}

func TestQuestionsAnswerWithARealDetector(t *testing.T) {
	q, _ := questions(grown(), nil)
	status, err := q.Status(context.Background(), "/repository", "main")
	if err != nil || status.Question != answer.Status || status.Version != "0.1.0" || len(status.Detectors) != 1 ||
		len(status.Marks) != 1 || status.Marks[0].ID != "shape.file-size:src/a.ts" || status.Sections != nil {
		t.Fatalf("Status = %+v, %v; want one size mark and no sections", status, err)
	}
	report, err := q.Report(context.Background(), "/repository", "main", nil)
	if err != nil || report.Question != answer.Report || len(report.Marks) != 1 || len(report.Sections) != 1 ||
		report.Sections[0].Statistics[0].Name != "product lines" {
		t.Fatalf("Report = %+v, %v; want the mark and one shape section", report, err)
	}
	if text := q.Text(status); !strings.HasPrefix(text, "plimsoll 0.1.0 status: base `") {
		t.Errorf("Text(status) = %q, want the header first", text)
	}
}

func TestQuestionsRefuseInOnePlainSentence(t *testing.T) {
	q, called := questions(grown(), nil)
	_, err := q.Status(context.Background(), "relative/path", "main")
	if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != "`repository` must be an absolute path." || *called {
		t.Errorf("Status(relative path) = %v, take called %v; want the absolute-path refusal before reading", err, *called)
	}
	cases := map[snapshot.RefusalKind]string{
		snapshot.NotARepository:  "`repository` does not name a folder inside a git working tree.",
		snapshot.NoCommit:        "The repository has no commit yet, so there is nothing to compare.",
		snapshot.UnknownBase:     "Unknown base `nope`: git finds no such commit in this repository.",
		snapshot.NoCommonHistory: "Base `nope` shares no history with the head commit.",
		snapshot.GitFailed:       "git could not read the repository: `nope`.",
	}
	for kind, want := range cases {
		q, _ := questions(snapshot.Source{}, &snapshot.Refusal{Kind: kind, Detail: "nope"})
		_, err := q.Report(context.Background(), "/repository", "nope", nil)
		if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != want {
			t.Errorf("Report(refusal %d) = %v, want %q", kind, err, want)
		}
	}
	boom := errors.New("boom")
	q, _ = questions(snapshot.Source{}, boom)
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
		q, _ = questions(source, nil)
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
	q, _ := questions(source, nil)
	report, err := q.Report(context.Background(), "/repository", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"shape.file-size:src/new.ts", "shape.file-size:src/gone.ts", "shape.file-size:src/old.ts"} {
		a, err := q.Explain(context.Background(), "/repository", "main", id)
		items := len(a.Marks) + len(a.Resolved) + len(a.Standing)
		if err != nil || a.Question != answer.Explain || items != 1 || a.Footprint != nil || a.Sections != nil ||
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
	q, _ := questions(snapshottest.Files(nil, files), nil)
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
	q, _ := questions(grown(), nil)
	_, err := q.Explain(context.Background(), "/repository", "main", "shape.file-size:nope.ts")
	want := "Unknown mark `shape.file-size:nope.ts`: nothing with this identifier exists against this base now. `report` lists the current identifiers."
	if refusal, ok := errors.AsType[*answer.Refusal](err); !ok || refusal.Sentence != want {
		t.Errorf("Explain(unknown mark) = %v, want %q", err, want)
	}
}

func TestReportTakesNamedFiles(t *testing.T) {
	q, called := questions(snapshottest.Files(nil, map[string]string{"src/a.ts": "import \"./b.ts\";\n", "src/b.ts": "export const b = 1;\n"}), nil)
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
