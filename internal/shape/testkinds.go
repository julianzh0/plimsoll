package shape

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// testKinds are the rows of the declared kinds of test code that own test code on either side, in declared order,
// with a remainder that makes them add up to the test lines; the note naming the kinds with the patterns in use; and
// the sentences on what the table declares but does not use. A row whose base lacks content (a partial clone) is
// counted now only; as the content it lacks is in neither the test lines nor the row, the other rows stay exact.
func testKinds(s snapshot.Snapshot, testNow, testBefore float64) (rows []answer.Statistic, notes, gaps []string) {
	ownedNow, ownedBefore := 0.0, 0.0
	var inUse []string
	for _, kind := range s.TestKinds.Kinds {
		gaps = append(gaps, kindGaps(s, kind)...)
		if len(kind.InUse) > 0 {
			inUse = append(inUse, fmt.Sprintf("`%s`: %s", kind.Name, answer.QuotedList(kind.InUse)))
		}
		if !kind.Owns() && !kind.BaseMissing {
			continue
		}
		rows = append(rows, kindRow("test lines of kind", kind.Name, float64(kind.Lines), float64(kind.BaseLines), !kind.BaseMissing))
		ownedNow += float64(kind.Lines)
		ownedBefore += float64(kind.BaseLines)
	}
	if len(rows) > 0 {
		rows = append(rows, kindRow("test lines of no declared kind", "", testNow-ownedNow, testBefore-ownedBefore, !s.TestKinds.RemainderBaseMissing))
	}
	if len(inUse) > 0 {
		notes = []string{fmt.Sprintf("test kinds declared in `%s` (`[%s]`), in order: %s", s.OwnFile, s.TestKinds.Table, strings.Join(inUse, "; "))}
	}
	return rows, notes, gaps
}

// kindRow is a row of test lines; with its base unknown it gives the lines now in its note.
func kindRow(statName, subject string, now, before float64, baseKnown bool) answer.Statistic {
	if baseKnown {
		return stat(statName, subject, now, before, true, "lines")
	}
	return answer.Statistic{Name: statName, Subject: subject,
		Note: answer.Plural(int(now), "line") + " now; the base is not counted, as a partial clone lacks the content of files it matches"}
}

// kindGaps are the sentences on one kind: patterns not of the form, matching nothing, matching product code or no
// test code the kind owns, and why a kind with usable patterns owns no test code.
func kindGaps(s snapshot.Snapshot, kind snapshot.TestKind) []string {
	prefix := fmt.Sprintf("`%s` `[%s]` `%s`", s.OwnFile, s.TestKinds.Table, kind.Name)
	var gaps []string
	if kind.Unusable > 0 {
		gaps = append(gaps, prefix+": "+answer.Agree(kind.Unusable, "value is no path pattern; it is", "values are no path patterns; they are")+" not used")
	}
	for _, pattern := range kind.Unmatched {
		if strings.ContainsAny(pattern, "*?") || !holdsFiles(s, pattern) {
			gaps = append(gaps, fmt.Sprintf("%s: `%s` matches no tracked file; it is not used", prefix, pattern))
		} else {
			gaps = append(gaps, fmt.Sprintf("%s: `%s` matches no tracked file; a folder's files are written `%s/**`", prefix, pattern, pattern))
		}
	}
	for _, match := range kind.ProductMatches {
		counted := "they are not counted"
		if match.Files == 1 {
			counted = "it is not counted"
		}
		gaps = append(gaps, fmt.Sprintf("%s: `%s` matches %s; a kind labels test code only, so %s", prefix, match.Pattern,
			answer.Plural(match.Files, "product code file"), counted))
	}
	switch {
	case kind.Owns():
		for _, pattern := range kind.Patterns {
			if !slices.Contains(kind.InUse, pattern) && !slices.Contains(kind.Unmatched, pattern) &&
				!slices.ContainsFunc(kind.ProductMatches, func(m snapshot.PatternFiles) bool { return m.Pattern == pattern }) {
				gaps = append(gaps, fmt.Sprintf("%s: `%s` matches no test code this kind owns; it is not used", prefix, pattern))
			}
		}
	case len(kind.Patterns) == 0 || kind.BaseMissing || len(kind.Unmatched) == len(kind.Patterns):
	case len(kind.TakenBy) > 0:
		which := "which comes"
		if len(kind.TakenBy) > 1 {
			which = "which come"
		}
		gaps = append(gaps, fmt.Sprintf("%s owns no test code: test files it matches belong to %s, %s before it in `[%s]`", prefix,
			answer.QuotedList(kind.TakenBy), which, s.TestKinds.Table))
	default:
		gaps = append(gaps, prefix+" owns no test code: its patterns match no test code")
	}
	return gaps
}

// holdsFiles says whether a path names a folder that holds tracked files on either side.
func holdsFiles(s snapshot.Snapshot, folder string) bool {
	under := func(p string) bool { return strings.HasPrefix(p, folder+"/") }
	return slices.ContainsFunc(s.Now.Paths, under) || slices.ContainsFunc(s.Before.Paths, under)
}
