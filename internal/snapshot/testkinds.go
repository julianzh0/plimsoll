package snapshot

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// TestKinds is what now's tests table declares, and what each kind holds on both sides.
type TestKinds struct {
	Table string     // the table's name in the own file
	Kinds []TestKind // in file order
}

// TestKind is one kind of test code the repository declares, with what it finds. A test-code file belongs to the
// first kind, in file order, with a pattern that matches its path; a kind never changes what a file is.
type TestKind struct {
	Name     string
	Patterns []string // its usable patterns, in file order, each once
	Unusable int      // its patterns that have not the form of a path pattern
	// Lines and BaseLines are the lines of the test code it owns now and at the base.
	Lines, BaseLines int
	// BaseMissing says a path its patterns match lacks its content (a partial clone), so its base lines are unknown.
	BaseMissing bool
	// Unmatched are its patterns that match no tracked path on either side.
	Unmatched []string
	// ProductMatches are its patterns that match code files that are product code now, each with how many.
	ProductMatches []PatternFiles
	// TakenBy is, when it owns no test code, an earlier kind that owns a test-code file its patterns match.
	TakenBy string
}

// Owns says whether the kind owns test code on either side.
func (k TestKind) Owns() bool { return k.Lines > 0 || k.BaseLines > 0 }

// PatternFiles is a pattern and a number of files it matches.
type PatternFiles struct {
	Pattern string
	Files   int
}

// testsTable is what one side's tests table says: its kinds in file order, each with its usable patterns and the
// count of its unusable ones, and its problem phrases.
type testsTable struct {
	kinds            []conventions.TestKind
	unusable         []int // per kind
	seen             map[string]bool
	noEntry, unnamed int
	named            []string
}

// read reads the entry that starts at lines[i], following a list over lines, and gives the index of its last line.
// The name is read before the line is split at "=": a quoted name as written, a bare one up to a space or "=", so
// a quote or an "=" inside a name leaves it outside its form instead of shortening it.
func (t *testsTable) read(table string, lines []string, i int) int {
	line := strings.TrimSpace(uncommented(lines[i]))
	name, rest, ok := quotedString(line)
	if !ok {
		end := strings.IndexAny(line, " \t=")
		if end <= 0 {
			t.noEntry++
			return i
		}
		name, rest = line[:end], line[end:]
	}
	value, assigns := strings.CutPrefix(strings.TrimLeft(rest, " \t"), "=")
	if !assigns {
		t.noEntry++
		return i
	}
	value = strings.TrimSpace(value)
	for strings.HasPrefix(value, "[") && unclosed(value) && i+1 < len(lines) {
		i++
		value += lines[i]
	}
	switch {
	case !conventions.KindName(name):
		t.unnamed++
		return i
	case t.seen[name]:
		t.named = append(t.named, fmt.Sprintf("`[%s]` `%s` is set twice; its first entry decides", table, name))
		return i
	}
	if t.seen == nil {
		t.seen = map[string]bool{}
	}
	t.seen[name] = true
	items, isList := textList(value)
	if !isList {
		t.named = append(t.named, fmt.Sprintf("`[%s]` `%s` is not a list of text", table, name))
		return i
	}
	kind, unusable := conventions.TestKind{Name: name}, 0
	for _, item := range items {
		pattern, usable := conventions.NewPathPattern(item)
		switch {
		case !usable:
			unusable++
		case !slices.ContainsFunc(kind.Patterns, func(p conventions.PathPattern) bool { return p.Text() == item }):
			kind.Patterns = append(kind.Patterns, pattern)
		}
	}
	if len(kind.Patterns) > 0 || unusable > 0 {
		t.kinds, t.unusable = append(t.kinds, kind), append(t.unusable, unusable)
	}
	return i
}

// problems are the table's phrases: counted lines and names, then the named ones.
func (t testsTable) problems(table string) []string {
	var phrases []string
	if t.noEntry > 0 {
		phrases = append(phrases, fmt.Sprintf("`[%s]`: %s not applied", table,
			counted(t.noEntry, "line is no `name = [\"pattern\", …]` entry; it is", "lines are no `name = [\"pattern\", …]` entries; they are")))
	}
	if t.unnamed > 0 {
		phrases = append(phrases, fmt.Sprintf("`[%s]`: %s not applied", table,
			counted(t.unnamed, "kind name is no plain name; it is", "kind names are no plain names; they are")))
	}
	return append(phrases, t.named...)
}

