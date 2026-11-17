package snapshot

import (
	"hash/fnv"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// repeat is a block of statements a function holds more than once: a hash of its statements and their tokens, and its
// copies.
type repeat struct {
	key    uint64      // hash of the block's statements and their tokens
	copies []blockSpan // in text order, none overlapping another
}

// blockSpan is where one copy of a block stands.
type blockSpan struct{ startLine, endLine, start, end int }

// statementRun is one statement of a block: the hash of its tokens, how many, and where it stands.
type statementRun struct {
	hash   uint64
	tokens int
	span   blockSpan
}

// repeatsOf finds the blocks a function repeats: from each statement of each block, the shortest run of following
// sibling statements with at least BlockStatements statements and BlockTokens tokens; such a run is repeated when the
// same statements with the same tokens stand at least twice without overlap. The statements are the named children of
// the function's Blocks nodes that are no comments (in a SwitchBlocks node, only statements and declarations); a declared
// function inside is not entered, a closure is.
func repeatsOf(pattern *regexp.Regexp, rules conventions.FunctionRules, tree parse.Tree, s shape, t table,
	from, to int, content []byte, lines lineIndex) []repeat {
	start, end := int(s.nodes[from].Start), int(s.nodes[to-1].End)
	for k := from; k < to; k++ {
		end = max(end, int(s.nodes[k].End))
	}
	toks := tokensOf(pattern, content[start:end], 0) // the function's tokens, once; a statement takes its slice of them
	tokensIn := func(a, b int) []token {
		lo, _ := slices.BinarySearchFunc(toks, a-start, func(t token, at int) int { return t.start - at })
		hi, _ := slices.BinarySearchFunc(toks, b-start, func(t token, at int) int { return t.start - at })
		return toks[lo:hi]
	}
	var sequences [][]statementRun
	for k := from + 1; k < to; k++ {
		n := s.nodes[k]
		if n.Named && t.declared.has(n) {
			k = t.skipEnd(s, k) - 1
			continue
		}
		if !n.Named || !t.blocks.has(n) {
			continue
		}
		var seq []statementRun
		for c := k + 1; c < s.endOf(k); c = s.endOf(c) {
			child := s.nodes[c]
			kind := tree.Kinds[child.Kind]
			if !child.Named || strings.Contains(kind, "comment") {
				continue
			}
			if t.switches.has(n) && !strings.HasSuffix(kind, "statement") && !strings.HasSuffix(kind, "declaration") {
				continue
			}
			start, end := int(child.Start), int(child.End)
			own := tokensIn(start, end)
			seq = append(seq, statementRun{hash: runHash(own), tokens: len(own),
				span: blockSpan{startLine: lines.line(start), endLine: lines.line(max(start, end-1)), start: start, end: end}})
		}
		sequences = append(sequences, seq)
	}
	where := map[uint64][]blockSpan{}
	var order []uint64
	for _, seq := range sequences {
		for i := range seq {
			total, j := 0, i
			for j < len(seq) && (j-i < rules.BlockStatements || total < rules.BlockTokens) {
				total += seq[j].tokens
				j++
			}
			if j-i < rules.BlockStatements || total < rules.BlockTokens {
				continue
			}
			key := blockKey(seq[i:j])
			if _, seen := where[key]; !seen {
				order = append(order, key)
			}
			where[key] = append(where[key], blockSpan{startLine: seq[i].span.startLine, endLine: seq[j-1].span.endLine,
				start: seq[i].span.start, end: seq[j-1].span.end})
		}
	}
	var found []repeat
	for _, key := range order {
		copies := slices.Clone(where[key])
		slices.SortStableFunc(copies, func(a, b blockSpan) int { return a.start - b.start })
		var disjoint []blockSpan
		for _, c := range copies {
			if len(disjoint) == 0 || c.start >= disjoint[len(disjoint)-1].end {
				disjoint = append(disjoint, c)
			}
		}
		if len(disjoint) >= 2 {
			found = append(found, repeat{key: key, copies: disjoint})
		}
	}
	slices.SortStableFunc(found, func(a, b repeat) int { return a.copies[0].start - b.copies[0].start })
	// a run pasted twice gives one repeat for every window that starts inside it: keep the first window of each run
	var kept []repeat
	for i, r := range found {
		if i > 0 && shifted(found[i-1], r) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// blockKey is the hash of a block's statements, each with its token count, in order.
func blockKey(statements []statementRun) uint64 {
	h := fnv.New64a()
	for _, st := range statements {
		writeUint64(h, st.hash)
		writeUint64(h, uint64(st.tokens))
	}
	return h.Sum64()
}

// shifted says whether each copy of b starts inside the matching copy of a: b is a later window of the same run.
func shifted(a, b repeat) bool {
	if len(a.copies) != len(b.copies) {
		return false
	}
	for i := range a.copies {
		if b.copies[i].start < a.copies[i].start || b.copies[i].start >= a.copies[i].end {
			return false
		}
	}
	return true
}
