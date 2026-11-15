// Package process is the process detector: tracked planning and review records, and documentation churn above
// code churn since the base.
package process

import (
	"fmt"
	"strings"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

const name = "process"

// The rules of the detector.
const (
	rulePlanning    = "planning-file"
	ruleDocChurn    = "doc-churn"
	ruleLimitChange = "limit-change"
)

const (
	planningSource = "plimsoll's rule: repositories hold no planning records"
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
		Notes:      []string{"planning and review paths: " + s.PlanningRule},
		Marks:      append(planningMarks(s), limitMarks(s)...),
	}
	if note := limitsNote(s); note != "" {
		f.Notes = append(f.Notes, note)
	}
	for _, problem := range s.LimitProblems {
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s`: %s; a limit it does not set keeps plimsoll's default", s.LimitsFile, problem))
	}
	churn := snapshot.ChurnOf(s.History.SinceBase())
	if churn.Commits == 0 {
		return f
	}
	docs, code := float64(churn.Total(snapshot.AreaDocs)), float64(churn.Total(snapshot.AreaProduct)+churn.Total(snapshot.AreaTests))
	zero := 0.0
	f.Statistics = append(f.Statistics,
		answer.Statistic{Name: "documentation churn since the base", Value: &docs, Before: &zero, Unit: "lines"},
		answer.Statistic{Name: "product and test churn since the base", Value: &code, Before: &zero, Unit: "lines"},
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

// planningMarks compares the planning files of both sides. A renamed file takes its base side from its old name,
// and the old name gives no item.
func planningMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	atBase, now := set(s.Before.Planning), set(s.Now.Planning)
	seenAtBase := map[string]bool{}
	for _, path := range s.Now.Planning {
		basePath := s.BasePath(path)
		seenAtBase[basePath] = true
		marks = append(marks, planningMark(path, atBase[basePath], true)...)
	}
	for _, path := range s.Before.Planning {
		if !seenAtBase[path] && !now[path] {
			marks = append(marks, planningMark(path, true, false)...)
		}
	}
	return marks
}

func planningMark(path string, atBase, now bool) []answer.Mark {
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
		if now == before {
			continue
		}
		defaultValue := d.Default
		marks = append(marks, answer.Mark{
			ID:         answer.ID(name, ruleLimitChange, d.Key),
			Change:     answer.New,
			Subject:    answer.Subject{Kind: "file", Paths: []string{s.LimitsFile}, Names: []string{d.Name}},
			Confidence: answer.Exact,
			Measured:   answer.Measured{Value: now, Before: &before, Limit: &defaultValue, LimitSource: "plimsoll's default", Unit: d.Unit},
			Hint:       fmt.Sprintf("compare the [limits] table of `%s` at the base and now.", s.LimitsFile),
		})
	}
	return marks
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
	return fmt.Sprintf("limits set by `%s`: %s", s.LimitsFile, strings.Join(set, ", "))
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
