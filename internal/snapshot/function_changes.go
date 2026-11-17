package snapshot

import (
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// measured are the files whose functions are measured on each side, and what is said of the changed product files
// that are not: minified, or of an entry without function tables.
type measured struct {
	now, base            map[string]bool
	minified, unmeasured []string
}

// measuredFiles chooses, now, the changed product files whose entry has function tables and that measuredFile admits;
// at the base, their old paths and the changed product files gone since that it admits.
func measuredFiles(rules conventions.Rules, changes []Change, now, before Tree, nowContent, beforeContent map[string][]byte) measured {
	m := measured{now: map[string]bool{}, base: map[string]bool{}}
	files := func(t Tree) map[string]File {
		found := map[string]File{}
		for _, f := range t.Files {
			found[f.Path] = f
		}
		return found
	}
	nowFiles, baseFiles := files(now), files(before)
	for _, change := range changes {
		basePath := cmp.Or(change.From, change.Path)
		if f, found := nowFiles[change.Path]; found {
			if m.admits(rules, f, nowContent[change.Path]) {
				m.now[change.Path] = true
				m.base[basePath] = true
			}
			continue
		}
		if b, atBase := baseFiles[basePath]; atBase && measuredFile(rules, b, beforeContent[basePath]) == fileMeasured {
			m.base[basePath] = true
		}
	}
	slices.Sort(m.minified)
	slices.Sort(m.unmeasured)
	m.minified, m.unmeasured = slices.Compact(m.minified), slices.Compact(m.unmeasured)
	return m
}

// admits says whether a changed file now is measured, and notes a minified file or an entry without function tables.
func (m *measured) admits(rules conventions.Rules, f File, content []byte) bool {
	switch measuredFile(rules, f, content) {
	case fileMeasured:
		return true
	case fileMinified:
		m.minified = append(m.minified, f.Path)
	case fileWithoutTables:
		m.unmeasured = append(m.unmeasured, f.Language)
	case fileNotProduct:
	}
	return false
}

// fileVerdict is what measuredFile says of one file.
type fileVerdict int

const (
	fileNotProduct fileVerdict = iota
	fileMeasured
	fileMinified
	fileWithoutTables
)

// measuredFile says whether a code file's functions are measured: product code outside the folders and names the
// additions skip, the function rules' skipped folders and benchmark names, not minified (more than MinifiedLineBytes
// bytes a line on average), of an entry with function tables.
func measuredFile(rules conventions.Rules, f File, content []byte) fileVerdict {
	fr := rules.FunctionRules
	if f.Role != Product || !mayDeclare(rules.Additions, f.Path) || inFolders(f.Path, fr.SkippedFolders) || benchmark(fr.BenchmarkWords, f.Path) {
		return fileNotProduct
	}
	if len(content) > fr.MinifiedLineBytes*max(f.Lines, 1) {
		return fileMinified
	}
	if language, known := rules.LanguageOf(f.Path); !known || language.Functions == nil {
		return fileWithoutTables
	}
	return fileMeasured
}

// benchmark says whether a segment of the path holds one of the words as a word of its own: segments split into words
// at every character that is no letter and before an upper-case letter that follows a lower-case one.
func benchmark(words []string, filePath string) bool {
	for _, segment := range strings.Split(filePath, "/") {
		var word []rune
		var prev rune
		flush := func() bool {
			found := slices.Contains(words, strings.ToLower(string(word)))
			word = word[:0]
			return found
		}
		for _, r := range segment {
			if !unicode.IsLetter(r) || unicode.IsUpper(r) && unicode.IsLower(prev) {
				if flush() {
					return true
				}
			}
			if unicode.IsLetter(r) {
				word = append(word, r)
			}
			prev = r
		}
		if flush() {
			return true
		}
	}
	return false
}

// candidate is a function now that crossed a rule, with its partner at the base when it has one.
type candidate struct {
	change  FunctionChange
	complex bool // crossed the complexity rule with a reason
	deeper  bool // crossed the nesting rule
}

// functionChanges finds the functions of the changed product files that the change made complex or nested deeper.
func functionChanges(rules conventions.Rules, changes []Change, m measured, missing []string, nowReadings, beforeReadings map[string]reading) Functions {
	fr := rules.FunctionRules
	limit, nest := rules.Limits.FunctionComplexity, rules.Limits.FunctionNesting
	result := Functions{PerMeasure: fr.PerMeasure, MovedShare: fr.MovedShare, CopyShare: fr.CopyShare, MinifiedLineBytes: fr.MinifiedLineBytes,
		Minified: m.minified, Unmeasured: m.unmeasured}
	var candidates []candidate
	for _, change := range changes {
		if !m.now[change.Path] {
			continue
		}
		r := nowReadings[change.Path]
		basePath := cmp.Or(change.From, change.Path)
		b, tracked := beforeReadings[basePath]
		if r.unread || r.functionsUnread || tracked && (b.unread || b.functionsUnread) || slices.Contains(missing, basePath) {
			result.Unread = append(result.Unread, change.Path)
			continue
		}
		result.Files++
		base := byName(beforeReadings[basePath].functions)
		seen := map[string]int{}
		for _, f := range r.functions {
			seen[f.name]++
			if f.broken {
				result.Broken++
				continue
			}
			result.Measured++
			c := FunctionChange{Path: change.Path, Name: f.name, Occurrence: seen[f.name], Start: f.startLine, End: f.endLine,
				Complexity: f.complexity, Nesting: f.nesting, NestingLine: f.nestingLine, New: true}
			partners := base[f.name]
			if len(partners) >= seen[f.name] {
				b := partners[seen[f.name]-1]
				if b.broken {
					result.Broken++
					continue
				}
				c.New, c.BasePath, c.BaseStart, c.BaseEnd = false, basePath, b.startLine, b.endLine
				c.ComplexityBefore, c.NestingBefore = b.complexity, b.nesting
			}
			complexRise := c.Complexity - c.ComplexityBefore
			crossed := float64(c.Complexity) > limit.Value &&
				(c.New || float64(c.ComplexityBefore) <= limit.Value && float64(complexRise) >= limit.Step)
			deeper := float64(c.Nesting) >= nest.Value && (c.New || c.Nesting > c.NestingBefore)
			if c.New && !tracked && len(f.repeats) > 0 {
				first := f.repeats[0].copies
				c.Repeated = []LineRange{{first[0].startLine, first[0].endLine}, {first[1].startLine, first[1].endLine}}
			}
			reason := c.Nesting >= fr.ReasonNesting || c.Repeated != nil
			if crossed && reason || deeper {
				candidates = append(candidates, candidate{change: c, complex: crossed && reason, deeper: deeper})
			}
		}
	}
	slices.Sort(result.Unread)
	result.Unread = slices.Compact(result.Unread)
	var complexList, deeperList []FunctionChange
	for _, c := range candidates {
		if c.complex {
			complexList = append(complexList, c.change)
		}
	}
	slices.SortFunc(complexList, func(a, b FunctionChange) int {
		return cmp.Or(cmp.Compare(b.Complexity-b.ComplexityBefore, a.Complexity-a.ComplexityBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
	})
	result.ComplexCrossed = len(complexList)
	result.Complex = complexList[:min(len(complexList), fr.PerMeasure)]
	kept := map[[2]string]bool{}
	for _, c := range result.Complex {
		kept[[2]string{c.Path, c.Name}] = true
	}
	for _, c := range candidates {
		if c.deeper && !kept[[2]string{c.change.Path, c.change.Name}] {
			deeperList = append(deeperList, c.change)
		}
	}
	slices.SortFunc(deeperList, func(a, b FunctionChange) int {
		return cmp.Or(cmp.Compare(b.Nesting-b.NestingBefore, a.Nesting-a.NestingBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
	})
	result.DeeperCrossed = len(deeperList)
	result.Deeper = deeperList[:min(len(deeperList), fr.PerMeasure)]
	return result
}

// byPlace orders two functions by path, name and line.
func byPlace(a, b FunctionChange) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Start, b.Start))
}

// byName groups a file's functions by name, in text order.
func byName(functions []function) map[string][]function {
	found := map[string][]function{}
	for _, f := range functions {
		found[f.name] = append(found[f.name], f)
	}
	return found
}
