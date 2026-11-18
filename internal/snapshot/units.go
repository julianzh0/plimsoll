package snapshot

import (
	"bytes"
	"cmp"
	"hash/fnv"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// unit is one comparison unit of a changed file (US-63): a declaration compared on its own, the names it declares
// directly, and the names of the containers it stands in. A container is a unit too: its own head, without the units
// inside it, which are compared on their own.
type unit struct {
	start, end int // byte offsets, end excluded
	names      []string
	context    string // the names of its containers, outermost first, each followed by a newline
	inner      []span // for a container: the units inside it, left out of its fingerprint
}

// decodeUnits turns the unit captures of a reading into the file's units, sorted by where they start, and its pure
// imports. A declaration inside another declaration is part of the outer one; attributes right before a unit are
// part of it.
func decodeUnits(result parse.Result, content []byte) ([]unit, []span) {
	byStart := map[[2]int]*unit{}
	var imports, reexports, attributes []span
	var containers []unit
	for _, match := range result.Matches {
		var u *unit
		var names []string
		var c unit
		bodyEnd := 0
		for _, capture := range match {
			switch capture.Name {
			case "unit":
				key := [2]int{capture.Start, capture.End}
				if byStart[key] == nil {
					byStart[key] = &unit{start: capture.Start, end: capture.End}
				}
				u = byStart[key]
			case "unit.name":
				names = append(names, string(content[capture.Start:capture.End]))
			case "unit.body":
				bodyEnd = capture.End
			case "unit.import":
				imports = append(imports, span{start: capture.Start, end: capture.End - 1})
			case "unit.reexport":
				reexports = append(reexports, span{start: capture.Start, end: capture.End - 1})
			case "unit.attribute":
				attributes = append(attributes, span{start: capture.Start, end: capture.End})
			case "container":
				c.start, c.end = capture.Start, capture.End
			case "container.name":
				c.names = []string{string(content[capture.Start:capture.End])}
			}
		}
		if u != nil {
			u.names = append(u.names, names...)
			u.end = max(u.end, bodyEnd)
		}
		if c.end > c.start {
			containers = append(containers, c)
		}
	}
	units := make([]unit, 0, len(byStart))
	for _, u := range byStart {
		u.names = sortedNames(u.names)
		units = append(units, *u)
	}
	byPlace := func(a, b unit) int { return cmp.Or(a.start-b.start, b.end-a.end) }
	slices.SortFunc(units, byPlace)
	outer := units[:0]
	for _, u := range units {
		if len(outer) > 0 && u.start < outer[len(outer)-1].end {
			continue // inside the unit before it
		}
		outer = append(outer, u)
	}
	slices.SortFunc(attributes, func(a, b span) int { return a.start - b.start })
	attach := func(units []unit) {
		for i := range units { // units sorted by start: the attributes right before each, the last one first
			before, _ := slices.BinarySearchFunc(attributes, units[i].start, func(a span, at int) int { return a.end - at })
			for j := before - 1; j >= 0 && attributes[j].end <= units[i].start; j-- {
				if len(bytes.TrimSpace(content[attributes[j].end:units[i].start])) > 0 {
					break
				}
				units[i].start = attributes[j].start
			}
		}
	}
	attach(outer)
	attach(containers)
	slices.SortFunc(containers, byPlace)
	for i, c := range containers {
		for _, u := range outer {
			if c.start <= u.start && u.end <= c.end {
				containers[i].inner = append(containers[i].inner, span{start: u.start, end: u.end - 1})
			}
		}
	}
	all := slices.Concat(outer, containers)
	for i, u := range all {
		var context strings.Builder
		for _, c := range containers {
			if c.start <= u.start && u.end <= c.end && (c.start != u.start || c.end != u.end) {
				context.WriteString(c.names[0] + "\n")
			}
		}
		all[i].context = context.String()
	}
	slices.SortFunc(all, byPlace)
	imports = slices.DeleteFunc(imports, func(s span) bool { return slices.Contains(reexports, s) })
	return all, imports
}

// prints are the fingerprints of a reading's code, made once per content: of every code token, of those outside pure
// imports, of those outside units and pure imports (the remainder), and of each unit.
type prints struct {
	whole, code, remainder uint64
	units                  []uint64
}

// printPair are a content's prints with its pure imports left out, and, for a file whose imports are what its package
// offers, with none left out.
type printPair struct{ pure, all prints }

// pick gives the prints that apply to a file: all for a file whose imports are its package's surface.
func (p printPair) pick(surface bool) prints {
	if surface {
		return p.all
	}
	return p.pure
}

// printsOf tokenizes a content once and gives its prints; withAll adds those with no import left out.
func printsOf(language conventions.Language, content []byte, units []unit, imports []span, withAll bool) printPair {
	toks := tokensOf(language.Tokens, content, 1)
	starts := lineStarts(content)
	hash := func(lo, hi int, skip []span, context string) uint64 {
		return hashTokens(language, content, starts, toks[lo:hi], skip, context)
	}
	n := len(toks)
	unitSpans := make([]span, 0, len(units))
	for _, u := range units {
		unitSpans = append(unitSpans, span{start: u.start, end: u.end - 1})
	}
	whole := hash(0, n, nil, "")
	var p printPair
	p.pure = prints{whole: whole, code: hash(0, n, imports, ""), remainder: hash(0, n, slices.Concat(unitSpans, imports), "")}
	if withAll {
		p.all = prints{whole: whole, code: whole, remainder: hash(0, n, unitSpans, "")}
	}
	for _, u := range units {
		lo, hi := tokensIn(toks, u.start, u.end)
		if u.inner == nil {
			h := hash(lo, hi, nil, u.context)
			p.pure.units, p.all.units = append(p.pure.units, h), append(p.all.units, h)
			continue
		}
		p.pure.units = append(p.pure.units, hash(lo, hi, slices.Concat(u.inner, imports), u.context))
		if withAll {
			p.all.units = append(p.all.units, hash(lo, hi, u.inner, u.context))
		}
	}
	return p
}

// lineStarts are the offsets where the lines of a content start, the first line's at index 1.
func lineStarts(content []byte) []int {
	starts := []int{0, 0}
	for i, c := range content {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// hashTokens hashes, after a context, the tokens in no span of skip, with the layout the language reads as code: a
// line break before a token on a new line, and the line's indentation.
func hashTokens(language conventions.Language, content []byte, starts []int, toks []token, skip []span, context string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(context)) // a hash's Write never fails
	skipped := make([]bool, len(toks))
	for _, sp := range skip { // span ends are included
		lo, hi := tokensIn(toks, sp.start, sp.end+1)
		for i := lo; i < hi; i++ {
			skipped[i] = true
		}
	}
	line := 0
	for i, t := range toks {
		if skipped[i] {
			continue
		}
		if t.line != line {
			if language.LayoutCode.LineBreaks && line > 0 {
				_, _ = h.Write([]byte{'\n'})
			}
			if language.LayoutCode.Indentation {
				_, _ = h.Write(content[starts[t.line]:t.start])
			}
			line = t.line
		}
		writeUint64(h, t.hash)
	}
	return h.Sum64()
}
