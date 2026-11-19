package snapshot

import (
	"cmp"
	"hash/fnv"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// FunctionRepeats is what the function-repeat rule found in a change: the functions the change added that are largely
// the code of one other function, and what it compared. Names, places and numbers only, never code.
type FunctionRepeats struct {
	Listed             []FunctionRepeat // at most PerMeasure, the fullest repeat first
	Found              int              // all pairs found, listed or not
	Compared           int              // new functions compared
	Moved              int              // new functions left out because their code moved from a gone function
	FromAnotherProject int              // pairs left out because the new function's file came from another project
	LeftOutFiles       int              // changed product files that are not the project's own maintained code
	Searched           int              // functions searched for a counterpart
	SearchedFiles      int              // files they are in
	Unreadable         int              // functions among those searched the parser could not read
	Unwalked           []string         // files the rule needed whose functions could not be read
	Unavailable        []string         // paths whose content the rule needs and lacks: then nothing is computed
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
	Path, Name             string
	Occurrence             int // 1 for the first function of its name in its file
	Start, End, Tokens     int // lines from 1
	FileNew                bool
	Counterpart            RepeatCounterpart
	Share, Reverse         float64 // of the new function's tokens in runs of the counterpart, and of the counterpart's in the new one's
	Stretch                int     // the most tokens in a row of the new function the counterpart covers
	StretchStart           int     // the lines of that stretch in the new function
	StretchEnd             int
	Others                 int // other functions that also met the rule for this one
	CodeLines, Partnered   int // the new function's lines that hold code, and how many of them have a partner
	Unpartnered            []LineRange
	CounterpartUnpartnered []LineRange
	GaveUp                 bool // the line comparison gave up: no partners or ranges
}

// RepeatCounterpart is the other function of a pair.
type RepeatCounterpart struct {
	Path, Name         string
	Occurrence         int
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

func (f repeatFn) before(g repeatFn) bool {
	return cmp.Or(strings.Compare(f.path, g.path), f.fn.start-g.fn.start) < 0
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
	from                        *origins
	unavailable                 []string // paths that changed and whose base content is missing (Source.Unavailable)
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
	result := FunctionRepeats{Rules: run.rr}
	for _, p := range run.unavailable {
		if _, code := run.rules.LanguageOf(p); code {
			result.Unavailable = append(result.Unavailable, p)
		}
	}
	if len(result.Unavailable) > 0 { // a changed code file the rule cannot read at the base: compare nothing
		return result
	}
	gone := run.gone()
	var cands []repeatFn
	for _, change := range run.changes {
		f, found := run.nowFiles[change.Path]
		r, read := run.nowReadings[change.Path]
		if !found || measuredFile(run.rules, f, run.nowContent[change.Path]) != fileMeasured {
			continue
		}
		if run.outside(change.Path) {
			result.LeftOutFiles++
			continue
		}
		if !read || r.unread || r.functionsUnread {
			continue
		}
		base := run.baseFunctions(cmp.Or(change.From, change.Path))
		seen := map[string]int{}
		for _, fn := range r.functions {
			seen[fn.name]++
			c := repeatFn{path: change.Path, fn: fn, occurrence: seen[fn.name], entry: run.entryOf(change.Path)}
			if fn.broken || len(fn.tokens) < run.rr.MinTokens || fn.endLine-fn.startLine+1 < run.rr.MinLines ||
				slices.ContainsFunc(base, func(b function) bool { return b.name == fn.name }) {
				continue
			}
			if run.moved(c, gone) {
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
	slices.SortFunc(cands, func(a, b repeatFn) int { return cmp.Or(strings.Compare(a.path, b.path), a.fn.start-b.fn.start) })
	best, others := run.search(cands, &result)
	run.list(cands, best, others, &result)
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
			for _, fn := range run.nowReadings[change.Path].functions {
				if !fn.broken {
					names[fn.name] = true
				}
			}
		}
		for _, fn := range run.baseFunctions(basePath) {
			if !fn.broken && !names[fn.name] {
				found = append(found, repeatFn{path: basePath, fn: fn, entry: run.entryOf(basePath)})
			}
		}
	}
	return found
}

// baseFunctions are the functions of a base version that is a measured product file, readable or not; none for a base
// version that is no measured file, that the parser could not read, or that does not exist.
func (run repeatRun) baseFunctions(basePath string) []function {
	f, found := run.baseFiles[basePath]
	if !found || measuredFile(run.rules, f, run.baseContent[basePath]) != fileMeasured {
		return nil
	}
	r := run.beforeReadings[basePath]
	if r.unread || r.functionsUnread {
		return nil
	}
	return r.functions
}

func (run repeatRun) entryOf(filePath string) string {
	language, _ := run.rules.LanguageOf(filePath)
	return language.Name
}

// moved says whether a gone function of the candidate's entry covers MovedShare of it, names aside.
func (run repeatRun) moved(c repeatFn, gone []repeatFn) bool {
	var grams []uint64
	for _, g := range gone {
		if g.entry != c.entry {
			continue
		}
		if grams == nil {
			grams = gramsOf(c.fn.tokens, run.rr.Run, withoutNames)
		}
		if share, _, _ := coverage(grams, len(c.fn.tokens), countGrams(gramsOf(g.fn.tokens, run.rr.Run, withoutNames)), run.rr.Run); share >= run.rr.MovedShare {
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
func licensedFolders(paths []string, licence interface{ MatchString(string) bool }) map[string]bool {
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
			allowed[i] = c.entry == s.entry && !(c.path == s.path && c.fn.start == s.fn.start) && !c.holds(s) && !s.holds(c) &&
				(!isCand[place{s.path, s.fn.start}] || s.before(c))
			any = any || allowed[i]
		}
		return any
	}
	best := make([]*pairScore, len(cands))
	others := make([]int, len(cands))
	for _, s := range run.sources(entries, result) {
		if !partners(s) {
			continue
		}
		if byGram == nil {
			index()
		}
		sw1 := gramsOf(s.fn.tokens, n, withoutNames)
		seen := map[int]bool{}
		for _, g := range sw1 {
			for _, ci := range byGram[g] {
				if seen[ci] || !allowed[ci] {
					continue
				}
				seen[ci] = true
				score, marks := run.score(cands[ci], s, cw1[ci], cw2[ci], sw1)
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

// score compares a candidate with a source: whether the source marks it, and the numbers.
func (run repeatRun) score(c, s repeatFn, cw1, cw2, sw1 []uint64) (pairScore, bool) {
	n := run.rr.Run
	sw2 := gramsOf(s.fn.tokens, n, nil)
	share, _, _ := coverage(cw2, len(c.fn.tokens), countGrams(sw2), n)
	_, stretch, at := coverage(cw1, len(c.fn.tokens), countGrams(sw1), n)
	if share < run.rr.Share || stretch < run.rr.Stretch {
		return pairScore{}, false
	}
	reverse, _, _ := coverage(sw2, len(s.fn.tokens), countGrams(cw2), n)
	return pairScore{source: s, share: share, reverse: reverse, stretch: stretch, stretchAt: at, status: run.status(s)}, true
}

// status is whether a source existed at the base: its file unchanged, or its base version holding a function of its
// name, readable or not; unchanged when that one is readable with the same tokens.
func (run repeatRun) status(s repeatFn) CounterpartStatus {
	i := slices.IndexFunc(run.changes, func(c Change) bool { return c.Path == s.path })
	if i < 0 {
		return CounterpartUnchanged
	}
	basePath := cmp.Or(run.changes[i].From, s.path)
	if _, atBase := run.baseFiles[basePath]; !atBase {
		return CounterpartNew
	}
	for _, b := range run.baseFunctions(basePath) {
		if b.name == s.fn.name {
			if !b.broken && slices.Equal(b.tokens, s.fn.tokens) {
				return CounterpartUnchanged
			}
			return CounterpartChanged
		}
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
	var kept []FunctionRepeat
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
		kept = append(kept, run.repeat(c, *best[i], others[i]))
	}
	result.Found = len(kept)
	slices.SortStableFunc(kept, func(a, b FunctionRepeat) int {
		return cmp.Or(cmp.Compare(min(b.Share, b.Reverse), min(a.Share, a.Reverse)), strings.Compare(a.Path, b.Path), a.Start-b.Start)
	})
	result.Listed = kept[:min(run.rules.FunctionRules.PerMeasure, len(kept))]
	for i := range result.Listed {
		result.Listed[i] = run.aligned(result.Listed[i], cands, best)
	}
}

// repeat is a listed pair's facts.
func (run repeatRun) repeat(c repeatFn, s pairScore, others int) FunctionRepeat {
	_, atBase := run.baseFiles[c.path]
	r := FunctionRepeat{Path: c.path, Name: c.fn.name, Occurrence: c.occurrence, Start: c.fn.startLine, End: c.fn.endLine,
		Tokens: len(c.fn.tokens), FileNew: !atBase && !slices.ContainsFunc(run.changes, func(ch Change) bool { return ch.Path == c.path && ch.From != "" }),
		Counterpart: RepeatCounterpart{Path: s.source.path, Name: s.source.fn.name, Occurrence: s.source.occurrence,
			Start: s.source.fn.startLine, End: s.source.fn.endLine, Tokens: len(s.source.fn.tokens), Status: s.status},
		Share: s.share, Reverse: s.reverse, Stretch: s.stretch, Others: others}
	r.StretchStart = int(c.fn.tokenLines[s.stretchAt])
	r.StretchEnd = int(c.fn.tokenLines[s.stretchAt+s.stretch-1])
	return r
}

// aligned adds what differs: the two functions' code lines aligned, each line taken as its tokens without names.
func (run repeatRun) aligned(r FunctionRepeat, cands []repeatFn, best []*pairScore) FunctionRepeat {
	i := slices.IndexFunc(cands, func(c repeatFn) bool { return c.path == r.Path && c.fn.startLine == r.Start && c.fn.name == r.Name })
	hashesA, linesA := codeLines(cands[i].fn)
	hashesB, linesB := codeLines(best[i].source.fn)
	ids := map[uint64]int32{}
	number := func(hashes []uint64) []int32 {
		out := make([]int32, len(hashes))
		for k, h := range hashes {
			id, found := ids[h]
			if !found {
				id = int32(len(ids))
				ids[h] = id
			}
			out[k] = id
		}
		return out
	}
	keptA, keptB, done := commonDone(number(hashesA), number(hashesB))
	r.CodeLines = len(linesA)
	if !done {
		r.GaveUp = true
		return r
	}
	r.Partnered = countFunc(keptA, func(k bool) bool { return k })
	r.Unpartnered = unpartnered(keptA, linesA)
	r.CounterpartUnpartnered = unpartnered(keptB, linesB)
	return r
}

// codeLines are a function's lines that hold code, each as a hash of its tokens without names, and their numbers.
func codeLines(f function) (hashes []uint64, lines []int) {
	for k := 0; k < len(f.tokens); {
		end := k
		for end < len(f.tokens) && f.tokenLines[end] == f.tokenLines[k] {
			end++
		}
		h := fnv.New64a()
		for _, t := range f.tokens[k:end] {
			writeUint64(h, withoutNames(t))
		}
		hashes, lines = append(hashes, h.Sum64()), append(lines, int(f.tokenLines[k]))
		k = end
	}
	return hashes, lines
}

// unpartnered are the code lines not kept, as ranges of line numbers: consecutive code lines join one range.
func unpartnered(kept []bool, lines []int) []LineRange {
	var ranges []LineRange
	for k, keep := range kept {
		if keep {
			continue
		}
		if n := len(ranges); n > 0 && k > 0 && !kept[k-1] {
			ranges[n-1].End = lines[k]
			continue
		}
		ranges = append(ranges, LineRange{Start: lines[k], End: lines[k]})
	}
	return ranges
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
	for h, k := range b {
		left[h] = k
	}
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
