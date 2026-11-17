package snapshot

import (
	"regexp"
	"slices"
)

// blankMacros replaces every name the pattern's group "macro" finds with spaces, again until none is left, so a run of
// macros goes too; every byte offset and line stays where it was. false when nothing was found.
func blankMacros(pattern *regexp.Regexp, content []byte) ([]byte, bool) {
	group := pattern.SubexpIndex("macro")
	var blanked []byte
	for {
		text := content
		if blanked != nil {
			text = blanked
		}
		found := pattern.FindAllSubmatchIndex(text, -1)
		if len(found) == 0 {
			return blanked, blanked != nil
		}
		if blanked == nil {
			blanked = slices.Clone(content)
		}
		for _, m := range found {
			for i := m[2*group]; i < m[2*group+1]; i++ {
				blanked[i] = ' '
			}
		}
	}
}

// withBlanked takes, from the functions read on a copy with macros blanked, those that overlap no function read
// cleanly as written; a function unreadable as written stays when no function of the copy takes its place. So a
// function that read cleanly never changes and none is lost.
func withBlanked(asWritten, blanked []function) []function {
	overlaps := func(a, b function) bool { return a.start < b.end && b.start < a.end }
	var found []function
	for _, f := range asWritten {
		if !f.broken {
			found = append(found, f)
		}
	}
	clean := len(found)
	for _, b := range blanked {
		if !slices.ContainsFunc(found[:clean], func(f function) bool { return overlaps(f, b) }) {
			found = append(found, b)
		}
	}
	for _, f := range asWritten {
		if f.broken && !slices.ContainsFunc(found[clean:], func(b function) bool { return overlaps(f, b) }) {
			found = append(found, f)
		}
	}
	slices.SortStableFunc(found, func(a, b function) int { return a.start - b.start })
	return found
}
