// Package process is the process detector: tracked planning and review records, and documentation churn above
// code churn since the base.
package process

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const name = "process"

// The rules of the detector.
const (
	rulePlanning    = "planning-file"
	ruleDocChurn    = "doc-churn"
	ruleLimitChange = "limit-change"
	// ruleDeclarationChange is a text key of the repository's own file declared differently now than at the base.
	ruleDeclarationChange = "declaration-change"
)

const (
	planningSource = "plimsoll's rule: repositories hold no planning records"
	// noteSource is the rule of a mark that a working-note name makes.
	noteSource     = "plimsoll's rule: notes about one change do not belong in the merged repository"
	docChurnSource = "documentation above code"
)

// Detector is the process detector.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

// Detect lists the tracked planning and review files of both sides and compares documentation churn with code
// churn over the commits since the base.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	now, before := float64(len(s.Now.Planning)), float64(len(s.Before.Planning))
	f := answer.Findings{
		Statistics: []answer.Statistic{{Name: "tracked planning and review files", Value: &now, Before: &before, Unit: "files"}},
		Notes:      planningNotes(s),
		Marks:      slices.Concat(planningMarks(s), limitMarks(s), declarationMarks(s)),
	}
	if named := namedNotes(s); len(named) > 0 {
		f.Notes = append(f.Notes, "working-note words in the name of an added Markdown file: "+s.NoteRule,
			"added files matched by name (candidates): "+named)
	}
	f.Ran(answer.Kind(name, rulePlanning))
	if (len(s.DeclaredLimits) > 0 || len(s.Declarations) > 0) && !s.OwnFileUnreadAtBase { // its own file, read on both sides
		f.Ran(answer.Kind(name, ruleLimitChange))
		f.Ran(answer.Kind(name, ruleDeclarationChange))
	}
	if note := limitsNote(s); note != "" {
		f.Notes = append(f.Notes, note)
	}
	f.Notes = append(f.Notes, answer.OwnNotes(s.Layout.OwnBase)...)
	for _, problem := range s.OwnFileProblems {
		if s.OwnFileUnread { // nothing of the file applies: said first, so that status shows it
			f.NotAnalysedFirst = append(f.NotAnalysedFirst, fmt.Sprintf("`%s`: %s; plimsoll's defaults apply", s.OwnFile, problem))
			continue
		}
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s`: %s; what it does not set keeps plimsoll's default", s.OwnFile, problem))
	}
	f.NotAnalysed = append(f.NotAnalysed, answer.OwnNotUsed(s.Layout.Planning)...)
	f.NotAnalysed = append(f.NotAnalysed, notCompared(s)...)
	churn := snapshot.ChurnOf(s.History.SinceBase())
	if churn.Commits == 0 {
		return f
	}
	docs, code := float64(churn.Total(snapshot.AreaDocs)), float64(churn.Total(snapshot.AreaProduct)+churn.Total(snapshot.AreaTests))
	zero := 0.0
	f.Statistics = append(f.Statistics,
		answer.Statistic{Name: "documentation churn since the base, summed over its commits", Value: &docs, Before: &zero, Unit: "lines"},
		answer.Statistic{Name: "product and test churn since the base, summed over its commits", Value: &code, Before: &zero, Unit: "lines"},
	)
	switch {
	case code == 0 && docs > 0:
		f.NotAnalysed = append(f.NotAnalysed, "no product or test churn since the base to compare documentation churn with")
	case docs > code && code > 0:
		f.Marks = append(f.Marks, answer.Mark{
			ID:         answer.ID(name, ruleDocChurn, "repository"),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "repository"},
			Confidence: answer.Exact,
			Measured:   answer.Measured{Value: docs, Limit: &code, LimitSource: docChurnSource, Unit: "lines of documentation churn"},
			Hint:       "sum the added and removed lines per area over the commits since the base.",
		})
	}
	return f
}

// namedNotesShown is how many files the note on name matches names; report lists every mark.
const namedNotesShown = 5

// namedNotes names the files the change added that match by name, at most namedNotesShown; "" when there are none.
func namedNotes(s snapshot.Snapshot) string {
	var named []string
	for _, p := range s.Now.Planning {
		if s.PlanningMatches[p].Word != "" {
			named = append(named, "`"+p+"`")
		}
	}
	if len(named) > namedNotesShown {
		return strings.Join(named[:namedNotesShown], ", ") + fmt.Sprintf(" and %d more", len(named)-namedNotesShown)
	}
	return strings.Join(named, ", ")
}

// planningNotes name the planning and review paths: the defaults, then those the repository declares.
func planningNotes(s snapshot.Snapshot) []string {
	return append([]string{"default planning and review paths: " + s.PlanningRule}, answer.OwnNotes(s.Layout.Planning)...)
}

// planningMarks compares the planning files of both sides. A renamed file takes its base side from its old name,
// and the old name gives no item.
func planningMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	atBase, now := set(s.Before.Planning), set(s.Now.Planning)
	seenAtBase := map[string]bool{}
	for _, path := range s.Now.Planning {
		basePath := s.BasePath(path)
		seenAtBase[basePath] = true
		marks = append(marks, planningMark(path, atBase[basePath], true, s.PlanningMatches[path].Word)...)
	}
	for _, path := range s.Before.Planning {
		if !seenAtBase[path] && !now[path] {
			marks = append(marks, planningMark(path, true, false, "")...)
		}
	}
	return marks
}

// planningMark is the mark of one planning file; word is the working-note word of an added file's name, for which the
// mark is a candidate (a name is an inference, a path a measurement).
func planningMark(path string, atBase, now bool, word string) []answer.Mark {
	reading := func(tracked bool) answer.Reading {
		if tracked {
			return answer.Reading{Holds: true, Value: 1}
		}
		return answer.Reading{}
	}
	change, found := answer.Compare(reading(atBase), reading(now), 1)
	if !found {
		return nil
	}
	m := answer.Mark{
		ID:         answer.ID(name, rulePlanning, path),
		Change:     change,
		Subject:    answer.Subject{Kind: "file", Paths: []string{path}},
		Confidence: answer.Exact,
		Measured:   answer.Measured{Value: reading(now).Value, LimitSource: planningSource, Unit: "tracked files"},
		Hint:       "list the tracked files under the planning and review paths at the base and now.",
	}
	if word != "" {
		m.Subject.Names, m.Confidence, m.Measured.LimitSource = []string{word}, answer.Candidate, noteSource
		m.Hint = "read the file and say whether it is a note about this change or a document the project keeps."
	}
	if atBase {
		one := 1.0
		m.Measured.Before = &one
	}
	return []answer.Mark{m}
}

// limitMarks are the limits the repository's limits file sets differently now than at the base. A side that does not
// set a limit has plimsoll's default.
func limitMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	for _, d := range s.DeclaredLimits {
		now, before := valueOr(d.Now, d.Default), valueOr(d.Before, d.Default)
		if !d.Compared || now == before {
			continue
		}
		defaultValue := d.Default
		marks = append(marks, answer.Mark{
			ID:         answer.ID(name, ruleLimitChange, d.Key),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "file", Paths: []string{s.OwnFile}, Names: []string{d.Name}},
			Confidence: answer.Exact,
			Measured:   answer.Measured{Value: now, Before: &before, Limit: &defaultValue, LimitSource: "plimsoll's default", Unit: d.Unit},
			Hint:       fmt.Sprintf("compare the [limits] table of `%s` at the base and now.", s.OwnFile),
		})
	}
	return marks
}

// declarationMarks are the text keys of the repository's own file whose values differ, as sets, now and at the base.
// A key not read on either side is not compared.
func declarationMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	for _, d := range s.Declarations {
		added, removed := answer.Added(d.Before, d.Now), answer.Added(d.Now, d.Before)
		if !d.Compared || len(added)+len(removed) == 0 {
			continue
		}
		named := slices.DeleteFunc(slices.Clone(added), func(v string) bool { return slices.Contains(d.Hidden, v) })
		slices.Sort(named)
		before := float64(len(d.Before))
		marks = append(marks, answer.Mark{
			ID:         answer.ID(name, ruleDeclarationChange, d.Key),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "file", Paths: []string{s.OwnFile}, Names: named, Unnamed: len(added) - len(named)},
			Confidence: answer.Exact,
			Measured:   answer.Measured{Value: float64(len(d.Now)), Before: &before, LimitSource: "any change", Unit: d.Name},
			Hint:       declarationHint(d, s.OwnFile),
		})
	}
	return marks
}

// declaredNamed is how many added values a declaration's sentence names before it counts the rest: a table of
// acknowledged items can hold hundreds.
const declaredNamed = 5

func declarationHint(d snapshot.DeclaredValues, ownFile string) string {
	if d.Whole {
		return fmt.Sprintf("compare the `[%s]` table of `%s` at the base and now.", d.Table, ownFile)
	}
	return fmt.Sprintf("compare `[%s]` `%s` of `%s` at the base and now.", d.Table, d.Key, ownFile)
}

// notCompared are the "not analysed" entries for a declaration the base side could not give.
func notCompared(s snapshot.Snapshot) []string {
	var entries []string
	if s.OwnFileUnreadAtBase && (len(s.Declarations) > 0 || len(s.DeclaredLimits) > 0) {
		entries = append(entries, fmt.Sprintf("`%s` could not be read at the base, so a change to its declarations is not marked", s.OwnFile))
	}
	for _, d := range s.Declarations {
		if d.UnreadAtBase {
			entries = append(entries, fmt.Sprintf("`%s`: `%s` could not be read at the base, so a change to it is not marked", s.OwnFile, d.Key))
		}
	}
	return entries
}

// limitsNote names the limits the repository's limits file sets now, or is "" when it sets none.
func limitsNote(s snapshot.Snapshot) string {
	var set []string
	for _, d := range s.DeclaredLimits {
		if d.Now != nil {
			set = append(set, fmt.Sprintf("the %s %s (plimsoll's default: %s)", d.Name, withUnit(*d.Now, d.Unit), bare(d.Default, d.Unit)))
		}
	}
	if len(set) == 0 {
		return ""
	}
	return fmt.Sprintf("limits set by `%s`: %s", s.OwnFile, strings.Join(set, ", "))
}

func valueOr(value *float64, otherwise float64) float64 {
	if value == nil {
		return otherwise
	}
	return *value
}

func set(paths []string) map[string]bool {
	found := make(map[string]bool, len(paths))
	for _, path := range paths {
		found[path] = true
	}
	return found
}
