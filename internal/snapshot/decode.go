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
	var targets, computed, names, typeBlocks []parse.Capture
	passes := map[parse.Span]bool{}
	type found struct {
		statement statement
		at        int           // where its target starts
		within    parse.Capture // the @import capture, which holds the statement's @name captures
	}
	var statements []found
	for _, match := range result.Matches {
		var specs []parse.Capture
		var within parse.Capture
		typeOnly, everywhere := false, false
		for _, c := range match {
			switch c.Name {
			case "spec":
				specs = append(specs, c)
			case "import":
				within = c
			case "name":
				names = append(names, c)
			case "type":
				typeOnly = true
			case "everywhere":
				everywhere = true
			case "type_block":
				typeBlocks = append(typeBlocks, c)
			case "computed":
				computed = append(computed, c)
			case "pass":
				passes[parse.Span{Start: c.Start, End: c.End}] = true
			case "prefix":
				if prefix := string(content[c.Start:c.End]); !slices.Contains(r.prefixes, prefix) {
					r.prefixes = append(r.prefixes, prefix)
				}
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
		statements = append(statements, found{statement{spec: unquoted(written), line: first.Line, typeOnly: typeOnly, everywhere: everywhere}, first.Start, within})
	}
	for _, f := range statements {
		st := f.statement
		for _, n := range names {
			if f.within.Start <= n.Start && n.End <= f.within.End {
				st.names = append(st.names, string(content[n.Start:n.End]))
			}
		}
		st.typeOnly = st.typeOnly || slices.ContainsFunc(typeBlocks, func(b parse.Capture) bool { return b.Start <= f.at && f.at < b.End })
		r.statements = append(r.statements, st)
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
