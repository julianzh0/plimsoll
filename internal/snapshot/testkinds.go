package snapshot

import (
	"fmt"
	"maps"
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
	// Lines and BaseLines are the lines of the test code it owns now and at the base; Owns says it owns test code
	// on either side.
	Lines, BaseLines int
	Owns             bool
	// BaseMissing says a path its patterns match lacks its content (a partial clone), so its base lines are unknown.
	BaseMissing bool
	// Unmatched are its patterns that match no tracked path on either side.
	Unmatched []string
	// ProductMatches are its patterns that match code files that are product code now, each with how many.
	ProductMatches []PatternFiles
	// TakenBy are the earlier kinds that own every test-code file its patterns match, when it owns none.
	TakenBy []string
}

// PatternFiles is a pattern and a number of files it matches.
type PatternFiles struct {
	Pattern string
	Files   int
}

// testsTable is what one side's tests table says: its kinds in file order, and its problem phrases.
type testsTable struct {
	kinds            []TestKind
	seen             map[string]bool
	noEntry, unnamed int
	named            []string
}

// read reads the entry that starts at lines[i], following a list over lines, and gives the index of its last line.
// The name is read before the line is split at "=": a quoted name as written, a bare one up to a space or "=".
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
	isList := strings.HasPrefix(value, "[")
	for isList && unclosed(value) && i+1 < len(lines) {
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
	var items []string
	rest, closed := "", false
	if isList {
		items, rest, closed = stringArray(value[1:])
	}
	if !closed || strings.TrimSpace(uncommented(rest)) != "" {
		t.named = append(t.named, fmt.Sprintf("`[%s]` `%s` is not a list of text", table, name))
		return i
	}
	kind := TestKind{Name: name}
	for _, item := range items {
		if _, usable := conventions.NewPathPattern(item); usable {
			kind.Patterns = appendNew(kind.Patterns, item)
		} else {
			kind.Unusable++
		}
	}
	if len(kind.Patterns) > 0 || kind.Unusable > 0 {
		t.kinds = append(t.kinds, kind)
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
		for _, pattern := range slices.Sorted(slices.Values(kind.Patterns)) {
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

// testKindRules are the kinds of now's tests table as the rules match them.
func testKindRules(t testsTable) []conventions.TestKind {
	var kinds []conventions.TestKind
	for _, kind := range t.kinds {
		rule := conventions.TestKind{Name: kind.Name}
		for _, text := range kind.Patterns {
			pattern, _ := conventions.NewPathPattern(text)
			rule.Patterns = append(rule.Patterns, pattern)
		}
		kinds = append(kinds, rule)
	}
	return kinds
}

// testKinds measures each declared kind on both sides: the lines it owns, its patterns that match nothing or match
// product code now, the earlier kinds that took its files, and whether a path it matches lacks its content.
func testKinds(rules conventions.Rules, table testsTable, now, before Tree, missing []string) TestKinds {
	declared := TestKinds{Table: rules.TestsTable}
	if len(table.kinds) == 0 {
		return declared
	}
	tracked := slices.Concat(now.Paths, before.Paths)
	for k, kind := range table.kinds {
		rule := rules.TestKinds[k]
		matches := func(p string) bool {
			return slices.ContainsFunc(rule.Patterns, func(pattern conventions.PathPattern) bool { return pattern.Match(p) })
		}
		owners := map[string]bool{}
		for _, side := range []struct {
			files []File
			lines *int
		}{{now.Files, &kind.Lines}, {before.Files, &kind.BaseLines}} {
			for _, file := range side.files {
				switch {
				case file.Kind == kind.Name:
					*side.lines += file.Lines
					kind.Owns = true
				case file.Role.IsTest() && matches(file.Path):
					owners[file.Kind] = true
				}
			}
		}
		if !kind.Owns {
			kind.TakenBy = slices.Sorted(maps.Keys(owners))
		}
		kind.BaseMissing = slices.ContainsFunc(missing, matches)
		for _, pattern := range rule.Patterns {
			if !slices.ContainsFunc(tracked, pattern.Match) {
				kind.Unmatched = append(kind.Unmatched, pattern.Text())
			}
			product := 0
			for _, file := range now.Files {
				if file.Role == Product && pattern.Match(file.Path) {
					product++
				}
			}
			if product > 0 {
				kind.ProductMatches = append(kind.ProductMatches, PatternFiles{Pattern: pattern.Text(), Files: product})
			}
		}
		declared.Kinds = append(declared.Kinds, kind)
	}
	return declared
}
