package shape

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// testKinds are the rows of the declared kinds of test code that own test code on either side, in declared order,
// with a remainder that makes them add up to the test lines; and the sentences on what the table declares but
// does not use. A kind whose base lacks content (a partial clone) is counted now only.
func testKinds(s snapshot.Snapshot, testNow, testBefore float64) (rows []answer.Statistic, gaps []string) {
	baseKnown := true
	ownedNow, ownedBefore := 0.0, 0.0
	for _, kind := range s.TestKinds.Kinds {
		gaps = append(gaps, kindGaps(s, kind)...)
		if !kind.Owns {
			continue
		}
		rows = append(rows, kindRow("test lines of kind", kind.Name, float64(kind.Lines), float64(kind.BaseLines), !kind.BaseMissing))
		ownedNow += float64(kind.Lines)
		ownedBefore += float64(kind.BaseLines)
		baseKnown = baseKnown && !kind.BaseMissing
	}
	if len(rows) > 0 {
		rows = append(rows, kindRow("test lines of no declared kind", "", testNow-ownedNow, testBefore-ownedBefore, baseKnown))
	}
	return rows, gaps
}

// kindRow is a row of test lines; with its base unknown it gives the lines now in its note.
func kindRow(statName, subject string, now, before float64, baseKnown bool) answer.Statistic {
	if baseKnown {
		return stat(statName, subject, now, before, true, "lines")
	}
	return answer.Statistic{Name: statName, Subject: subject,
		Note: answer.Plural(int(now), "line") + " now; the base is not counted, as a partial clone lacks the content of files it matches"}
}

// kindGaps are the sentences on one kind: patterns not of the form, matching nothing or matching product code, and
// why a kind with usable patterns owns no test code.
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
		gaps = append(gaps, fmt.Sprintf("%s: `%s` matches %s; a kind labels test code only, so they are not counted",
			prefix, match.Pattern, answer.Plural(match.Files, "product code file")))
	}
	switch {
	case kind.Owns || len(kind.Patterns) == 0 || kind.BaseMissing || len(kind.Unmatched) == len(kind.Patterns):
	case len(kind.TakenBy) > 0:
		gaps = append(gaps, fmt.Sprintf("%s owns no test code: the files its patterns match belong to %s, listed earlier", prefix, answer.QuotedList(kind.TakenBy)))
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
