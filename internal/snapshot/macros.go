package snapshot

import (
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// macroPasses is how often one rule is applied to a file at most: a rule finds a whole run of macros in one pass, so
// the passes after the first only take what a rewrite uncovered (the second comma of a foreach); the bound keeps a
// pathological file from costing a pass per macro.
const macroPasses = 4

// rewriteMacros applies the rules in order, each again until it finds nothing or macroPasses are spent: what a rule's
// group "macro" finds is blanked, or replaced by its As padded with spaces; every byte offset and line break stays
// where it was. nil when no rule found anything.
func rewriteMacros(rules []conventions.MacroRule, content []byte) []byte {
	rewritten, changed := slices.Clone(content), false
	for _, rule := range rules {
		group := rule.Pattern.SubexpIndex("macro")
		for range macroPasses {
			found := rule.Pattern.FindAllSubmatchIndex(rewritten, -1)
			if len(found) == 0 {
				break
			}
			changed = true
			for _, m := range found {
				start, end := m[2*group], m[2*group+1]
				copy(rewritten[start:end], rule.As)
				for i := start + len(rule.As); i < end; i++ {
					if rewritten[i] != '\n' {
						rewritten[i] = ' '
					}
				}
			}
		}
	}
	if !changed {
		return nil
	}
	return rewritten
}

// withBlanked takes, from the functions read on a copy with macros rewritten, those that overlap no function read
// cleanly as written; a function unreadable as written stays unless a function of the copy overlaps it, which then
// stands for it (or for the pieces the grammar split it into). So a function that read cleanly never changes.
func withBlanked(asWritten, blanked []function) []function {
	var clean, unreadable, taken []function
	for _, f := range asWritten {
		if f.broken {
			unreadable = append(unreadable, f)
		} else {
			clean = append(clean, f)
		}
	}
	for _, b := range blanked {
		if !overlapsAny(clean, b) {
			taken = append(taken, b)
		}
	}
	found := slices.Concat(clean, taken)
	for _, f := range unreadable {
		if !overlapsAny(taken, f) {
			found = append(found, f)
		}
	}
	slices.SortStableFunc(found, func(a, b function) int { return a.start - b.start })
	return found
}

// overlapsAny says whether f shares a byte with one of the functions.
func overlapsAny(functions []function, f function) bool {
	return slices.ContainsFunc(functions, func(g function) bool { return g.start < f.end && f.start < g.end })
}
