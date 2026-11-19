package shape

import (
	"fmt"
	"math"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// ruleDuplicate is the rule of a function the change added that is largely the code of one other function.
const ruleDuplicate = "function-duplicate"

// duplicateSource is where the rule's limit comes from.
const duplicateSource = "plimsoll's rule, read blind on agent pull requests"

// duplicateLeftOut is what the rule leaves out on both sides, said in every verify line.
const duplicateLeftOut = "not compared: test, example, vendored and generated code, folders with their own licence"

// duplicateMarks are the marks of the listed pairs: one per new function, its counterpart carried with it.
func duplicateMarks(r snapshot.FunctionRepeats) []answer.Mark {
	limit := r.Rules.Share * 100
	var marks []answer.Mark
	for _, p := range r.Listed {
		marks = append(marks, answer.Mark{
			ID:          answer.ID(name, ruleDuplicate, functionID(snapshot.FunctionChange{Path: p.Path, Name: p.Name, Occurrence: p.Occurrence})),
			Change:      answer.New,
			Subject:     answer.Subject{Kind: "function", Paths: []string{p.Path}, Names: []string{p.Name}},
			Confidence:  answer.Candidate,
			Measured:    answer.Measured{Value: math.Floor(p.Share * 100), Limit: &limit, LimitSource: duplicateSource, Unit: "% of its tokens"},
			Counterpart: &answer.Counterpart{Path: p.Counterpart.Path, Name: p.Counterpart.Name, Status: p.Counterpart.Status.String()},
			Hint:        duplicateHint(p, r.Rules.Stretch),
		})
	}
	return marks
}

// duplicateSentence names the pair from the mark alone: the counterpart that was there before, or one also new.
func duplicateSentence(m answer.Mark) string {
	named := fmt.Sprintf("`%s` `%s`", m.Subject.Paths[0], m.Subject.Names[0])
	other := fmt.Sprintf("`%s` `%s`", m.Counterpart.Path, m.Counterpart.Name)
	limit := fmt.Sprintf("Limit: %s%%, %s.", answer.Count(*m.Measured.Limit), m.Measured.LimitSource)
	share := answer.Count(m.Measured.Value) + "%"
	if m.Counterpart.Status == snapshot.CounterpartNew.String() {
		where := "also new"
		if m.Counterpart.Path == m.Subject.Paths[0] {
			where = "also new, in the same file"
		}
		return fmt.Sprintf("%s (new in this change) and %s (%s) are largely the same code: %s of the first is in the second. %s",
			named, other, where, share, limit)
	}
	there := "which was there before"
	if m.Counterpart.Status == snapshot.CounterpartChanged.String() {
		there = "which was there before and this change changed"
	}
	return fmt.Sprintf("%s is new in this change, and %s of its tokens are in %s, %s. %s", named, share, other, there, limit)
}

// duplicateHint is the verify line: both places, what differs, the covered stretch, and what was not compared.
func duplicateHint(p snapshot.FunctionRepeat, stretch int) string {
	places := fmt.Sprintf("compare lines %d–%d with lines %d–%d of `%s`", p.Start, p.End, p.Counterpart.Start, p.Counterpart.End, p.Counterpart.Path)
	run := fmt.Sprintf("%d tokens in a row of this function are covered, lines %d–%d (a covered stretch counts from %d tokens)",
		p.Stretch, p.StretchStart, p.StretchEnd, stretch)
	if p.GaveUp {
		return fmt.Sprintf("%s: the line comparison gave up, so the lines without a partner are not listed; %s; %s.", places, run, duplicateLeftOut)
	}
	return fmt.Sprintf("%s: %d of the %d lines that hold code have a partner there; %s; %s; %s.",
		places, p.Partnered, p.CodeLines, withoutPartner(p.Unpartnered, p.CounterpartUnpartnered), run, duplicateLeftOut)
}

// withoutPartner says which lines of each function have no partner: "without one: lines 44–46 and 104 here, 16–18
// there".
func withoutPartner(here, there []snapshot.LineRange) string {
	switch {
	case len(here) == 0 && len(there) == 0:
		return "every line has one"
	case len(there) == 0:
		return fmt.Sprintf("without one: lines %s here", ranges(here))
	case len(here) == 0:
		return fmt.Sprintf("without one: lines %s there", ranges(there))
	}
	return fmt.Sprintf("without one: lines %s here, %s there", ranges(here), ranges(there))
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

// duplicateRows are the report's rows of the rule: what was compared and what repeats, with what the rule leaves out.
func duplicateRows(r snapshot.FunctionRepeats) []answer.Statistic {
	if r.Compared == 0 {
		return nil
	}
	count := func(n int) *float64 { v := float64(n); return &v }
	compared := fmt.Sprintf("of %d tokens and %d lines or more, against %d functions in %d product files of their language, of any size; "+
		"not checked as new: functions the change rewrote or moved, smaller functions; "+
		"left out on both sides: test, demo, example, vendored and generated code, folders with their own licence",
		r.Rules.MinTokens, r.Rules.MinLines, r.Searched, r.SearchedFiles)
	return []answer.Statistic{
		{Name: "new functions compared for repeats", Value: count(r.Compared), Note: compared},
		{Name: "new functions that repeat another", Value: count(r.Found), Note: duplicateListed(len(r.Listed), r.Found, r.PerMeasure)},
	}
}

// duplicateListed is "2 listed", or "2 listed (at most three per measure, the fullest repeat first)" when more repeat.
func duplicateListed(listed, found, perMeasure int) string {
	if found <= listed {
		return fmt.Sprintf("%d listed", listed)
	}
	return fmt.Sprintf("%d listed (at most %s per measure, the fullest repeat first)", listed, words(perMeasure))
}

// duplicateSummary is the status sentence when the list was cut.
func duplicateSummary(r snapshot.FunctionRepeats) []string {
	if r.Found <= len(r.Listed) {
		return nil
	}
	return []string{fmt.Sprintf("Function repeats: %d of %d new functions that repeat another are listed (the fullest repeat first); `report` counts the rest.",
		len(r.Listed), r.Found)}
}

// duplicatesNotAnalysed are what the rule could not see.
func duplicatesNotAnalysed(r snapshot.FunctionRepeats) []string {
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
		found = append(found, answer.Agree(r.Unreadable, "function searched for repeats could not be read by the parser and was not compared",
			"functions searched for repeats could not be read by the parser and were not compared"))
	}
	return found
}
