package snapshot

import (
	"bytes"
	"encoding/binary"
	"hash"
	"hash/fnv"
	"iter"
	"regexp"
	"slices"
)

// token is one token of a text: a hash of its bytes, where it stands, and whether it is a name or a quoted literal.
type token struct {
	hash       uint64
	line       int  // from firstLine
	start, end int  // byte offsets into the text
	word       bool // a name or a quoted literal
	identifier bool // a name
}

// tokensOf splits text by a language's token pattern; comments and white space are no tokens.
func tokensOf(pattern *regexp.Regexp, text []byte, firstLine int) []token {
	var found []token
	for t := range eachToken(pattern, text, firstLine) {
		found = append(found, t)
	}
	return found
}

// eachToken yields the tokens of text one by one, holding no more than one match at a time: a whole file is read as a
// stream, never kept as a list of tokens.
func eachToken(pattern *regexp.Regexp, text []byte, firstLine int) iter.Seq[token] {
	ws, com := pattern.SubexpIndex("ws"), pattern.SubexpIndex("com")
	id, lit := pattern.SubexpIndex("id"), pattern.SubexpIndex("lit")
	return func(yield func(token) bool) {
		line, last := firstLine, 0
		for at := 0; at < len(text); {
			m := pattern.FindSubmatchIndex(text[at:])
			if m == nil || m[1] == 0 {
				return
			}
			start, end := at+m[0], at+m[1]
			line += bytes.Count(text[last:start], []byte("\n"))
			last, at = start, end
			if m[2*ws] >= 0 || m[2*com] >= 0 {
				continue
			}
			word := m[2*id] >= 0 || m[2*lit] >= 0 && bytes.ContainsAny(text[start:start+1], "\"'`")
			h := fnv.New64a()
			_, _ = h.Write(text[start:end]) // a hash's Write never fails
			if !yield(token{hash: h.Sum64(), line: line, start: start, end: end, word: word, identifier: m[2*id] >= 0}) {
				return
			}
		}
	}
}

// rollBase is the base of the rolling hash of a window of token hashes: odd, so no hash is lost to it.
const rollBase = 1099511628211

// windows rolls hashes of windows of several lengths over a stream of token hashes: the hash of a window is the sum
// of its tokens' hashes times rollBase to the power of their distance from its end, so a window's hash is the same
// wherever it stands. It holds one ring of the longest length.
type windows struct {
	lengths []int
	ring    []uint64
	hashes  []uint64 // per length: the hash of the window ending at the last token
	powers  []uint64 // per length: rollBase to that length
	n       int
}

func newWindows(lengths ...int) *windows {
	w := &windows{lengths: lengths, hashes: make([]uint64, len(lengths)), powers: make([]uint64, len(lengths))}
	longest := 1
	for i, l := range lengths {
		longest = max(longest, l)
		w.powers[i] = 1
		for range l {
			w.powers[i] *= rollBase
		}
	}
	w.ring = make([]uint64, longest)
	return w
}

// push adds the next token's hash and calls visit for each length with a full window, with the window's hash and the
// index of its first token.
func (w *windows) push(h uint64, visit func(length int, hash uint64, first int)) {
	for i, l := range w.lengths {
		w.hashes[i] = w.hashes[i]*rollBase + h
		if w.n >= l {
			w.hashes[i] -= w.ring[(w.n-l)%len(w.ring)] * w.powers[i]
		}
	}
	w.ring[w.n%len(w.ring)] = h
	w.n++
	for i, l := range w.lengths {
		if w.n >= l {
			visit(l, w.hashes[i], w.n-l)
		}
	}
}

// windowHashes are the hashes of every window of n tokens in a row, by the window's first token.
func windowHashes(tokens []token, n int) []uint64 {
	if len(tokens) < n {
		return nil
	}
	found := make([]uint64, len(tokens)-n+1)
	w := newWindows(n)
	for _, t := range tokens {
		w.push(t.hash, func(_ int, hash uint64, first int) { found[first] = hash })
	}
	return found
}

// tokenSpan is the tokens of a range of a content, from its byte base, with prefix hashes: the hash of any run of them
// in constant time, equal to the hash windows gives that run.
type tokenSpan struct {
	toks   []token
	base   int
	prefix []uint64 // prefix[i]: the rolling hash of toks[:i]
	power  []uint64 // power[i]: rollBase to i
}

func newTokenSpan(toks []token, base int) tokenSpan {
	ts := tokenSpan{toks: toks, base: base, prefix: make([]uint64, len(toks)+1), power: make([]uint64, len(toks)+1)}
	ts.power[0] = 1
	for i, t := range toks {
		ts.prefix[i+1] = ts.prefix[i]*rollBase + t.hash
		ts.power[i+1] = ts.power[i] * rollBase
	}
	return ts
}

// in is the run of tokens that start inside the content's bytes a to b.
func (ts tokenSpan) in(a, b int) (lo, hi int) { return tokensIn(ts.toks, a-ts.base, b-ts.base) }

// tokensIn is the run of tokens, sorted by start, that start at a or later and before b.
func tokensIn(toks []token, a, b int) (lo, hi int) {
	lo, _ = slices.BinarySearchFunc(toks, a, func(t token, at int) int { return t.start - at })
	hi, _ = slices.BinarySearchFunc(toks, b, func(t token, at int) int { return t.start - at })
	return lo, hi
}

// hash is the rolling hash of the tokens lo to hi.
func (ts tokenSpan) hash(lo, hi int) uint64 { return ts.prefix[hi] - ts.prefix[lo]*ts.power[hi-lo] }

// writeUint64 writes v to a hash in eight little-endian bytes.
func writeUint64(h hash.Hash64, v uint64) {
	_, _ = h.Write(binary.LittleEndian.AppendUint64(nil, v)) // a hash's Write never fails
}
