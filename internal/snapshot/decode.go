package snapshot

import (
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/parse"
)

// decode turns the captures of an entry's query into the reading of one file. The capture names are the contract
// with the queries, listed at conventions.Language.Query.
func decode(result parse.Result, content []byte) reading {
	var r reading
	for _, match := range result.Matches {
		var specs []parse.Capture
		for _, c := range match {
			switch c.Name {
			case "spec":
				specs = append(specs, c)
			case "dynamic":
				r.dynamic++
			}
		}
		if len(specs) == 0 {
			continue
		}
		first, last := specs[0], specs[len(specs)-1]
		written := string(content[first.Start:last.End])
		if len(specs) > 1 {
			written = strings.Map(func(c rune) rune {
				if unicode.IsSpace(c) {
					return -1
				}
				return c
			}, written)
		}
		r.statements = append(r.statements, statement{spec: unquoted(written), line: first.Line})
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
