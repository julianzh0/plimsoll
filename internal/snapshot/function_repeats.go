package snapshot

import (
	"cmp"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// FunctionRepeats is what the function-repeat rule found in a change: the functions the change added that are largely
// the code of one other function, and what it compared. Names, places and numbers only, never code.
type FunctionRepeats struct {
	Listed             []FunctionRepeat // at most PerMeasure, the most alike first
	Found              int              // all pairs found, listed or not
	Files              int              // changed product files of the project's own the rule looked at
	Compared           int              // new functions compared
	Moved              int              // new functions left out because their code moved from a gone function
	FromAnotherProject int              // pairs left out because the new function's file came from another project
	Searched           int              // functions searched for a counterpart
	SearchedFiles      int              // files they are in
	Unreadable         int              // functions among those searched the parser could not read
	Unwalked           []string         // files the rule needed whose functions could not be read
	PerMeasure         int              // the most pairs listed
	Rules              conventions.RepeatRules
}

// CounterpartStatus says whether a function's counterpart existed before the change.
type CounterpartStatus int

// The statuses of a counterpart.
const (
	CounterpartUnchanged CounterpartStatus = iota + 1 // it was there, with the same code as the rule sees it
	CounterpartChanged                                // it was there, and the change changed it
	CounterpartNew                                    // the change wrote it too
)

func (s CounterpartStatus) String() string {
	switch s {
	case CounterpartUnchanged:
		return "unchanged"
	case CounterpartChanged:
		return "changed"
	case CounterpartNew:
		return "new"
	}
	return ""
}

// FunctionRepeat is one function the change added and its counterpart.
type FunctionRepeat struct {
	Path, Name         string
	Occurrence         int // 1 for the first function of its name in its file
	Start, End, Tokens int // lines from 1
	Counterpart        RepeatCounterpart
	Share, Reverse     float64 // of the new function's tokens in runs of the counterpart, and of the counterpart's in the new one's
	Stretch            int     // the most tokens in a row of the new function the counterpart covers
	StretchStart       int     // the lines of that stretch in the new function
	StretchEnd         int
	Others             int // other functions that also met the rule for this one
}

// RepeatCounterpart is the other function of a pair.
type RepeatCounterpart struct {
	Path, Name         string
	Start, End, Tokens int
	Status             CounterpartStatus
}

// repeatFn is one function the rule compares: its file, its place among the file's functions of its name, its entry.
type repeatFn struct {
	path       string
	fn         function
	occurrence int
	entry      string
}

// compare orders functions by path, then offset.
func (f repeatFn) compare(g repeatFn) int {
	return cmp.Or(strings.Compare(f.path, g.path), f.fn.start-g.fn.start)
}

func (f repeatFn) holds(g repeatFn) bool {
	return f.path == g.path && f.fn.start <= g.fn.start && g.fn.end <= f.fn.end
}

// repeatRun is what one question's rule shares.
type repeatRun struct {
	rules                       conventions.Rules
	rr                          conventions.RepeatRules
	changes                     []Change
	nowFiles, baseFiles         map[string]File
	nowContent, baseContent     map[string][]byte
	nowReadings, beforeReadings map[string]reading
	readings                    *Readings
	question                    uint64
	licensed                    map[string]bool
	lacking                     map[string]bool // base paths tracked without content: a partial clone lacks it
	from                        *origins
}

// repeatBaseFiles are the base files read with their functions: those the function rules measure, and the base version
// of every changed file that is itself a measured product file (the repeat rule tests each side on its own).
func repeatBaseFiles(rules conventions.Rules, changes []Change, before Tree, beforeContent map[string][]byte, measured map[string]bool) map[string]bool {
	files := filesByPath(before)
	all := make(map[string]bool, len(measured))
	for p := range measured {
		all[p] = true
	}
	for _, change := range changes {
		basePath := cmp.Or(change.From, change.Path)
		if f, found := files[basePath]; found && measuredFile(rules, f, beforeContent[basePath]) == fileMeasured {
			all[basePath] = true
		}
	}
	return all
}

func filesByPath(t Tree) map[string]File {
	found := make(map[string]File, len(t.Files))
	for _, f := range t.Files {
		found[f.Path] = f
	}
	return found
}

// functionRepeats finds the functions the change added that are largely the code of one other function.
func functionRepeats(run repeatRun) FunctionRepeats {
	result := FunctionRepeats{Rules: run.rr, PerMeasure: run.rules.FunctionRules.PerMeasure}
	gone := run.gone()
	goneGrams := make([]map[uint64]int, len(gone)) // built on first use, once per question
	var cands []repeatFn
	for _, change := range run.changes {
		f, found := run.nowFiles[change.Path]
		r, read := run.nowReadings[change.Path]
		if !found || measuredFile(run.rules, f, run.nowContent[change.Path]) != fileMeasured {
			continue
		}
		if run.outside(change.Path) || !read {
			continue
		}
		result.Files++
		if r.unread || r.functionsUnread {
			result.Unwalked = append(result.Unwalked, change.Path)
			continue
		}
		base, baseRead := run.baseFunctions(cmp.Or(change.From, change.Path))
		if !baseRead {
			result.Unwalked = append(result.Unwalked, change.Path)
			continue
		}
		seen := map[string]int{}
		for _, fn := range r.functions {
			seen[fn.name]++
			c := repeatFn{path: change.Path, fn: fn, occurrence: seen[fn.name], entry: run.entryOf(change.Path)}
			if fn.broken || len(fn.tokens) < run.rr.MinTokens || fn.endLine-fn.startLine+1 < run.rr.MinLines ||
				slices.ContainsFunc(base, func(b function) bool { return b.name == fn.name }) {
				continue
			}
			if run.moved(c, gone, goneGrams) {
				result.Moved++
				continue
			}
			cands = append(cands, c)
		}
	}
	result.Compared = len(cands)
	if len(cands) == 0 {
		return result
	}
	slices.SortFunc(cands, repeatFn.compare)
	best, others := run.search(cands, &result)
	run.list(cands, best, others, &result)
	result.Unwalked = slices.Compact(slices.Sorted(slices.Values(result.Unwalked)))
	return result
}

// gone are the readable functions of the measured base versions of changed files whose names no readable function of
// the file has now: code that may have moved.
func (run repeatRun) gone() []repeatFn {
	var found []repeatFn
	for _, change := range run.changes {
		basePath := cmp.Or(change.From, change.Path)
		names := map[string]bool{}
		if f, ok := run.nowFiles[change.Path]; ok && measuredFile(run.rules, f, run.nowContent[change.Path]) == fileMeasured {
			if now := run.nowReadings[change.Path]; now.unread || now.functionsUnread {
				continue // which functions are gone cannot be told
			}
			for _, fn := range run.nowReadings[change.Path].functions {
				if !fn.broken {
					names[fn.name] = true
				}
			}
		}
		base, _ := run.baseFunctions(basePath)
		for _, fn := range base {
			if !fn.broken && !names[fn.name] {
				found = append(found, repeatFn{path: basePath, fn: fn, entry: run.entryOf(basePath)})
			}
		}
	}
	return found
}

// baseFunctions are the functions of a base version that is a measured product file, readable or not; none for a base
// version that is no measured file or does not exist. read is false when its functions could not be read at all (the
// clone lacks the content, or the parser could not read it): then its old functions cannot be told from new ones.
func (run repeatRun) baseFunctions(basePath string) (functions []function, read bool) {
	f, found := run.baseFiles[basePath]
	if !found && run.lacking[basePath] {
		f, found = File{Path: basePath, Role: role(run.rules, basePath, nil)}, !run.outside(basePath)
	}
	if !found || measuredFile(run.rules, f, run.baseContent[basePath]) != fileMeasured {
		return nil, true
	}
	r, read := run.beforeReadings[basePath] // none when the clone lacks the content, or it is binary
	if !read || r.unread || r.functionsUnread {
		return nil, false
	}
	return r.functions, true
}

// lackingContent are the entries tracked without content and not binary: the content a partial clone lacks.
func lackingContent(entries []Entry) map[string]bool {
	found := map[string]bool{}
	for _, e := range entries {
		if e.Content == nil && !e.Binary {
			found[e.Path] = true
		}
	}
	return found
}

// baseRead says whether a changed file's base version could be read, when it is a measured product file; true for a
// file the change did not touch.
func (run repeatRun) baseRead(filePath string) bool {
	i := slices.IndexFunc(run.changes, func(c Change) bool { return c.Path == filePath })
	if i < 0 {
		return true
	}
	_, read := run.baseFunctions(cmp.Or(run.changes[i].From, filePath))
	return read
}

func (run repeatRun) entryOf(filePath string) string {
	language, _ := run.rules.LanguageOf(filePath)
	return language.Name
}

// moved says whether a gone function of the candidate's entry covers MovedShare of it, names aside. counts holds each
// gone function's runs, filled on first use.
func (run repeatRun) moved(c repeatFn, gone []repeatFn, counts []map[uint64]int) bool {
	var grams []uint64
	for k, g := range gone {
		if g.entry != c.entry {
			continue
		}
		if grams == nil {
			grams = gramsOf(c.fn.tokens, run.rr.Run, withoutNames)
		}
		if counts[k] == nil {
			counts[k] = countGrams(gramsOf(g.fn.tokens, run.rr.Run, withoutNames))
		}
		if share, _, _ := coverage(grams, len(c.fn.tokens), counts[k], run.rr.Run); share >= run.rr.MovedShare {
			return true
		}
	}
	return false
}

// outside says whether a file is not the project's own maintained code: a left-out folder or file name on its path, or
// a folder below the root that holds its own licence file.
func (run repeatRun) outside(filePath string) bool {
	if run.rr.OutsideNames.MatchString(path.Base(filePath)) {
		return true
	}
	parts := strings.Split(path.Dir(filePath), "/")
	for k, part := range parts {
		if run.rr.OutsideFolders.MatchString(part) || run.licensed[strings.Join(parts[:k+1], "/")] {
			return true
		}
	}
	return false
}

// licensedFolders are the folders below the root that hold their own licence file.
func licensedFolders(paths []string, licence *regexp.Regexp) map[string]bool {
	found := map[string]bool{}
	for _, p := range paths {
		if dir := path.Dir(p); dir != "." && licence.MatchString(path.Base(p)) {
			found[dir] = true
		}
	}
	return found
}

// pairScore is one candidate against one source.
type pairScore struct {
	source             repeatFn
	share, reverse     float64
	stretch, stretchAt int
	status             CounterpartStatus
}

// better says whether a source beats the chosen one: one that existed at the base before one the change wrote, then
// the higher share, then the higher reverse share; on a tie the first by path and offset, which the search sees first.
func better(s pairScore, cur *pairScore) bool {
	if cur == nil {
		return true
	}
	if (s.status == CounterpartNew) != (cur.status == CounterpartNew) {
		return cur.status == CounterpartNew
	}
	return s.share > cur.share || s.share == cur.share && s.reverse > cur.reverse
}

// search compares every source that shares a run (names aside) with a candidate, one source at a time, and keeps per
// candidate its best counterpart and how many sources met the rule.
func (run repeatRun) search(cands []repeatFn, result *FunctionRepeats) ([]*pairScore, []int) {
	n := run.rr.Run
	entries := map[string]bool{}
	type place struct {
		path  string
		start int
	}
	isCand := map[place]bool{}
	for _, c := range cands {
		entries[c.entry] = true
		isCand[place{c.path, c.fn.start}] = true
	}
	// the runs of the candidates, indexed once a source may pair with one
	var byGram map[uint64][]int
	var cw1, cw2 [][]uint64
	index := func() {
		byGram = map[uint64][]int{}
		cw1, cw2 = make([][]uint64, len(cands)), make([][]uint64, len(cands))
		for i, c := range cands {
			cw1[i] = gramsOf(c.fn.tokens, n, withoutNames)
			cw2[i] = gramsOf(c.fn.tokens, n, nil)
			for _, g := range cw1[i] {
				if l := byGram[g]; len(l) == 0 || l[len(l)-1] != i {
					byGram[g] = append(byGram[g], i)
				}
			}
		}
	}
	// a source may pair with a candidate of its entry that is not itself, not inside it and not around it; a source
	// that is itself a candidate only with a later one (a pair of new functions is marked once, on the later)
	allowed := make([]bool, len(cands))
	partners := func(s repeatFn) bool {
		any := false
		for i, c := range cands {
			allowed[i] = c.entry == s.entry && (c.path != s.path || c.fn.start != s.fn.start) && !c.holds(s) && !s.holds(c) &&
				(!isCand[place{s.path, s.fn.start}] || s.compare(c) < 0)
			any = any || allowed[i]
		}
		return any
	}
	// a covered stretch of Stretch tokens takes at least Stretch/Run of the source's runs: a shorter source marks none
	shortest := (run.rr.Stretch+n-1)/n + n - 1
	best := make([]*pairScore, len(cands))
	others := make([]int, len(cands))
	for _, s := range run.sources(entries, result) {
		if len(s.fn.tokens) < shortest || !partners(s) {
			continue
		}
		if byGram == nil {
			index()
		}
		sg := sourceGrams{w1: gramsOf(s.fn.tokens, n, withoutNames)}
		seen := map[int]bool{}
		for _, g := range sg.w1 {
			for _, ci := range byGram[g] {
				if seen[ci] || !allowed[ci] {
					continue
				}
				seen[ci] = true
				score, marks := run.score(cands[ci], s, cw1[ci], cw2[ci], &sg)
				if !marks {
					continue
				}
				others[ci]++
				if better(score, best[ci]) {
					best[ci] = &score
				}
			}
		}
	}
	for i := range others {
		others[i] = max(others[i]-1, 0)
	}
	return best, others
}

// sourceGrams are a source's runs in both views and their counts, built on its first pair and kept for the others.
type sourceGrams struct {
	w1, w2 []uint64
	c1, c2 map[uint64]int
}

// score compares a candidate with a source: whether the source marks it, and the numbers.
func (run repeatRun) score(c, s repeatFn, cw1, cw2 []uint64, sg *sourceGrams) (pairScore, bool) {
	n := run.rr.Run
	if sg.c1 == nil {
		sg.w2 = gramsOf(s.fn.tokens, n, nil)
		sg.c1, sg.c2 = countGrams(sg.w1), countGrams(sg.w2)
	}
	share, _, _ := coverage(cw2, len(c.fn.tokens), sg.c2, n)
	_, stretch, at := coverage(cw1, len(c.fn.tokens), sg.c1, n)
	if share < run.rr.Share || stretch < run.rr.Stretch {
		return pairScore{}, false
	}
	reverse, _, _ := coverage(sg.w2, len(s.fn.tokens), countGrams(cw2), n)
	return pairScore{source: s, share: share, reverse: reverse, stretch: stretch, stretchAt: at, status: run.status(s)}, true
}

// status is whether a source existed at the base: its file unchanged, or its base version holding a function of its
// name, readable or not; unchanged when the function of its name at the same place among them (the first, the
// second ...) is readable with the same tokens.
func (run repeatRun) status(s repeatFn) CounterpartStatus {
	i := slices.IndexFunc(run.changes, func(c Change) bool { return c.Path == s.path })
	if i < 0 {
		return CounterpartUnchanged
	}
	basePath := cmp.Or(run.changes[i].From, s.path)
	if _, atBase := run.baseFiles[basePath]; !atBase {
		return CounterpartNew
	}
	base, _ := run.baseFunctions(basePath)
	namesakes := 0
	for _, b := range base {
		if b.name != s.fn.name {
			continue
		}
		namesakes++
		if namesakes == s.occurrence && !b.broken && slices.Equal(b.tokens, s.fn.tokens) {
			return CounterpartUnchanged
		}
	}
	if namesakes > 0 {
		return CounterpartChanged
	}
	return CounterpartNew
}

// sources are the readable functions of the measured product files now of the project's own code in the entries, in
// path and offset order: changed files from the question's readings, others from a light reading of their places.
func (run repeatRun) sources(entries map[string]bool, result *FunctionRepeats) []repeatFn {
	paths := make([]string, 0, len(run.nowFiles))
	for p := range run.nowFiles {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	var found []repeatFn
	for _, p := range paths {
		language, known := run.rules.LanguageOf(p)
		if !known || !entries[language.Name] || run.outside(p) || measuredFile(run.rules, run.nowFiles[p], run.nowContent[p]) != fileMeasured {
			continue
		}
		functions, ok := run.functionsNow(language, p)
		if ok && !run.baseRead(p) { // whether its functions were there before cannot be told
			ok = false
		}
		if !ok {
			result.Unwalked = append(result.Unwalked, p)
			continue
		}
		result.SearchedFiles++
		seen := map[string]int{}
		for _, fn := range functions {
			seen[fn.name]++
			if fn.broken {
				result.Unreadable++
				continue
			}
			result.Searched++
			found = append(found, repeatFn{path: p, fn: fn, occurrence: seen[fn.name], entry: language.Name})
		}
	}
	return found
}

// functionsNow are a file's functions now outside its test code: the question's own reading when it holds them,
// otherwise a light reading of their places and tokens.
func (run repeatRun) functionsNow(language conventions.Language, filePath string) ([]function, bool) {
	r := run.nowReadings[filePath]
	if r.unread {
		return nil, false
	}
	if r.functions != nil || r.functionsUnread {
		return r.functions, !r.functionsUnread
	}
	light := run.readings.of(language, language.GrammarOf(filePath), run.nowContent[filePath], run.question,
		wanted{places: true, functionRules: &run.rules.FunctionRules})
	if light.functionsUnread {
		return nil, false
	}
	var kept []function
	for _, fn := range light.functions {
		if !inSpans(r.testSpans, fn.start) {
			kept = append(kept, fn)
		}
	}
	return kept, true
}

// list keeps the outermost of nested marks, drops those whose file came from another project, and lists the fullest.
func (run repeatRun) list(cands []repeatFn, best []*pairScore, others []int, result *FunctionRepeats) {
	type pair struct {
		FunctionRepeat
		cand int
	}
	var kept []pair
	for i, c := range cands {
		if best[i] == nil {
			continue
		}
		inner := false
		for j, o := range cands {
			if j != i && best[j] != nil && o.holds(c) {
				inner = true
			}
		}
		if inner {
			continue
		}
		if run.from.fromAnotherProject(c.path, run.nowContent[c.path]) {
			result.FromAnotherProject++
			continue
		}
		kept = append(kept, pair{run.repeat(c, *best[i], others[i]), i})
	}
	result.Found = len(kept)
	slices.SortStableFunc(kept, func(a, b pair) int {
		return cmp.Or(cmp.Compare(min(b.Share, b.Reverse), min(a.Share, a.Reverse)), strings.Compare(a.Path, b.Path), a.Start-b.Start)
	})
	for _, p := range kept[:min(run.rules.FunctionRules.PerMeasure, len(kept))] {
		result.Listed = append(result.Listed, p.FunctionRepeat)
	}
}

// repeat is a listed pair's facts.
func (run repeatRun) repeat(c repeatFn, s pairScore, others int) FunctionRepeat {
	r := FunctionRepeat{Path: c.path, Name: c.fn.name, Occurrence: c.occurrence, Start: c.fn.startLine, End: c.fn.endLine,
		Tokens: len(c.fn.tokens),
		Counterpart: RepeatCounterpart{Path: s.source.path, Name: s.source.fn.name,
			Start: s.source.fn.startLine, End: s.source.fn.endLine, Tokens: len(s.source.fn.tokens), Status: s.status},
		Share: s.share, Reverse: s.reverse, Stretch: s.stretch, Others: others}
	r.StretchStart = c.fn.tokenLines[s.stretchAt]
	r.StretchEnd = c.fn.tokenLines[s.stretchAt+s.stretch-1]
	return r
}

// gramsOf are the hashes of every run of n tokens, each token first passed through view (nil keeps it). A token's
// number is mixed before it enters the rolling hash: the numbers differ by single bits (a name's top bit, the literal's
// 1), which the hash's powers of an odd base would otherwise not keep apart between places.
func gramsOf(tokens []uint64, n int, view func(uint64) uint64) []uint64 {
	if len(tokens) < n {
		return nil
	}
	out := make([]uint64, 0, len(tokens)-n+1)
	w := newWindows(n)
	for _, t := range tokens {
		if view != nil {
			t = view(t)
		}
		w.push(mixed(t), func(_ int, hash uint64, _ int) { out = append(out, hash) })
	}
	return out
}

// mixed spreads a number's bits over all 64 (the finalizer of SplitMix64).
func mixed(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

func countGrams(grams []uint64) map[uint64]int {
	counts := make(map[uint64]int, len(grams))
	for _, g := range grams {
		counts[g]++
	}
	return counts
}

// coverage is the share of a's tokens inside runs of n tokens that b also holds, each of b's runs used as often as b
// holds it (a's runs in text order, first come first served), the longest stretch of covered tokens and where it
// starts.
func coverage(aGrams []uint64, tokens int, b map[uint64]int, n int) (share float64, longest, at int) {
	if tokens == 0 {
		return 0, 0, 0
	}
	left := make(map[uint64]int, len(b))
	maps.Copy(left, b)
	covered := make([]bool, tokens)
	for i, h := range aGrams {
		if left[h] > 0 {
			left[h]--
			for k := i; k < i+n; k++ {
				covered[k] = true
			}
		}
	}
	hit, run := 0, 0
	for k, c := range covered {
		if !c {
			run = 0
			continue
		}
		hit++
		run++
		if run > longest {
			longest, at = run, k-run+1
		}
	}
	return float64(hit) / float64(tokens), longest, at
}