// pairs are the usable patterns of the kinds as "kind: pattern" texts: kinds in file order, each kind's patterns
// sorted, so a reordered list of patterns is no change and a reordered list of kinds is one.
func (t testsTable) pairs() []string {
	var pairs []string
	for _, kind := range t.kinds {
		for _, pattern := range slices.Sorted(slices.Values(texts(kind.Patterns))) {
			pairs = append(pairs, kind.Name+": "+pattern)
		}
	}
	return pairs
}

// names are the names of the kinds with a usable pattern, in file order.
func (t testsTable) names() []string {
	var names []string
	for _, kind := range t.kinds {
		if len(kind.Patterns) > 0 {
			names = append(names, kind.Name)
		}
	}
	return names
}

// orderChanged says whether the kinds named on both sides stand in another order now than at the base.
func orderChanged(now, before []string) bool {
	inBoth := func(names, other []string) []string {
		return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return !slices.Contains(other, n) })
	}
	return !slices.Equal(inBoth(now, before), inBoth(before, now))
}

func texts(patterns []conventions.PathPattern) []string {
	written := make([]string, len(patterns))
	for i, p := range patterns {
		written[i] = p.Text()
	}
	return written
}

// testKinds measures each declared kind on both sides: the lines it owns, its patterns that match nothing or match
// product code now, the earlier kinds that took its files, and whether a path it matches lacks its content. Each
// path is split once and tried against the patterns that have not matched yet: on swc's 83,000 paths with twenty
// patterns, trying every pattern on every path took 1.7 s.
func testKinds(rules conventions.Rules, table testsTable, now, before Tree, missing []string) TestKinds {
	declared := TestKinds{Table: rules.TestsTable}
	if len(table.kinds) == 0 {
		return declared
	}
	type patternAt struct{ kind, pattern int }
	var patterns []patternAt
	for k, kind := range table.kinds {
		declared.Kinds = append(declared.Kinds, TestKind{Name: kind.Name, Patterns: texts(kind.Patterns), Unusable: table.unusable[k]})
		for p := range kind.Patterns {
			patterns = append(patterns, patternAt{k, p})
		}
	}
	match := func(at patternAt, parts []string) bool {
		return table.kinds[at.kind].Patterns[at.pattern].MatchSegments(parts)
	}
	matched := make([]bool, len(patterns))
	for _, p := range slices.Compact(slices.Sorted(slices.Values(slices.Concat(now.Paths, before.Paths)))) {
		parts := strings.Split(p, "/")
		for i, at := range patterns {
			matched[i] = matched[i] || match(at, parts)
		}
	}
	for i, at := range patterns {
		if !matched[i] {
			kind := &declared.Kinds[at.kind]
			kind.Unmatched = append(kind.Unmatched, kind.Patterns[at.pattern])
		}
	}
	products := make([]int, len(patterns))
	for _, file := range now.Files {
		if file.Role != Product {
			continue
		}
		parts := strings.Split(file.Path, "/")
		for i, at := range patterns {
			if matched[i] && match(at, parts) {
				products[i]++
			}
		}
	}
	index := map[string]int{}
	for k, kind := range declared.Kinds {
		index[kind.Name] = k
	}
	sides := [][]File{now.Files, before.Files}
	for side, files := range sides {
		for _, file := range files {
			if file.Kind == "" {
				continue
			}
			if owner := &declared.Kinds[index[file.Kind]]; side == 0 {
				owner.Lines += file.Lines
			} else {
				owner.BaseLines += file.Lines
			}
		}
	}
	for i, at := range patterns {
		if products[i] > 0 {
			kind := &declared.Kinds[at.kind]
			kind.ProductMatches = append(kind.ProductMatches, PatternFiles{Pattern: kind.Patterns[at.pattern], Files: products[i]})
		}
	}
	for k := range declared.Kinds {
		kind := &declared.Kinds[k]
		if !kind.Owns() && len(kind.Unmatched) < len(kind.Patterns) {
			kind.TakenBy = takenBy(table.kinds[k], sides)
		}
		kind.BaseMissing = slices.ContainsFunc(missing, func(p string) bool {
			return slices.ContainsFunc(table.kinds[k].Patterns, func(pattern conventions.PathPattern) bool { return pattern.Match(p) })
		})
	}
	return declared
}

// takenBy is the kind that owns the first test-code file a kind's patterns match, or "": for a kind that owns none,
// an earlier kind that took its files. It stops at the first such file; naming every earlier kind meant matching
// every pattern against every test file (0.8 s on swc's 80,000 test paths).
func takenBy(kind conventions.TestKind, sides [][]File) string {
	for _, files := range sides {
		for _, file := range files {
			if file.Kind != "" && slices.ContainsFunc(kind.Patterns, func(p conventions.PathPattern) bool { return p.Match(file.Path) }) {
				return file.Kind
			}
		}
	}
	return ""
}
