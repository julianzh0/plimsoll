package shape

import (
	"fmt"
	"math"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// ruleRepeat is the rule of a function the change added that is largely the code of one other function.
const ruleRepeat = "function-repeat"

// repeatSource is where the rule's limit comes from.
const repeatSource = "plimsoll's rule, read blind on agent pull requests"

// repeatLeftOut is what the rule leaves out on both sides, said in the report's rows and in explain.
const repeatLeftOut = "left out on both sides: test, demo, example, vendored and generated code, folders with their own licence"

// repeatMarks are the marks of the listed pairs: one per new function, its counterpart carried with it.
func repeatMarks(r snapshot.FunctionRepeats) []answer.Mark {
	limit := r.Rules.Share * 100
	var marks []answer.Mark
	for _, p := range r.Listed {
		marks = append(marks, answer.Mark{
			ID:          repeatID(p),
			Change:      answer.New,
			Subject:     answer.Subject{Kind: "function", Paths: []string{p.Path}, Names: []string{p.Name}},
			Confidence:  answer.Candidate,
			Measured:    answer.Measured{Value: math.Floor(p.Share * 100), Limit: &limit, LimitSource: repeatSource, Unit: "% of its tokens"},
			Counterpart: &answer.Counterpart{Path: p.Counterpart.Path, Name: p.Counterpart.Name, Status: p.Counterpart.Status.String()},
			Hint:        repeatHint(p, r.Rules.Stretch),
		})
	}
	return marks
}

// repeatID is a pair's mark identifier, named by its new function as the other function marks are.
func repeatID(p snapshot.FunctionRepeat) string {
	return answer.ID(name, ruleRepeat, functionID(snapshot.FunctionChange{Path: p.Path, Name: p.Name, Occurrence: p.Occurrence}))
}

// counterpartWhen says, by a counterpart's status, whether it was there before the change.
func counterpartWhen(status string) string {
	switch status {
	case snapshot.CounterpartChanged.String():
		return "which was there before; this change edited it"
	case snapshot.CounterpartNew.String():
		return "also new in this change"
	}
	return "which was there before"
}

// repeatSentence names the pair from the mark alone: the new function, its share, and its counterpart with whether it
// was there before. A counterpart of the same path and name is named as such; the verify line gives both places.
func repeatSentence(m answer.Mark) string {
	path, fn := m.Subject.Paths[0], m.Subject.Names[0]
	other := fmt.Sprintf("`%s` `%s`", m.Counterpart.Path, m.Counterpart.Name)
	if m.Counterpart.Path == path && m.Counterpart.Name == fn {
		other = "another function of its name in the same file"
	}
	return fmt.Sprintf("`%s` `%s` is new in this change, and %s%% of its tokens are in %s, %s. Limit: %s%%, %s.",
		path, fn, answer.Count(m.Measured.Value), other, counterpartWhen(m.Counterpart.Status), answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
}

// repeatHint is the verify line: both places, the lines without a match, and the longest covered run.
func repeatHint(p snapshot.FunctionRepeat, stretch int) string {
	there := fmt.Sprintf("`%s`", p.Counterpart.Path)
	if p.Counterpart.Path == p.Path {
		there = "the same file"
	}
	places := fmt.Sprintf("compare lines %d–%d of `%s` with lines %d–%d of %s", p.Start, p.End, p.Path, p.Counterpart.Start, p.Counterpart.End, there)
	run := fmt.Sprintf("the longest covered run is %d tokens, lines %d–%d (at least %d needed)", p.Stretch, p.StretchStart, p.StretchEnd, stretch)
	if p.GaveUp {
		return fmt.Sprintf("%s: the line comparison gave up, so the lines without a match are not listed; %s.", places, run)
	}
	return fmt.Sprintf("%s: %d of its %d lines that hold code have a matching line there%s; %s.",
		places, p.Partnered, p.CodeLines, withoutPartner(p.Unpartnered, p.CounterpartUnpartnered), run)
}

// withoutPartner says which lines of each function have no match: "; without one: lines 44–46 and 104 here, 16–18
// there", or nothing when every line has one.
func withoutPartner(here, there []snapshot.LineRange) string {
	switch {
	case len(here) == 0 && len(there) == 0:
		return ""
	case len(there) == 0:
		return fmt.Sprintf("; without one: lines %s here", ranges(here))
	case len(here) == 0:
		return fmt.Sprintf("; without one: lines %s there", ranges(there))
	}
	return fmt.Sprintf("; without one: lines %s here, %s there", ranges(here), ranges(there))
}

// ranges writes line ranges as "44–46 and 104".
func ranges(list []snapshot.LineRange) string {
	parts := make([]string, len(list))
	for i, r := range list {
		parts[i] = fmt.Sprint(r.Start)
		if r.End > r.Start {
			parts[i] = fmt.Sprintf("%d–%d", r.Start, r.End)
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// repeatRows are the report's rows of the rule: what was compared and what repeats, with what the rule leaves out.
func repeatRows(r snapshot.FunctionRepeats) []answer.Statistic {
	if r.Compared == 0 {
		return nil
	}
	compared := fmt.Sprintf("only functions of %d tokens and %d lines or more; against %d functions of any size in %d product files of their language; "+
		"not checked as new: a function of a name its file held at the base, one moved from a function the change removed, smaller functions; %s",
		r.Rules.MinTokens, r.Rules.MinLines, r.Searched, r.SearchedFiles, repeatLeftOut)
	return []answer.Statistic{
		{Name: "new functions compared for repeats", Value: new(float64(r.Compared)), Note: compared},
		{Name: "new functions that repeat another", Value: new(float64(r.Found)), Note: repeatListed(len(r.Listed), r.Found, r.PerMeasure)},
	}
}

// repeatListed is "2 listed", or "2 listed (at most three per measure, the highest share first)" when more repeat.
func repeatListed(listed, found, perMeasure int) string {
	if found <= listed {
		return fmt.Sprintf("%d listed", listed)
	}
	return fmt.Sprintf("%d listed (at most %s per measure, the highest share first)", listed, words(perMeasure))
}

// repeatSummary is the sentence when the list was cut: what report lists and what it only counts.
func repeatSummary(r snapshot.FunctionRepeats) []string {
	if r.Found <= len(r.Listed) {
		return nil
	}
	rest, verb := r.Found-len(r.Listed), "are"
	if rest == 1 {
		verb = "is"
	}
	return []string{fmt.Sprintf("Function repeats: `report` lists %d of the %d new functions that repeat another (the highest share first); the other %s %s counted, not listed.",
		len(r.Listed), r.Found, words(rest), verb)}
}

// repeatsNotAnalysed are what the rule could not see.
func repeatsNotAnalysed(r snapshot.FunctionRepeats) []string {
	var found []string
	if len(r.Unavailable) > 0 {
		found = append(found, fmt.Sprintf("new functions were not compared for repeats: the clone lacks the base content of %s that changed: %s",
			answer.Plural(len(r.Unavailable), "code file"), answer.QuotedList(r.Unavailable)))
	}
	if len(r.Unwalked) > 0 {
		found = append(found, fmt.Sprintf("the parser could not read the functions of %s searched for repeats: %s",
			answer.Plural(len(r.Unwalked), "file"), answer.QuotedList(r.Unwalked)))
	}
	if r.Unreadable > 0 {
		verb := "were"
		if r.Unreadable == 1 {
			verb = "was"
		}
		found = append(found, fmt.Sprintf("%d of the %d functions searched for a counterpart could not be read by the parser and %s not compared",
			r.Unreadable, r.Searched+r.Unreadable, verb))
	}
	return found
}

// explainedRanges is how many line ranges without a partner explain names on each side.
const explainedRanges = 10

// repeatEvidence is what explain says of a pair: both places, both shares, the covered stretch, the lines without
// a partner, and what was searched. Lines, counts, shares, paths and the two names only.
func repeatEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	r := s.Repeats
	for _, p := range r.Listed {
		if repeatID(p) != m.ID {
			continue
		}
		lines := []string{
			fmt.Sprintf("new in this change: lines %d–%d (%d tokens)", p.Start, p.End, p.Tokens),
			fmt.Sprintf("counterpart: `%s` `%s`, lines %d–%d (%d tokens), %s", p.Counterpart.Path, p.Counterpart.Name, p.Counterpart.Start,
				p.Counterpart.End, p.Counterpart.Tokens, counterpartWhen(p.Counterpart.Status.String())),
			fmt.Sprintf("%d%% of its tokens are in runs of %d tokens the counterpart holds (the limit is %d%%); %d%% of the counterpart's are in its",
				int(math.Floor(p.Share*100)), r.Rules.Run, int(r.Rules.Share*100), int(math.Floor(p.Reverse*100))),
			fmt.Sprintf("%d tokens in a row are covered, lines %d–%d (at least %d needed)", p.Stretch, p.StretchStart, p.StretchEnd, r.Rules.Stretch),
			"tokens: names, keywords and operators by their text, every literal the same, comments left out; a run counts as often as the counterpart holds it",
		}
		if p.GaveUp {
			lines = append(lines, "the line comparison gave up: the lines without a match are not listed")
		} else {
			lines = append(lines, fmt.Sprintf("%d of its %d lines that hold code have a matching line; without one here: %s; there: %s",
				p.Partnered, p.CodeLines, cappedRanges(p.Unpartnered), cappedRanges(p.CounterpartUnpartnered)))
		}
		return append(lines,
			fmt.Sprintf("searched: %d functions in %d product files of its language; %s also met the limits", r.Searched, r.SearchedFiles, answer.Plural(p.Others, "other function")),
			repeatLeftOut+"; not checked as new: a function of a name its file held at the base, one moved from a function the change removed")
	}
	return nil
}

// cappedRanges names at most explainedRanges ranges and counts the rest.
func cappedRanges(list []snapshot.LineRange) string {
	if len(list) == 0 {
		return "none"
	}
	if len(list) <= explainedRanges {
		return ranges(list)
	}
	return fmt.Sprintf("%s and %d more", ranges(list[:explainedRanges]), len(list)-explainedRanges)
}
