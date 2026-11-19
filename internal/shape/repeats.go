package shape

import (
	"fmt"
	"math"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// ruleRepeat is the rule of a function the change added that is largely the code of one other function.
const ruleRepeat = "function-repeat"

// repeatMarks are the marks of the listed pairs: one per new function, its counterpart carried with it.
func repeatMarks(r snapshot.FunctionRepeats) []answer.Mark {
	limit := r.Rules.Share * 100
	var marks []answer.Mark
	for _, p := range r.Listed {
		there := fmt.Sprintf("`%s`", p.Counterpart.Path)
		if p.Counterpart.Path == p.Path {
			there = "the same file"
		}
		marks = append(marks, answer.Mark{
			ID:         repeatID(p),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "function", Paths: []string{p.Path}, Names: []string{p.Name}},
			Confidence: answer.Candidate,
			Measured: answer.Measured{Value: math.Floor(p.Share * 100), Limit: &limit,
				LimitSource: "plimsoll's rule, read blind on agent pull requests", Unit: "% of its tokens"},
			Counterpart: &answer.Counterpart{Path: p.Counterpart.Path, Name: p.Counterpart.Name, Status: p.Counterpart.Status.String()},
			Hint: fmt.Sprintf("compare lines %d–%d of `%s` with lines %d–%d of %s; the longest covered run is %d tokens, lines %d–%d (at least %d needed).",
				p.Start, p.End, p.Path, p.Counterpart.Start, p.Counterpart.End, there, p.Stretch, p.StretchStart, p.StretchEnd, r.Rules.Stretch),
		})
	}
	return marks
}

// repeatID is a pair's mark identifier, named by its new function as the other function marks are.
func repeatID(p snapshot.FunctionRepeat) string {
	return answer.ID(name, ruleRepeat, functionID(snapshot.FunctionChange{Path: p.Path, Name: p.Name, Occurrence: p.Occurrence}))
}

// repeatSentence names the pair from the mark alone: the new function, its share, and its counterpart with whether it
// was there before. A counterpart of the same path and name is named as such; the verify line gives both places.
func repeatSentence(m answer.Mark) string {
	path, fn := m.Subject.Paths[0], m.Subject.Names[0]
	other := fmt.Sprintf("`%s` `%s`", m.Counterpart.Path, m.Counterpart.Name)
	if m.Counterpart.Path == path && m.Counterpart.Name == fn {
		other = "another function of its name in the same file"
	}
	when := "which was there before"
	switch m.Counterpart.Status {
	case snapshot.CounterpartChanged.String():
		when = "which was there before; this change edited it"
	case snapshot.CounterpartNew.String():
		when = "also new in this change"
	}
	return fmt.Sprintf("`%s` `%s` is new in this change, and %s%% of its tokens are in %s, %s. Limit: %s%%, %s.",
		path, fn, answer.Count(m.Measured.Value), other, when, answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
}

// repeatRows are the report's rows of the rule: what was compared, what was left out and what repeats.
func repeatRows(r snapshot.FunctionRepeats) []answer.Statistic {
	if r.Files == 0 {
		return nil
	}
	compared := fmt.Sprintf("only functions of %d tokens and %d lines or more; against %d functions of any size in %d product files of their language; "+
		"not checked as new: a function of a name its file held at the base, one moved from a function the change removed (%d), "+
		"one in a file from another project (%d), smaller functions; left out on both sides: test, demo, example, vendored and "+
		"generated code, folders with their own licence", r.Rules.MinTokens, r.Rules.MinLines, r.Searched, r.SearchedFiles, r.Moved, r.FromAnotherProject)
	listed := fmt.Sprintf("%d listed", len(r.Listed))
	if r.Found > len(r.Listed) {
		listed += fmt.Sprintf(" (at most %s per measure, the most alike first: by the lower of the two shares)", words(r.PerMeasure))
	}
	return []answer.Statistic{
		{Name: "new functions compared for repeats", Value: new(float64(r.Compared)), Note: compared},
		{Name: "new functions that repeat another", Value: new(float64(r.Found)), Note: listed},
	}
}

// repeatsNotAnalysed are what the rule could not see.
func repeatsNotAnalysed(r snapshot.FunctionRepeats) []string {
	var found []string
	if len(r.Unwalked) > 0 {
		found = append(found, fmt.Sprintf("the parser could not read the functions of %s searched for repeats: %s",
			answer.Plural(len(r.Unwalked), "file"), answer.QuotedList(r.Unwalked)))
	}
	if r.Unreadable > 0 {
		found = append(found, fmt.Sprintf("%d of the %d functions searched for a counterpart could not be read by the parser and were not compared",
			r.Unreadable, r.Searched+r.Unreadable))
	}
	return found
}

// repeatEvidence is what explain says of a pair beside its verify line: both sizes, both shares and what was searched.
func repeatEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	for _, p := range s.Repeats.Listed {
		if repeatID(p) == m.ID {
			return []string{fmt.Sprintf("%d tokens against %d; %d%% of its tokens are in runs of %d tokens the counterpart holds, %d%% of the counterpart's in its; "+
				"searched: %d functions in %d product files", p.Tokens, p.Counterpart.Tokens, int(math.Floor(p.Share*100)),
				s.Repeats.Rules.Run, int(math.Floor(p.Reverse*100)), s.Repeats.Searched, s.Repeats.SearchedFiles)}
		}
	}
	return nil
}
