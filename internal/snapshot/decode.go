package snapshot

import (
	"slices"
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/parse"
)

// decode turns the captures of an entry's query into the reading of one file. The capture names are the contract
// with the queries, listed at conventions.Language.Query.
func decode(result parse.Result, content []byte) reading {
	var r reading
	var targets, computed []parse.Capture
	passes := map[parse.Span]bool{}
	for _, match := range result.Matches {
		var specs []parse.Capture
		typeOnly := false
		for _, c := range match {
			switch c.Name {
			case "spec":
				specs = append(specs, c)
			case "type":
				typeOnly = true
			case "computed":
				computed = append(computed, c)
			case "pass":
				passes[parse.Span{Start: c.Start, End: c.End}] = true
			case "dynamic":
				r.dynamic++
			}
		}
		if len(specs) == 0 {
			continue
		}
		targets = append(targets, specs[0])
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
		r.statements = append(r.statements, statement{spec: unquoted(written), line: first.Line, typeOnly: typeOnly})
	}
	for _, c := range computed {
		if !slices.ContainsFunc(targets, func(t parse.Capture) bool { return c.Start <= t.Start && t.End <= c.End }) {
			r.computed++
		}
	}
	r.passesOn = len(passes) > 0 && !slices.ContainsFunc(result.Top, func(top parse.Span) bool { return !passes[top] })
	return r
}

// unquoted removes one pair of quotes or backticks around a target.
func unquoted(s string) string {
	if len(s) >= 2 && strings.ContainsRune("\"'`", rune(s[0])) && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
