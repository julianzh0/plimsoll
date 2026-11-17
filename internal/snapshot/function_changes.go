package snapshot

import (
	"bytes"
	"cmp"
	"slices"
	"strings"
	"unicode"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// measured are the files whose functions are measured on each side, and the changed product files that are not
// because they are minified.
type measured struct {
	now, base map[string]bool
	minified  []string
}

// measuredFiles chooses, now, the changed product files that measuredFile admits;
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
	m.minified = slices.Compact(m.minified)
	return m
}

// admits says whether a changed file now is measured, and notes a minified file.
func (m *measured) admits(rules conventions.Rules, f File, content []byte) bool {
	switch measuredFile(rules, f, content) {
	case fileMeasured:
		return true
	case fileMinified:
		m.minified = append(m.minified, f.Path)
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
)

// measuredFile says whether a code file's functions are measured: product code outside the folders and names the
// additions skip, the function rules' skipped folders and benchmark names, not minified (more than MinifiedLineBytes
// bytes a line on average), of a language entry (each has function tables).
func measuredFile(rules conventions.Rules, f File, content []byte) fileVerdict {
	fr := rules.FunctionRules
	if f.Role != Product || !mayDeclare(rules.Additions, f.Path) || inFolders(f.Path, fr.SkippedFolders) || benchmark(fr.BenchmarkWords, f.Path) {
		return fileNotProduct
	}
	if len(content) > fr.MinifiedLineBytes*max(f.Lines, 1) {
		return fileMinified
	}
	if _, known := rules.LanguageOf(f.Path); !known {
		return fileNotProduct
	}
	return fileMeasured
}

// benchmark says whether a segment of the path holds one of the words as a word of its own: segments split into words
// at every character that is no letter and before an upper-case letter that follows a lower-case one.
func benchmark(words []string, filePath string) bool {
	for segment := range strings.SplitSeq(filePath, "/") {
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
	file    Change // the change of its file
	change  FunctionChange
	fn      function
	base    *function     // its partner at the base
	tokens  []token       // its tokens, lines from its first
	added   []bool        // per line from its first: the change added it
	counted []bool        // per token: weighed by the moved and copied shares (every token of a new function, else those on added lines)
	blocks  []blockWindow // per repeat of fn: its first copy as a window of tokens
	complex bool          // crossed the complexity rule
	deeper  bool          // crossed the nesting rule
}

// gone is a base function no function now has by name: it may have moved or been renamed.
type gone struct {
	path string
	fn   function
	body [][]byte // its body lines, made on first need
}

// functionChanges finds the functions of the changed product files that the change made complex or nested deeper, and
// leaves out those whose code existed before.
func functionChanges(rules conventions.Rules, changes []Change, m measured, missing []string, atBase map[string]bool, now, before Tree,
	nowContent, beforeContent map[string][]byte, nowReadings, beforeReadings map[string]reading) Functions {
	fr := rules.FunctionRules
	result := Functions{PerMeasure: fr.PerMeasure, Run: fr.Run, MovedShare: fr.MovedShare, CopyShare: fr.CopyShare, MinifiedLineBytes: fr.MinifiedLineBytes, HeaderLines: fr.HeaderLines,
		Minified: m.minified, Checks: functionChecks}
	candidates, pool := collect(rules, changes, m, missing, atBase, nowReadings, beforeReadings, &result)
	if len(candidates) == 0 {
		return result
	}
	run := functionRun{rules: rules, result: &result, pool: pool, now: nowContent, base: beforeContent,
		lines: newLineChanges(changes, nowContent, beforeContent),
		from:  origins{rules: fr, nowPaths: now.Paths, nowContent: nowContent, baseFiles: before.Files, base: beforeContent}}
	slices.SortFunc(candidates, func(a, b candidate) int { return byRise(a.change, b.change) })
	var open []candidate
	for _, c := range candidates {
		if c, stays := run.screen(c); stays {
			open = append(open, c)
		}
	}
	run.list(run.notMoved(open), before.Files)
	slices.SortFunc(result.LeftOut, func(a, b LeftOutFunction) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Occurrence, b.Occurrence))
	})
	return result
}

// functionRun is what one question's function rules share once the candidates are collected.
type functionRun struct {
	rules     conventions.Rules
	result    *Functions
	pool      []gone // base functions no function now has by name, until a candidate matches one by body
	now, base map[string][]byte
	lines     *lineChanges
	from      origins
}

func (r *functionRun) leaveOut(c candidate, reason LeftOutReason, source string) {
	r.result.LeftOut = append(r.result.LeftOut, LeftOutFunction{Path: c.change.Path, Name: c.change.Name, Occurrence: c.change.Occurrence, Reason: reason, Source: source})
}

