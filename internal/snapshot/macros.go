package snapshot

import (
	"regexp"
	"slices"
)

// blankMacros replaces every name the pattern's group "macro" finds with spaces, again until none is left, so a run of
// macros goes too; every byte offset and line stays where it was. nil when nothing was found.
func blankMacros(pattern *regexp.Regexp, content []byte) []byte {
	group := pattern.SubexpIndex("macro")
	blanked, changed := slices.Clone(content), false
	for found := pattern.FindAllSubmatchIndex(blanked, -1); len(found) > 0; found = pattern.FindAllSubmatchIndex(blanked, -1) {
		changed = true
		for _, m := range found {
			for i := m[2*group]; i < m[2*group+1]; i++ {
				blanked[i] = ' '
			}
		}
	}
	if !changed {
		return nil
	}
	return blanked
}

// withBlanked takes, from the functions read on a copy with macros blanked, those that overlap no function read
// cleanly as written; a function unreadable as written stays when no function of the copy takes its place. So a
// function that read cleanly never changes and none is lost.
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
