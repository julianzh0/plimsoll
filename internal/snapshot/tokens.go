package snapshot

import (
	"bytes"
	"encoding/binary"
	"hash"
	"hash/fnv"
	"iter"
	"slices"

	"github.com/julianzh0/plimsoll/internal/conventions"
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
func tokensOf(pattern conventions.Tokens, text []byte, firstLine int) []token {
	var found []token
	for t := range eachToken(pattern, text, firstLine) {
		found = append(found, t)
	}
	return found
}

// scanFrame is an open string or hole of the scanner's stack.
type scanFrame struct {
	form  stringForm // the string's form; for a hole, that of the string it stands in
	hole  bool       // a hole read as code
	spec  bool       // a Python hole's conversion and format text: a string ending at the hole's closer
	piece int        // a string's: where its current literal piece starts
	depth int        // a hole's: the brackets open inside it
}

// eachToken yields the tokens of text one by one, holding no more than one match at a time: a whole file is read as a
// stream, never kept as a list of tokens. One pass, left to right, with an explicit stack: a string of the forms the
// language's scanner reads is handed out as literal pieces, from its opening delimiter to the first hole, between
// holes and from the last hole to its closing delimiter, and the code in a hole as code, until the hole's closer at
// its own bracket level. A string still open at the end of the text ends there.
func eachToken(tokens conventions.Tokens, text []byte, firstLine int) iter.Seq[token] {
	pattern := tokens.Pattern
	ws, com := pattern.SubexpIndex("ws"), pattern.SubexpIndex("com")
	id, lit := pattern.SubexpIndex("id"), pattern.SubexpIndex("lit")
	return func(yield func(token) bool) {
		line, last := firstLine, 0
		emit := func(start, end int, word, identifier bool) bool {
			line += bytes.Count(text[last:start], []byte("\n"))
			last = start
			h := fnv.New64a()
			_, _ = h.Write(text[start:end]) // a hash's Write never fails
			return yield(token{hash: h.Sum64(), line: line, start: start, end: end, word: word, identifier: identifier})
		}
		var stack []scanFrame
		for at := 0; at < len(text); {
			if n := len(stack); n > 0 && !stack[n-1].hole {
				var ok bool
				if stack, at, ok = scanString(text, stack, at, emit); !ok {
					return
				}
				continue
			}
			if n := len(stack); n > 0 {
				h := &stack[n-1]
				if h.depth == 0 && bytes.HasPrefix(text[at:], []byte(h.form.holeClose)) {
					stack = stack[:n-1]
					stack[n-2].piece = at // the closer starts the next literal piece
					at += len(h.form.holeClose)
					continue
				}
				if h.depth == 0 && h.form.pythonHoles {
					if length := pythonSpecAt(text, at); length > 0 {
						spec := h.form // the f-string's: a format text ends at a line end where the string does
						spec.closer, spec.escape = "}", ""
						stack[n-1] = scanFrame{form: spec, spec: true, piece: at}
						at += length
						continue
					}
				}
			}
			if form, length, found := stringAt(tokens.Strings, text, at); found {
				stack = append(stack, scanFrame{form: form, piece: at})
				at += length
				continue
			}
			m := pattern.FindSubmatchIndex(text[at:])
			if m == nil || m[1] == 0 {
				return
			}
			start, end := at+m[0], at+m[1]
			at = end
			if m[2*ws] >= 0 || m[2*com] >= 0 {
				continue
			}
			if n := len(stack); n > 0 && end-start == 1 {
				switch text[start] {
				case '(', '[', '{':
					stack[n-1].depth++
				case ')', ']', '}':
					stack[n-1].depth = max(stack[n-1].depth-1, 0)
				}
			}
			// a literal is a string unless it is a number: a prefixed string (b"…", @"…") is one too
			isString := m[2*lit] >= 0 && (text[start] < '0' || text[start] > '9') && text[start] != '.'
			if !emit(start, end, m[2*id] >= 0 || isString, m[2*id] >= 0) {
				return
			}
		}
	}
}

// scanString reads the string on top of the stack from at: to its closer, which pops it, to a hole's opener, which
// pushes the hole, or to a $name, handed out as an identifier. It returns the stack and where reading goes on, and
// false when the reader stopped.
func scanString(text []byte, stack []scanFrame, at int, emit func(start, end int, word, identifier bool) bool) ([]scanFrame, int, bool) {
	s := &stack[len(stack)-1]
	f := s.form
	for i := at; i < len(text); {
		switch rest := text[i:]; {
		case f.singleLine && text[i] == '\n':
			ok := s.piece >= i || emit(s.piece, i, true, false)
			stack = stack[:len(stack)-1]
			if s.spec { // the f-string a format text stands in ends with it
				stack = stack[:len(stack)-1]
			}
			return stack, i, ok
		case f.quoteTwice && bytes.HasPrefix(rest, []byte(`""`)):
			i += 2
		case bytes.HasPrefix(rest, []byte(f.closer)):
			if s.spec { // a format text ends with its hole: the string it stands in goes on from its piece
				piece := s.piece
				stack = stack[:len(stack)-1]
				stack[len(stack)-1].piece = piece
				return stack, i + len(f.closer), true
			}
			return stack[:len(stack)-1], i + len(f.closer), emit(s.piece, i+len(f.closer), true, false)
		case f.holeOpen != "" && bytes.HasPrefix(rest, []byte(f.holeOpen)) && (!f.doubled || !bytes.HasPrefix(rest, []byte("{{"))):
			if f.holeOpen[0] == '{' && len(f.holeOpen) > 1 { // in a run of more braces, the last ones open the hole
				i += run(text, i, '{') - len(f.holeOpen)
			}
			end := i + len(f.holeOpen)
			return append(stack, scanFrame{form: f, hole: true}), end, emit(s.piece, end, true, false)
		case f.doubled && (bytes.HasPrefix(rest, []byte("{{")) || bytes.HasPrefix(rest, []byte("}}"))):
			i += 2
		case f.escape != "" && bytes.HasPrefix(rest, []byte(f.escape)):
			if f.pythonHoles && i+1 < len(text) && (text[i+1] == '{' || text[i+1] == '}') {
				i++ // Python escapes no brace: the brace keeps its meaning
				continue
			}
			i += len(f.escape) + 1
		case f.dollarName && text[i] == '$' && identifierEnd(text, i+1) > i+1:
			end := identifierEnd(text, i+1)
			if !emit(s.piece, i+1, true, false) || !emit(i+1, end, true, true) {
				return stack, i, false
			}
			s.piece, i = end, end
		default:
			i++
		}
	}
	return stack[:len(stack)-1], len(text), s.piece >= len(text) || emit(s.piece, len(text), true, false)
}

// pythonSpecAt is the length of a Python hole's conversion (!r, !s or !a, then : or }) or format text (:) that starts
// at, at the hole's own level, or 0: !=, and every other !, is code.
func pythonSpecAt(text []byte, at int) int {
	switch {
	case text[at] == ':':
		return 1
	case text[at] == '!' && at+2 < len(text) && bytes.IndexByte([]byte("rsa"), text[at+1]) >= 0 && (text[at+2] == ':' || text[at+2] == '}'):
		return 2
	}
	return 0
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