// screen applies the checks that need no other candidate: a body that existed under another name, another project's
// file, code split out of existing code. It returns the candidate with its added lines and tokens, and whether it stays.
func (r *functionRun) screen(c candidate) (candidate, bool) {
	fr := r.rules.FunctionRules
	if c.change.New {
		if i := c.likeBody(r.pool, fr.LikeBody, r.now, r.base); i >= 0 {
			g := r.pool[i]
			r.pool = slices.Delete(r.pool, i, i+1)
			if g.fn.broken {
				r.result.Broken++ // its code existed under another name but cannot be compared
				return c, false
			}
			c.partner(g.path, g.fn)
			c.change.ByBody = true
			if c.cross(r.rules); !c.complex && !c.deeper {
				return c, false
			}
		}
	}
	// a file whose header or folder says it is another project's holds that project's code, whatever the change did
	if r.from.fromAnotherProject(c.change.Path, r.now[c.change.Path]) {
		r.leaveOut(c, FromAnotherProject, "")
		return c, false
	}
	c.added = addedMask(r.lines.of(c.file).added, c.change.Start, c.change.End)
	c.change.AddedLines = countFunc(c.added, func(a bool) bool { return a })
	if c.change.New && float64(c.change.AddedLines) < fr.ExtractedShare*float64(len(c.added)) {
		r.leaveOut(c, Extracted, "")
		return c, false
	}
	c.read(r.rules, r.now[c.change.Path])
	return c, true
}

// notMoved reads the removed code once for all open candidates, leaves out those whose code was moved from elsewhere,
// and finds the repeated block each remaining one introduced.
func (r *functionRun) notMoved(open []candidate) []candidate {
	if len(open) == 0 {
		return nil
	}
	run := r.rules.FunctionRules.Run
	wantRuns, wantBlocks := map[uint64]bool{}, map[blockWindow]bool{}
	bodies := make([]oldBody, len(open))
	for k, c := range open {
		for _, h := range windowHashes(c.tokens, run) {
			wantRuns[h] = true
		}
		for _, b := range c.blocks {
			wantBlocks[b] = true
		}
		if !c.change.New {
			bodies[k] = oldBody{path: c.change.BasePath, start: c.change.BaseStart, end: c.change.BaseEnd}
		}
	}
	removed := readRemoved(r.rules, r.lines, wantRuns, wantBlocks, bodies)
	var kept []candidate
	for k, c := range open {
		c.change.MovedShare = movedShare(c, run, removed, removed.own[k])
		if c.change.MovedShare >= r.rules.FunctionRules.MovedShare {
			r.leaveOut(c, Moved, "")
			continue
		}
		c.change.Repeated = introducedBlock(c, removed)
		kept = append(kept, c)
	}
	return kept
}

// list runs both measures over the kept candidates, the complexity first, and counts what crossed each.
func (r *functionRun) list(kept []candidate, baseFiles []File) {
	nest := r.rules.Limits.FunctionNesting.Value
	copies := copyChecks{rules: r.rules, baseFiles: baseFiles, base: r.base, now: r.now,
		checks: map[place]copyCheck{}, copied: map[place]bool{}, starved: map[place]bool{}, leaveOut: r.leaveOut}
	complexList := slices.DeleteFunc(slices.Clone(kept), func(c candidate) bool {
		// a reason to mark it: deep nesting or a repeated block the change introduced
		return !c.complex || float64(c.change.Nesting) < nest && c.change.Repeated == nil
	})
	r.result.Complex = copies.measure(complexList, byRise)
	listed := map[place]bool{}
	for _, c := range r.result.Complex {
		listed[placeOf(c)] = true
	}
	deeperList := slices.DeleteFunc(slices.Clone(kept), func(c candidate) bool {
		return !c.deeper || listed[placeOf(c.change)] || copies.copied[placeOf(c.change)]
	})
	r.result.Deeper = copies.measure(deeperList, byDepth)
	// counted after both measures, so a function the second measure found copied or compared counts once
	standing := func(c candidate) bool { return !copies.copied[placeOf(c.change)] }
	r.result.ComplexCrossed = countFunc(complexList, standing)
	r.result.DeeperCrossed = countFunc(deeperList, standing)
	for key := range copies.starved {
		if _, done := copies.checks[key]; !done && !copies.copied[key] {
			r.result.Uncompared++
		}
	}
}

