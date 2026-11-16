package snapshot

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/parse"
)

// decode turns the captures of an entry's query into the reading of one file. The capture names are the contract
// with the queries, listed at conventions.Language.Query.
func decode(result parse.Result, content []byte) reading {
	var r reading
	byMatch := map[uint32][]parse.Capture{}
	var order []uint32
	for _, c := range result.Captures {
		if _, seen := byMatch[c.Match]; !seen {
			order = append(order, c.Match)
		}
		byMatch[c.Match] = append(byMatch[c.Match], c)
	}
	for _, id := range order {
		var first, last *parse.Capture
		dynamic := false
		for i, c := range byMatch[id] {
			switch c.Name {
			case "spec":
				if first == nil {
					first = &byMatch[id][i]
				}
				last = &byMatch[id][i]
			case "dynamic":
				dynamic = true
			}
		}
		if dynamic {
			r.dynamic++
		}
		if first == nil {
			continue
		}
		written := string(content[first.Start:last.End])
		if first != last {
			written = strings.Map(func(c rune) rune {
				if unicode.IsSpace(c) {
					return -1
				}
				return c
			}, written)
		}
		r.statements = append(r.statements, statement{spec: unquoted(written), line: lineAt(content, first.Start)})
	}
	return r
}

// unquoted removes one pair of quotes or backticks around a target.
func unquoted(s string) string {
	if len(s) >= 2 && strings.ContainsRune("\"'`", rune(s[0])) && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// lineAt is the line, from 1, of an offset.
func lineAt(content []byte, at int) int { return 1 + bytes.Count(content[:at], []byte("\n")) }
