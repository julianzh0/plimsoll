package answer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Questions answers status and report with the detectors, in their order.
type Questions struct {
	Take      func(ctx context.Context, repository, base string) (snapshot.Snapshot, error)
	Detectors []Detector
	Version   string
}

// Refusal is a question that cannot be answered, in one plain sentence that says what is wrong.
type Refusal struct {
	Sentence string
}

func (r *Refusal) Error() string { return r.Sentence }

// MissingArgument refuses a question without one of its arguments.
func MissingArgument(name string) *Refusal {
	return &Refusal{Sentence: fmt.Sprintf("The argument `%s` is missing.", name)}
}

// UnknownArgument refuses a question with an argument it does not take.
func UnknownArgument(name string) *Refusal {
	return &Refusal{Sentence: fmt.Sprintf("Unknown argument `%s`.", name)}
}

// NotTextList refuses a question whose argument is not a list of text.
func NotTextList(name string) *Refusal {
	return &Refusal{Sentence: fmt.Sprintf("The argument `%s` must be a list of text.", name)}
}

// NotText refuses a question whose argument is not text.
func NotText(name string) *Refusal {
	return &Refusal{Sentence: fmt.Sprintf("The argument `%s` must be text.", name)}
}

// Status answers with at most five new or grown marks since the base.
func (q Questions) Status(ctx context.Context, repository, base string) (Answer, error) {
	return q.answer(ctx, Status, repository, base)
}

// Report answers with everything measured against the base. files names tracked paths whose importers the answer
// also gives; it is checked before the repository is read, and nil or empty names none.
func (q Questions) Report(ctx context.Context, repository, base string, files []string) (Answer, error) {
	named, ok := snapshot.NamedPaths(files)
	if !ok {
		return Answer{}, &Refusal{Sentence: "Every entry of `files` must be a path relative to the repository's top folder."}
	}
	_, a, err := q.measure(ctx, Report, repository, base, named)
	return a, err
}

// Text renders an answer with the detectors' wording.
func (q Questions) Text(a Answer) string {
	return Render(a, WordingOf(q.Detectors))
}

// Explain answers with one item of the full answer and its evidence, computed again.
func (q Questions) Explain(ctx context.Context, repository, base, mark string) (Answer, error) {
	s, report, err := q.measure(ctx, Report, repository, base, nil)
	if err != nil {
		return Answer{}, err
	}
	explained := report
	explained.Question, explained.Footprint, explained.Sections = Explain, nil, nil
	explained.Marks, explained.Resolved, explained.Standing = only(report.Marks, mark), only(report.Resolved, mark), only(report.Standing, mark)
	items := append(append(slices.Clone(explained.Marks), explained.Resolved...), explained.Standing...)
	index := slices.IndexFunc(q.Detectors, func(d Detector) bool { return d.Name() == DetectorOf(mark) })
	if len(items) == 0 || index < 0 {
		return Answer{}, &Refusal{Sentence: fmt.Sprintf("Unknown mark `%s`: nothing with this identifier exists against this base now. `report` lists the current identifiers.", mark)}
	}
	evidence := NewEvidence(mark, q.Detectors[index].Explain(s, items[0]))
	explained.Evidence = &evidence
	return explained, nil
}

// only keeps the item with the identifier, as a non-nil list.
func only(marks []Mark, id string) []Mark {
	kept := []Mark{}
	for _, m := range marks {
		if m.ID == id {
			kept = append(kept, m)
		}
	}
	return kept
}

func (q Questions) answer(ctx context.Context, question Question, repository, base string) (Answer, error) {
	_, a, err := q.measure(ctx, question, repository, base, nil)
	return a, err
}

// measure takes the snapshot, runs every detector in order and assembles the answer.
func (q Questions) measure(ctx context.Context, question Question, repository, base string, named []string) (snapshot.Snapshot, Answer, error) {
	if !filepath.IsAbs(repository) {
		return snapshot.Snapshot{}, Answer{}, &Refusal{Sentence: "`repository` must be an absolute path."}
	}
	s, err := q.Take(ctx, repository, base)
	if err != nil {
		return snapshot.Snapshot{}, Answer{}, refused(err)
	}
	s.Named = named
	results := make([]Result, 0, len(q.Detectors))
	for _, detector := range q.Detectors {
		results = append(results, Result{Detector: detector.Name(), Findings: detector.Detect(s)})
	}
	return s, Assemble(question, q.Version, s, results), nil
}

// refused turns the reader's refusal into its sentence; any other error is returned as it is.
func refused(err error) error {
	refusal, ok := errors.AsType[*snapshot.Refusal](err)
	if !ok {
		return err
	}
	return &Refusal{Sentence: sentenceOf(refusal)}
}

func sentenceOf(refusal *snapshot.Refusal) string {
	switch refusal.Kind {
	case snapshot.NotARepository:
		return "`repository` does not name a folder inside a git working tree."
	case snapshot.NoCommit:
		return "The repository has no commit yet, so there is nothing to compare."
	case snapshot.UnknownBase:
		return fmt.Sprintf("Unknown base `%s`: git finds no such commit in this repository.", refusal.Detail)
	case snapshot.NoCommonHistory:
		return fmt.Sprintf("Base `%s` shares no history with the head commit.", refusal.Detail)
	case snapshot.GitFailed:
		return fmt.Sprintf("git could not read the repository: `%s`.", refusal.Detail)
	}
	return "The repository could not be read."
}