// collect counts the measured files and functions into result and returns the functions that crossed a rule by name,
// and the pool of base functions no function now has by name.
func collect(rules conventions.Rules, changes []Change, m measured, missing []string, atBase map[string]bool,
	nowReadings, beforeReadings map[string]reading, result *Functions) (candidates []candidate, pool []gone) {
	for _, change := range changes {
		basePath := cmp.Or(change.From, change.Path)
		b, readAtBase := beforeReadings[basePath]
		if !m.now[change.Path] {
			if m.base[basePath] {
				pool = append(pool, goneFrom(basePath, b.functions)...)
			}
			continue
		}
		r, readNow := nowReadings[change.Path]
		tracked := atBase[basePath]
		if !readNow || r.unread || r.functionsUnread || tracked && (!readAtBase || b.unread || b.functionsUnread) || slices.Contains(missing, basePath) {
			result.Unread = append(result.Unread, change.Path)
			continue
		}
		result.Files++
		crossed, unmatched := collectFile(rules, change, tracked, r.functions, b.functions, result)
		candidates = append(candidates, crossed...)
		pool = append(pool, goneFrom(basePath, unmatched)...)
	}
	slices.Sort(result.Unread)
	result.Unread = slices.Compact(result.Unread)
	slices.SortFunc(pool, func(a, b gone) int { return cmp.Or(cmp.Compare(a.path, b.path), a.fn.start-b.fn.start) })
	return candidates, pool
}

// collectFile pairs a file's functions now with those of its base version by name and place, counts them into result,
// and returns those that crossed a rule and the base functions no function now has by name.
func collectFile(rules conventions.Rules, change Change, tracked bool, functions, baseFunctions []function, result *Functions) (crossed []candidate, unmatched []function) {
	base := byName(baseFunctions)
	seen := map[string]int{}
	for _, f := range functions {
		seen[f.name]++
		if f.broken {
			result.Broken++
			continue
		}
		c := candidate{file: change, fn: f, change: FunctionChange{Path: change.Path, Name: f.name, Occurrence: seen[f.name],
			Start: f.startLine, End: f.endLine, Complexity: f.complexity, Nesting: f.nesting, NestingLine: f.nestingLine, New: true,
			AddedWhole: !tracked && change.From == ""}}
		c.cross(rules)
		partners := base[f.name]
		if len(partners) >= seen[f.name] && partners[seen[f.name]-1].broken {
			// it cannot be compared; it is counted only when it could have crossed as new
			if c.complex || c.deeper {
				result.Broken++
			} else {
				result.Measured++
			}
			continue
		}
		if len(partners) >= seen[f.name] {
			c.partner(cmp.Or(change.From, change.Path), partners[seen[f.name]-1])
			c.cross(rules)
		}
		result.Measured++
		if c.complex || c.deeper {
			crossed = append(crossed, c)
		}
	}
	for name, partners := range base {
		unmatched = append(unmatched, partners[min(seen[name], len(partners)):]...)
	}
	return crossed, unmatched
}

// goneFrom puts base functions into the pool, with the path of their file.
func goneFrom(path string, functions []function) []gone {
	found := make([]gone, len(functions))
	for i, f := range functions {
		found[i] = gone{path: path, fn: f}
	}
	return found
}

// read takes the candidate's tokens from its file's content, which tokens the shares weigh, and its repeated blocks as
// windows of tokens.
func (c *candidate) read(rules conventions.Rules, content []byte) {
	if language, known := rules.LanguageOf(c.change.Path); known && language.Tokens != nil {
		c.tokens = tokensOf(language.Tokens, content[c.fn.start:c.fn.end], c.change.Start)
	}
	c.counted = make([]bool, len(c.tokens))
	for i, t := range c.tokens {
		c.counted[i] = c.change.New || c.added[t.line-c.change.Start]
	}
	c.blocks = make([]blockWindow, len(c.fn.repeats))
	for k, r := range c.fn.repeats {
		c.blocks[k] = c.blockOf(r)
	}
}

