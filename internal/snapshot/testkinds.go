package snapshot

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// TestKinds is what now's tests table declares, and what each kind holds on both sides.
type TestKinds struct {
	Table      string     // the table's name in the own file
	Kinds      []TestKind // in file order
	PatternCap int        // the most usable patterns of the table that are matched
	// RemainderBaseMissing says a test path no kind owns lacks its content at the base (a partial clone), so the
	// base of the test lines of no declared kind is unknown.
	RemainderBaseMissing bool
}

// TestKind is one kind of test code the repository declares, with what it finds. A test-code file belongs to the
// first kind, in file order, with a pattern that matches its path; a kind never changes what a file is.
type TestKind struct {
	Name     string
	Patterns []string // its usable patterns, in file order, each once
	Unusable int      // its patterns that have not the form of a path pattern
	// Over are its usable patterns beyond the first conventions.TestPatterns of the table: declared, not matched.
	Over int
	// Files and BaseFiles are the test-code files it owns now and at the base; Lines and BaseLines their lines.
	Files, BaseFiles, Lines, BaseLines int
	// InUse are its patterns, in file order, that are the first of its patterns to match a test-code file it owns.
	InUse []string
	// BaseMissing says a test path it owns lacks its content at the base (a partial clone), so its base is unknown.
	BaseMissing bool
	// Unmatched are its patterns that match no tracked path on either side.
	Unmatched []string
	// ProductMatches are its patterns that match code files that are product code now, each with how many.
	ProductMatches []PatternFiles
	// TakenBy are, when it owns no test code, the earlier kinds, in file order, that own test-code files its patterns
	// match.
	TakenBy []string
}

// Owns says whether the kind owns a test-code file on either side, an empty one included.
func (k TestKind) Owns() bool { return k.Files > 0 || k.BaseFiles > 0 }

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
	first := i
	for depth := listDepth(0, value); depth > 0 && i+1 < len(lines) && listContinues(lines[i+1]); i++ {
		depth = listDepth(depth, lines[i+1])
	}
	value = strings.Join(append([]string{strings.TrimSpace(value)}, lines[first+1:i+1]...), "\n")
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

// used are the kinds as matching uses them: in file order, the table's first conventions.TestPatterns usable patterns,
// so a hostile table cannot make every question try thousands of patterns on every path. A kind keeps its place
// with no pattern when all of its patterns lie beyond.
func (t testsTable) used() []conventions.TestKind {
	kinds, left := make([]conventions.TestKind, len(t.kinds)), conventions.TestPatterns
	for k, kind := range t.kinds {
		n := min(left, len(kind.Patterns))
		kinds[k] = conventions.TestKind{Name: kind.Name, Patterns: kind.Patterns[:n]}
		left -= n
	}
	return kinds
}

// listDepth is the depth of open square brackets after a line of a list, given the depth before it; once they close,
// the rest of the line does not matter. Strings do not span lines, so a string left open ends the list, and a "#"
// outside a string ends the line.
func listDepth(depth int, line string) int {
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '#':
			return depth
		case '[':
			depth++
		case ']':
			if depth--; depth <= 0 {
				return 0
			}
		case '"', '\'':
			for i++; i < len(line) && line[i] != c && line[i] != '\n'; i++ {
				if c == '"' && line[i] == '\\' {
					i++
				}
			}
			if i >= len(line) || line[i] != c {
				return 0
			}
		}
	}
	return depth
}

