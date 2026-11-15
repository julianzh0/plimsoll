// Package cochange is the changed-together detector: pairs of code files that the history changes in the same
// commits, at least one of them product code.
package cochange

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

const (
	name     = "co-change"
	rulePair = "pair"
	shown    = 3
)

// Detector is the changed-together detector.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

// pair is two eligible files, sorted, and the usable commits that list them.
type pair struct {
	a, b   string
	shared int // usable commits that list both
	atBase int // of those, the ones that are not since the base
	either int // usable commits that list either
}

func (p pair) id() string      { return answer.ID(name, rulePair, p.a+"+"+p.b) }
func (p pair) subject() string { return p.a + " + " + p.b }

// Detect counts the shared usable commits of every eligible pair and marks the pairs at or above the limit.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	if s.History.Usable == 0 {
		return answer.Findings{NotAnalysed: []string{"no usable commit in the history, so no pairs"}}
	}
	limit := s.Limits.SharedCommits
	var marks []answer.Mark
	var atLimit []pair
	countBase := 0.0
	for _, p := range pairs(s) {
		holdsNow, holdsBase := float64(p.shared) >= limit.Value, float64(p.atBase) >= limit.Value
		if holdsNow {
			atLimit = append(atLimit, p)
		}
		if holdsBase {
			countBase++
		}
		change, found := answer.Compare(answer.Reading{Holds: holdsBase, Value: float64(p.atBase)}, answer.Reading{Holds: holdsNow, Value: float64(p.shared)}, limit.Step)
		if !found {
			continue
		}
		value, before, total, limitValue := float64(p.shared), float64(p.atBase), float64(p.either), limit.Value
		marks = append(marks, answer.Mark{
			ID:         p.id(),
			Change:     change,
			Subject:    answer.Subject{Kind: "files", Paths: []string{p.a, p.b}},
			Confidence: answer.Exact,
			Measured: answer.Measured{
				Value: value, Before: &before, Limit: &limitValue, LimitSource: limit.Source, Total: &total, Unit: "shared commits",
			},
			Hint: "`explain` lists the shared commits by name.",
		})
	}
	countNow := float64(len(atLimit))
	slices.SortStableFunc(atLimit, func(x, y pair) int { return cmp.Compare(y.shared, x.shared) })
	f := answer.Findings{
		Marks: marks,
		Statistics: []answer.Statistic{{
			Name: "pairs with at least " + answer.Count(limit.Value) + " shared commits", Value: &countNow, Before: &countBase, Unit: "pairs",
		}},
		Notes: []string{"pairs: two code files tracked now, at least one product code, neither a file that defines tests, not both in one folder where a language imports folders; planning, review and measurement paths left out"},
	}
	for _, p := range atLimit[:min(shown, len(atLimit))] {
		value, before := float64(p.shared), float64(p.atBase)
		f.Statistics = append(f.Statistics, answer.Statistic{
			Name: "pair", Subject: p.subject(), Value: &value, Before: &before, Unit: "shared commits",
			Note: "of the " + answer.Count(float64(p.either)) + " that touched either",
		})
	}
	return f
}

// together is how often a pair changed together: "6 of the 7 commits that touched either".
func together(shared, either int) string {
	return answer.Count(float64(shared)) + " of the " + answer.Count(float64(either)) + " commits that touched either"
}

// eligible are the code files tracked now that may form a pair: no record, no file that defines tests.
func eligible(s snapshot.Snapshot) map[string]snapshot.File {
	found := map[string]snapshot.File{}
	for _, file := range s.Now.Files {
		if !file.Record && file.Role != snapshot.Test {
			found[file.Path] = file
		}
	}
	return found
}

// pairs counts, in one pass over the usable commits, every eligible pair with at least one product file, sorted
// by identifier. A usable commit lists at most the history's path limit, so the pairs per commit stay bounded.
func pairs(s snapshot.Snapshot) []pair {
	files := eligible(s)
	byFolder := map[string]bool{} // languages whose imports name folders: a folder is one unit there
	for _, l := range s.Now.Links.Languages {
		byFolder[l.Language] = l.Folders
	}
	counted := map[[2]string]*pair{}
	for _, commit := range s.History.Commits {
		if !commit.Usable {
			continue
		}
		var paths []string
		for _, change := range commit.Files {
			if _, ok := files[change.Path]; ok {
				paths = append(paths, change.Path)
			}
		}
		slices.Sort(paths)
		paths = slices.Compact(paths)
		for i, x := range paths {
			for _, y := range paths[i+1:] {
				if files[x].Role != snapshot.Product && files[y].Role != snapshot.Product {
					continue
				}
				if byFolder[files[x].Language] && byFolder[files[y].Language] && path.Dir(x) == path.Dir(y) {
					continue // files of one package change together by nature
				}
				p := counted[[2]string{x, y}]
				if p == nil {
					p = &pair{a: x, b: y}
					counted[[2]string{x, y}] = p
				}
				p.shared++
				if !commit.Since {
					p.atBase++
				}
			}
		}
	}
	touches := s.History.Touches()
	found := make([]pair, 0, len(counted))
	for _, p := range counted {
		p.either = touches[p.a].All + touches[p.b].All - p.shared
		found = append(found, *p)
	}
	slices.SortFunc(found, func(x, y pair) int { return cmp.Or(strings.Compare(x.a, y.a), strings.Compare(x.b, y.b)) })
	return found
}
