package snapshot

import (
	"cmp"
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// removedCode is what the lines a change removed hold, read once per question: how often each wanted run of Run tokens
// stands there, and how often each wanted block (a whole run of tokens of one length) stands there without overlap.
// own[k] counts the runs that stand inside the old body of the k-th function asked about: that function's own code
// is not code moved from elsewhere.
type removedCode struct {
	runs   map[uint64]int
	blocks map[blockWindow]int
	own    []map[uint64]int
}

// oldBody is where a function that existed stood at the base.
type oldBody struct {
	path       string
	start, end int // lines from 1
}

// blockWindow is a block's tokens as a window: its length and hash.
type blockWindow struct {
	length int
	hash   uint64
}

// readRemoved streams the removed ranges of every changed code file once, each range as its own stream of tokens,
// counting the wanted runs and blocks. Nothing of the removed text is kept.
func readRemoved(rules conventions.Rules, lines *lineChanges, runs map[uint64]bool, blocks map[blockWindow]bool, bodies []oldBody) removedCode {
	run := rules.FunctionRules.Run
	found := removedCode{runs: map[uint64]int{}, blocks: map[blockWindow]int{}, own: make([]map[uint64]int, len(bodies))}
	bodiesIn := map[string][]int{}
	for k, b := range bodies {
		found.own[k] = map[uint64]int{}
		bodiesIn[b.path] = append(bodiesIn[b.path], k)
	}
	lengths := []int{rules.FunctionRules.Run}
	for b := range blocks {
		if !slices.Contains(lengths, b.length) {
			lengths = append(lengths, b.length)
		}
	}
	slices.Sort(lengths)
	for _, change := range lines.changes {
		basePath := cmp.Or(change.From, change.Path)
		language, readable := rules.LanguageOf(basePath)
		base := lines.before[basePath]
		if !readable || language.Tokens.Pattern == nil || base == nil {
			continue
		}
		newlines := linesOf(base)
		for _, r := range lines.of(change).removed {
			w := newWindows(lengths...)
			nextFree := map[blockWindow]int{} // a block's copies count only when they do not overlap
			lineOf := make([]int, run)        // the lines of the last Run tokens, by their index modulo Run
			n := 0
			for t := range eachToken(language.Tokens, lineText(base, newlines, r), r.Start) {
				lineOf[n%run] = t.line
				n++
				w.push(t.hash, func(length int, hash uint64, first int) {
					if length == run && runs[hash] {
						found.runs[hash]++
						for _, k := range bodiesIn[basePath] {
							if b := bodies[k]; b.start <= lineOf[first%run] && t.line <= b.end {
								found.own[k][hash]++
							}
						}
					}
					if key := (blockWindow{length, hash}); blocks[key] && first >= nextFree[key] {
						found.blocks[key]++
						nextFree[key] = first + length
					}
				})
			}
		}
	}
	return found
}

// movedShare is the share of a candidate's counted tokens that lie in runs of Run tokens found in lines the change
// removed outside its own old body (own), each removed run used once; a token of a new function on a line the change
// did not add counts as found. 0 when no token is counted.
func movedShare(c candidate, run int, removed removedCode, own map[uint64]int) float64 {
	left := map[uint64]int{}
	covered := make([]bool, len(c.tokens))
	for i, h := range windowHashes(c.tokens, run) {
		have, seen := left[h]
		if !seen {
			have = removed.runs[h] - own[h]
		}
		if have > 0 {
			left[h] = have - 1
			cover(covered, i, run)
		}
	}
	counted, hit := 0, 0
	for i, t := range c.tokens {
		if !c.counted[i] {
			continue
		}
		counted++
		if covered[i] || !c.added[t.line-c.change.Start] {
			hit++
		}
	}
	if counted == 0 {
		return 0
	}
	return float64(hit) / float64(counted)
}

// blockOf is a repeat's first copy as a window of the candidate's tokens.
func (c candidate) blockOf(r repeat) blockWindow {
	var block []token
	for _, t := range c.tokens {
		if at := c.fn.start + t.start; at >= r.copies[0].start && at < r.copies[0].end {
			block = append(block, t)
		}
	}
	hashes := windowHashes(block, len(block))
	if len(hashes) == 0 {
		return blockWindow{}
	}
	return blockWindow{length: len(block), hash: hashes[0]}
}

// introducedBlock is the first two copies of the first block the change introduced into the function: one of its
// copies lies at least half on added lines, its partner at the base held no such block, and its tokens do not stand
// twice in the lines the change removed. nil when there is none.
func introducedBlock(c candidate, removed removedCode) []LineRange {
	onAdded := func(b blockSpan) bool {
		n := 0
		for line := b.startLine; line <= b.endLine; line++ {
			if c.added[line-c.change.Start] {
				n++
			}
		}
		return 2*n >= b.endLine-b.startLine+1
	}
	for k, r := range c.fn.repeats {
		if !slices.ContainsFunc(r.copies, onAdded) {
			continue
		}
		if c.base != nil && slices.ContainsFunc(c.base.repeats, func(b repeat) bool { return b.key == r.key }) {
			continue
		}
		if removed.blocks[c.blocks[k]] >= 2 {
			continue
		}
		first, second := r.copies[0], r.copies[1]
		return []LineRange{{first.startLine, first.endLine}, {second.startLine, second.endLine}}
	}
	return nil
}

// cover marks the n tokens from i as covered.
func cover(covered []bool, i, n int) {
	for j := i; j < i+n; j++ {
		covered[j] = true
	}
}