// byRise ranks as the complexity measure does: the largest rise first, then the most complex.
func byRise(a, b FunctionChange) int {
	return cmp.Or(cmp.Compare(b.Complexity-b.ComplexityBefore, a.Complexity-a.ComplexityBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
}

// byDepth ranks as the nesting measure does: the largest rise in depth first, then the most complex.
func byDepth(a, b FunctionChange) int {
	return cmp.Or(cmp.Compare(b.Nesting-b.NestingBefore, a.Nesting-a.NestingBefore), cmp.Compare(b.Complexity, a.Complexity), byPlace(a, b))
}

// copyChecks compares candidates with the code at the base for both measures of one question: a function compared
// once is not compared again, and each measure spends at most functionChecks comparisons.
type copyChecks struct {
	rules      conventions.Rules
	baseFiles  []File
	base, now  map[string][]byte
	byLanguage map[string][]File // the base files of each entry with content, made on first need
	checks     map[place]copyCheck
	copied     map[place]bool
	starved    map[place]bool // reached by a measure after its checks were spent
	leaveOut   func(c candidate, reason LeftOutReason, source string)
}

// measure keeps, in rank order, the candidates no copy at the base explains, until PerMeasure are kept or its
// functionChecks comparisons are spent.
func (cc *copyChecks) measure(list []candidate, rank func(a, b FunctionChange) int) (kept []FunctionChange) {
	slices.SortFunc(list, func(a, b candidate) int { return rank(a.change, b.change) })
	spent := 0
	for i, c := range list {
		key := placeOf(c.change)
		if len(kept) == cc.rules.FunctionRules.PerMeasure {
			break
		}
		if cc.copied[key] {
			continue
		}
		check, done := cc.checks[key]
		if !done {
			if spent == functionChecks {
				for _, rest := range list[i:] {
					cc.starved[placeOf(rest.change)] = true
				}
				break
			}
			check = copyAtBase(cc.rules, c, cc.filesOf(c.change.Path), cc.base, cc.now[c.change.Path])
			cc.checks[key] = check
			spent++
		}
		if check.source != "" {
			cc.copied[key] = true
			cc.leaveOut(c, Copied, check.source)
			continue
		}
		c.change.CopyFiles, c.change.ClosestFile, c.change.ClosestShare = check.files, check.closest, check.closestShare
		kept = append(kept, c.change)
	}
	return kept
}

// filesOf are the base files of the path's language entry that have content.
func (cc *copyChecks) filesOf(path string) []File {
	language, known := cc.rules.LanguageOf(path)
	if !known {
		return nil
	}
	if cc.byLanguage == nil {
		cc.byLanguage = map[string][]File{}
		for _, f := range cc.baseFiles {
			if other, readable := cc.rules.LanguageOf(f.Path); readable && cc.base[f.Path] != nil {
				cc.byLanguage[other.Name] = append(cc.byLanguage[other.Name], f)
			}
		}
	}
	return cc.byLanguage[language.Name]
}

// partner makes the base function the candidate's partner.
func (c *candidate) partner(basePath string, b function) {
	c.change.New, c.change.BasePath, c.change.BaseStart, c.change.BaseEnd = false, basePath, b.startLine, b.endLine
	c.change.ComplexityBefore, c.change.NestingBefore = b.complexity, b.nesting
	c.base = &b
}

// cross applies the two rules to the candidate as it now stands.
func (c *candidate) cross(rules conventions.Rules) {
	limit, nest := rules.Limits.FunctionComplexity, rules.Limits.FunctionNesting
	f := &c.change
	c.complex = float64(f.Complexity) > limit.Value &&
		(f.New || float64(f.ComplexityBefore) <= limit.Value && float64(f.Complexity-f.ComplexityBefore) >= limit.Step)
	c.deeper = float64(f.Nesting) >= nest.Value && (f.New || f.Nesting > f.NestingBefore)
}

// likeBody is the first gone function whose body lines, trimmed and without the first, are at least share alike to the
// candidate's; -1 for none. A pair whose line counts alone keep it below the share is not compared.
func (c candidate) likeBody(pool []gone, share float64, nowContent, beforeContent map[string][]byte) int {
	mine := bodyLines(nowContent[c.change.Path], c.fn)
	for i := range pool {
		g := &pool[i]
		if g.body == nil {
			g.body = bodyLines(beforeContent[g.path], g.fn)
		}
		if 2*float64(min(len(mine), len(g.body))) < share*float64(len(mine)+len(g.body)) {
			continue
		}
		if alike(mine, g.body) >= share {
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

// byPlace orders two functions by path, name, place among functions of that name and line.
func byPlace(a, b FunctionChange) int {
	return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Occurrence, b.Occurrence), cmp.Compare(a.Start, b.Start))
}

// place names one function of the files now: its path, name and place among functions of that name.
type place struct {
	path, name string
	occurrence int
}

func placeOf(c FunctionChange) place { return place{c.Path, c.Name, c.Occurrence} }

// byName groups a file's functions by name, in text order.
func byName(functions []function) map[string][]function {
	found := map[string][]function{}
	for _, f := range functions {
		found[f.name] = append(found[f.name], f)
	}
	return found
}

func countFunc[T any](list []T, keep func(T) bool) int {
	n := 0
	for _, x := range list {
		if keep(x) {
			n++
		}
	}
	return n
}
