package snapshot

import (
	"bytes"
	"slices"
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/parse"
)

// decode turns the captures of an entry's query into the reading of one file. The capture names are the contract
// with the queries, listed at conventions.Language.Query.
func decode(language conventions.Language, result parse.Result, content []byte) reading {
	var r reading
	var targets, computed, names, typeBlocks, texts, blanks []parse.Capture
	var modules, tests []span
	passes := map[parse.Span]bool{}
	type found struct {
		statement statement
		at        int           // where its target starts
		within    parse.Capture // the @import capture, which holds the statement's @name captures
	}
	var statements []found
	for _, match := range result.Matches {
		var specs []parse.Capture
		var within, module, moduleName, test, testEnd parse.Capture
		typeOnly, everywhere, declaration := false, false, false
		for _, c := range match {
			switch c.Name {
			case "spec", "spec.mod":
				specs = append(specs, c)
				declaration = declaration || c.Name == "spec.mod"
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
			case "module":
				module = c
			case "module.name":
				moduleName = c
			case "test":
				test = c
			case "test.end":
				testEnd = c
			case "text":
				texts = append(texts, c)
			case "blank":
				blanks = append(blanks, c)
			}
		}
		if moduleName.End > 0 {
			modules = append(modules, span{start: module.Start, end: module.End - 1, name: string(content[moduleName.Start:moduleName.End])})
		}
		if testEnd.End > test.Start {
			tests = append(tests, span{start: test.Start, end: testEnd.End - 1})
		}
		if len(specs) == 0 {
			continue
		}
		targets = append(targets, specs[0])
		first, last := specs[0], specs[len(specs)-1]
		written := string(blanked(content[first.Start:last.End], first.Start, blanks))
		if len(specs) > 1 || declaration {
			written = strings.Map(func(c rune) rune {
				if unicode.IsSpace(c) {
					return -1
				}
				return c
			}, written)
		}
		if declaration {
			written = "mod " + written
		}
		st := statement{spec: unquoted(written), line: first.Line, typeOnly: typeOnly, everywhere: everywhere}
		statements = append(statements, found{st, first.Start, within})
	}
	for _, f := range statements {
		st := f.statement
		for _, n := range names {
			if f.within.Start <= n.Start && n.End <= f.within.End {
				st.names = append(st.names, string(content[n.Start:n.End]))
			}
		}
		st.typeOnly = st.typeOnly || slices.ContainsFunc(typeBlocks, func(b parse.Capture) bool { return b.Start <= f.at && f.at < b.End })
		st.within = modulesAt(modules, f.at)
		r.statements = append(r.statements, expanded(language.Imports, st)...)
	}
	r.statements = append(r.statements, readTexts(language.Imports, content, texts, blanks, modules)...)
	for _, c := range computed {
		if !slices.ContainsFunc(targets, func(t parse.Capture) bool { return c.Start <= t.Start && t.End <= c.End }) {
			r.computed++
		}
	}
	r.passesOn = len(passes) > 0 && !slices.ContainsFunc(result.Top, func(top parse.Span) bool { return !passes[top] })
	r.testLines = spanLines(content, tests)
	return r
}

// expanded is one statement per target: a grouped target ("a::{b, c}") names each of its paths.
func expanded(imports conventions.Imports, st statement) []statement {
	if !imports.Grouped {
		return []statement{st}
	}
	var found []statement
	for _, target := range grouped(st.spec) {
		st.spec = target
		found = append(found, st)
	}
	return found
}

// readTexts reads the stretches the grammar keeps as tokens (macro bodies) by the entry's text forms, the rest of the
// file and every comment and string blanked, so offsets and lines stay the file's.
func readTexts(imports conventions.Imports, content []byte, texts, blanks []parse.Capture, modules []span) []statement {
	if len(texts) == 0 || len(imports.Statements) == 0 {
		return nil
	}
	b := bytes.Map(func(c rune) rune {
		if c == '\n' {
			return c
		}
		return ' '
	}, content)
	for _, t := range texts {
		copy(b[t.Start:t.End], blanked(content[t.Start:t.End], t.Start, blanks))
	}
	found := scan(imports, b).statements
	for i, st := range found {
		found[i].within = modulesAt(modules, lineStart(content, st.line))
	}
	return found
}

// blanked is a stretch of content that starts at offset, with every blank capture's bytes as spaces, line ends kept.
func blanked(stretch []byte, offset int, blanks []parse.Capture) []byte {
	b := slices.Clone(stretch)
	for _, bl := range blanks {
		for i := max(bl.Start, offset); i < min(bl.End, offset+len(b)); i++ {
			if b[i-offset] != '\n' {
				b[i-offset] = ' '
			}
		}
	}
	return b
}

// spanLines counts the lines the spans touch, each line once; a span that starts inside the one before it is part of
// that one.
func spanLines(content []byte, spans []span) int {
	slices.SortFunc(spans, func(a, b span) int { return a.start - b.start })
	counted, lastCounted, until := 0, 0, -1
	for _, s := range spans {
		if s.start <= until {
			continue
		}
		until = s.end
		first := max(1+bytes.Count(content[:s.start], []byte("\n")), lastCounted+1)
		last := 1 + bytes.Count(content[:s.end], []byte("\n"))
		if last >= first {
			counted, lastCounted = counted+last-first+1, last
		}
	}
	return counted
}

// lineStart is the offset where a line, from 1, starts.
func lineStart(content []byte, line int) int {
	at := 0
	for l := 1; l < line; l++ {
		next := bytes.IndexByte(content[at:], '\n')
		if next < 0 {
			return at
		}
		at += next + 1
	}
	return at
}

// unquoted removes one pair of quotes or backticks around a target.
func unquoted(s string) string {
	if len(s) >= 2 && strings.ContainsRune("\"'`", rune(s[0])) && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
