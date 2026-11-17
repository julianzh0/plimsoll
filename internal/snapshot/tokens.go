package snapshot

import (
	"bytes"
	"encoding/binary"
	"hash"
	"hash/fnv"
	"regexp"
)

// token is one token of a text: a hash of its bytes, where it stands, and whether it is a name or a quoted literal.
type token struct {
	hash       uint64
	line       int // from firstLine
	start, end int // byte offsets into the text
	word       bool
}

// tokensOf splits text by a language's token pattern; comments and white space are no tokens.
func tokensOf(pattern *regexp.Regexp, text []byte, firstLine int) []token {
	ws, com := pattern.SubexpIndex("ws"), pattern.SubexpIndex("com")
	id, lit := pattern.SubexpIndex("id"), pattern.SubexpIndex("lit")
	var found []token
	line, last := firstLine, 0
	for _, m := range pattern.FindAllSubmatchIndex(text, -1) {
		line += bytes.Count(text[last:m[0]], []byte("\n"))
		last = m[0]
		switch {
		case m[2*ws] >= 0, m[2*com] >= 0:
			continue
		}
		word := m[2*id] >= 0 || m[2*lit] >= 0 && bytes.ContainsAny(text[m[0]:m[0]+1], "\"'`")
		h := fnv.New64a()
		h.Write(text[m[0]:m[1]])
		found = append(found, token{hash: h.Sum64(), line: line, start: m[0], end: m[1], word: word})
	}
	return found
}

// runHash is the hash of n tokens in a row.
func runHash(tokens []token) uint64 {
	h := fnv.New64a()
	for _, t := range tokens {
		writeUint64(h, t.hash)
	}
	return h.Sum64()
}

// writeUint64 writes v to a hash in eight little-endian bytes.
func writeUint64(h hash.Hash64, v uint64) {
	h.Write(binary.LittleEndian.AppendUint64(nil, v))
}
