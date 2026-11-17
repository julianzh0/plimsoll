package snapshot

import (
	"bytes"
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

// candidate is a function now that crossed a rule by name, with what is known of it.
type candidate struct {
	change   FunctionChange
	fn       function
	tracked  bool // its file is at the base
	complex  bool // crossed the complexity rule
	deeper   bool // crossed the nesting rule
	reasonOK bool // a reason to mark it: deep nesting or a repeated block the change introduced
}

// gone is a base function no function now has by name: it may have moved or been renamed.
type gone struct {
	path string
	fn   function
}

// functionChanges finds the functions of the changed product files that the change made complex or nested deeper, and
// leaves out those whose code existed before.
func functionChanges(rules conventions.Rules, changes []Change, m measured, missing []string, nowContent, beforeContent map[string][]byte,
	nowReadings, beforeReadings map[string]reading) Functions {
	fr := rules.FunctionRules
	result := Functions{PerMeasure: fr.PerMeasure, MovedShare: fr.MovedShare, CopyShare: fr.CopyShare, MinifiedLineBytes: fr.MinifiedLineBytes,
		Minified: m.minified, Unmeasured: m.unmeasured}
	var candidates []candidate
	var pool []gone
	for _, change := range changes {
		basePath := cmp.Or(change.From, change.Path)
		if !m.now[change.Path] {
			if m.base[basePath] {
				for _, f := range beforeReadings[basePath].functions {
					pool = append(pool, gone{path: basePath, fn: f})
				}
			}
			continue
		}
		r := nowReadings[change.Path]
		b, tracked := beforeReadings[basePath]
		if r.unread || r.functionsUnread || tracked && (b.unread || b.functionsUnread) || slices.Contains(missing, basePath) {
			result.Unread = append(result.Unread, change.Path)
			continue
		}
		result.Files++
		base := byName(b.functions)
		seen := map[string]int{}
		for _, f := range r.functions {
			seen[f.name]++
			if f.broken {
				result.Broken++
				continue
			}
			result.Measured++
			c := candidate{fn: f, tracked: tracked, change: FunctionChange{Path: change.Path, Name: f.name, Occurrence: seen[f.name],
				Start: f.startLine, End: f.endLine, Complexity: f.complexity, Nesting: f.nesting, NestingLine: f.nestingLine, New: true}}
			if partners := base[f.name]; len(partners) >= seen[f.name] {
				partner := partners[seen[f.name]-1]
				if partner.broken {
					result.Broken++
					continue
				}
				c.partner(basePath, partner)
			}
			c.cross(rules)
			if c.complex || c.deeper {
				candidates = append(candidates, c)
			}
		}
		for name, partners := range base {
			for _, f := range partners[min(seen[name], len(partners)):] {
				pool = append(pool, gone{path: basePath, fn: f})
			}
		}
	}
	slices.Sort(result.Unread)
	result.Unread = slices.Compact(result.Unread)
	if len(candidates) == 0 {
		return result
	}
	slices.SortFunc(pool, func(a, b gone) int { return cmp.Or(cmp.Compare(a.path, b.path), a.fn.start-b.fn.start) })
	lines := lineChanges{changes: changes, now: nowContent, before: beforeContent}
	sortByRise(candidates)
	var kept []candidate
	for _, c := range candidates {
		if c.change.New {
			if i := c.likeBody(pool, fr.LikeBody, nowContent, beforeContent); i >= 0 {
				c.partner(pool[i].path, pool[i].fn)
				c.change.ByBody = true
				pool = slices.Delete(pool, i, i+1)
				if c.cross(rules); !c.complex && !c.deeper {
					continue
				}
			}
		}
		added := lines.added(c.change.Path)
		for _, r := range added {
			c.change.AddedLines += max(0, min(r.End, c.change.End)-max(r.Start, c.change.Start)+1)
		}
		lineCount := c.change.End - c.change.Start + 1
		if c.change.New && float64(c.change.AddedLines) < fr.ExtractedShare*float64(lineCount) {
			result.LeftOut = append(result.LeftOut, LeftOutFunction{Path: c.change.Path, Name: c.change.Name, Reason: Extracted})
			continue
		}
		c.change.MovedShare = movedShare(rules, c, added, &lines)
		if c.change.MovedShare >= fr.MovedShare {
			result.LeftOut = append(result.LeftOut, LeftOutFunction{Path: c.change.Path, Name: c.change.Name, Reason: Moved})
			continue
		}
		kept = append(kept, c)
	}
	slices.SortFunc(result.LeftOut, func(a, b LeftOutFunction) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name))
	})
	var complexList, deeperList []FunctionChange
	for _, c := range kept {
		if c.complex && c.reasonOK {
			complexList = append(complexList, c.change)
		}
	}
	slices.SortFunc(complexList, func(a, b FunctionChange) int {
		return cmp.Or(cmp.Compare(b.Complexity-b.ComplexityBefore, a.Complexity-a.ComplexityBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
	})
	result.ComplexCrossed = len(complexList)
	result.Complex = complexList[:min(len(complexList), fr.PerMeasure)]
	listed := map[[2]string]bool{}
	for _, c := range result.Complex {
		listed[[2]string{c.Path, c.Name}] = true
	}
	for _, c := range kept {
		if c.deeper && !listed[[2]string{c.change.Path, c.change.Name}] {
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

// partner makes the base function the candidate's partner.
func (c *candidate) partner(basePath string, b function) {
	c.change.New, c.change.BasePath, c.change.BaseStart, c.change.BaseEnd = false, basePath, b.startLine, b.endLine
	c.change.ComplexityBefore, c.change.NestingBefore = b.complexity, b.nesting
}

// cross applies the two rules to the candidate as it now stands, and finds its reason.
func (c *candidate) cross(rules conventions.Rules) {
	limit, nest := rules.Limits.FunctionComplexity, rules.Limits.FunctionNesting
	f := &c.change
	c.complex = float64(f.Complexity) > limit.Value &&
		(f.New || float64(f.ComplexityBefore) <= limit.Value && float64(f.Complexity-f.ComplexityBefore) >= limit.Step)
	c.deeper = float64(f.Nesting) >= nest.Value && (f.New || f.Nesting > f.NestingBefore)
	f.Repeated = nil
	if f.New && !c.tracked && len(c.fn.repeats) > 0 {
		first := c.fn.repeats[0].copies
		f.Repeated = []LineRange{{first[0].startLine, first[0].endLine}, {first[1].startLine, first[1].endLine}}
	}
	c.reasonOK = f.Nesting >= rules.FunctionRules.ReasonNesting || f.Repeated != nil
}

// sortByRise orders candidates as the complexity measure ranks them: the largest rise first.
func sortByRise(candidates []candidate) {
	slices.SortFunc(candidates, func(a, b candidate) int {
		x, y := a.change, b.change
		return cmp.Or(cmp.Compare(y.Complexity-y.ComplexityBefore, x.Complexity-x.ComplexityBefore), cmp.Compare(y.Complexity, x.Complexity), byPlace(x, y))
	})
}

// likeBody is the first gone function whose body lines, trimmed and without the first, are at least share alike to the
// candidate's; -1 for none.
func (c candidate) likeBody(pool []gone, share float64, nowContent, beforeContent map[string][]byte) int {
	mine := bodyLines(nowContent[c.change.Path], c.fn)
	for i, g := range pool {
		if alike(mine, bodyLines(beforeContent[g.path], g.fn)) >= share {
			return i
		}
	}
	return -1
}

// bodyLines are a function's lines after its first, trimmed of white space.
func bodyLines(content []byte, f function) [][]byte {
	lines := splitLines(content[f.start:f.end])
	if len(lines) <= 1 {
		return nil
	}
	body := make([][]byte, 0, len(lines)-1)
	for _, l := range lines[1:] {
		body = append(body, bytes.TrimSpace(l))
	}
	return body
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