// listContinues says whether a line can go on a list of text: it starts with a string that no "=" follows, a comma,
// the closing bracket or a comment, or is empty. Any other line, a table header or the next entry (a quoted name
// included), ends the list, so a list left open never takes the entries after it.
func listContinues(line string) bool {
	trimmed := strings.TrimSpace(line)
	if _, rest, quoted := quotedString(trimmed); quoted {
		return !strings.HasPrefix(strings.TrimLeft(rest, " \t"), "=")
	}
	return trimmed == "" || strings.ContainsRune(`"',]#`, rune(trimmed[0]))
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

// testKinds measures each declared kind on both sides: the files and lines it owns and the patterns that match them,
// its patterns that match nothing or match product code now, the earlier kinds that took its files, and whether a
// path it owns lacks its base content (lacking: the base paths a partial clone lacks). Each path is split once and
// tried against the patterns that have not matched yet: on swc's 83,000 paths with twenty patterns, trying every
// pattern on every path took 1.7 s.
func testKinds(rules conventions.Rules, table testsTable, now, before Tree, lacking []string) TestKinds {
	declared := TestKinds{Table: rules.TestsTable, PatternCap: conventions.TestPatterns}
	kinds := table.used()
	type patternAt struct{ kind, pattern int }
	var patterns []patternAt
	for k, kind := range kinds {
		declared.Kinds = append(declared.Kinds, TestKind{Name: kind.Name, Patterns: texts(kind.Patterns), Unusable: table.unusable[k],
			Over: len(table.kinds[k].Patterns) - len(kind.Patterns)})
		for p := range kind.Patterns {
			patterns = append(patterns, patternAt{k, p})
		}
	}
	if len(patterns) == 0 {
		return declared
	}
	match := func(at patternAt, parts []string) bool {
		return kinds[at.kind].Patterns[at.pattern].MatchSegments(parts)
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
	for i, at := range patterns {
		if products[i] > 0 {
			kind := &declared.Kinds[at.kind]
			kind.ProductMatches = append(kind.ProductMatches, PatternFiles{Pattern: kind.Patterns[at.pattern], Files: products[i]})
		}
	}
	index := map[string]int{}
	for k, kind := range declared.Kinds {
		index[kind.Name] = k
	}
	inUse := make([][]bool, len(kinds))
	for k, kind := range kinds {
		inUse[k] = make([]bool, len(kind.Patterns))
	}
	use := func(k int, filePath string) {
		for p, pattern := range kinds[k].Patterns {
			inUse[k][p] = inUse[k][p] || pattern.Match(filePath)
		}
	}
	sides := [][]File{now.Files, before.Files}
	for side, files := range sides {
		for _, file := range files {
			if file.Kind == "" {
				continue
			}
			k := index[file.Kind]
			use(k, file.Path)
			if owner := &declared.Kinds[k]; side == 0 {
				owner.Files++
				owner.Lines += file.Lines
			} else {
				owner.BaseFiles++
				owner.BaseLines += file.Lines
			}
		}
	}
	for _, p := range lacking {
		if excluded(rules, p) {
			continue
		}
		owner := ""
		if testPath(rules, p) {
			owner = conventions.KindOf(rules.TestKinds, p)
		}
		switch language, known := rules.LanguageOf(p); {
		case owner != "":
			declared.Kinds[index[owner]].BaseMissing = true
			use(index[owner], p)
		case testPath(rules, p) || known && language.HoldsTestCode():
			declared.RemainderBaseMissing = true
		}
	}
	for k := range declared.Kinds {
		kind := &declared.Kinds[k]
		for p, used := range inUse[k] {
			if used {
				kind.InUse = append(kind.InUse, kind.Patterns[p])
			}
		}
		if !kind.Owns() && !kind.BaseMissing && len(kind.Unmatched) < len(kind.Patterns) {
			kind.TakenBy = takenBy(kinds[k], sides, declared.Kinds[:k])
		}
	}
	return declared
}

// takenBy are the earlier kinds, in file order, that own test-code files a kind's patterns match: for a kind that
// owns none, the kinds that took its files. It runs only for such a kind and passes over a file whose kind it has
// already named, so it stays one pass over the test files.
func takenBy(kind conventions.TestKind, sides [][]File, earlier []TestKind) []string {
	found := map[string]bool{}
	for _, files := range sides {
		for _, file := range files {
			if file.Kind == "" || file.Kind == kind.Name || found[file.Kind] {
				continue
			}
			if slices.ContainsFunc(kind.Patterns, func(p conventions.PathPattern) bool { return p.Match(file.Path) }) {
				found[file.Kind] = true
			}
		}
	}
	var owners []string
	for _, k := range earlier {
		if found[k.Name] {
			owners = append(owners, k.Name)
		}
	}
	return owners
}
