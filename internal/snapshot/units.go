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

// unit is one comparison unit of a changed file: a declaration compared on its own, the names it declares
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
	units, containers, attributes, imports := captured(result, content)
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
	attachAttributes(outer, attributes, content)
	attachAttributes(containers, attributes, content)
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
	return all, imports
}

// captured reads the unit captures of a reading: its units (one per captured span, with the names and the furthest
// body end of every match on it), its containers, its attributes and its pure imports (re-exports left out).
func captured(result parse.Result, content []byte) (units, containers []unit, attributes, imports []span) {
	byStart := map[[2]int]*unit{}
	var reexports []span
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
	units = make([]unit, 0, len(byStart))
	for _, u := range byStart {
		u.names = sortedNames(u.names)
		units = append(units, *u)
	}
	imports = slices.DeleteFunc(imports, func(s span) bool { return slices.Contains(reexports, s) })
	return units, containers, attributes, imports
}

// attachAttributes moves the start of each unit to the attributes right before it: those separated
// from it by white space only, the last one first. The attributes are sorted by start.
func attachAttributes(units []unit, attributes []span, content []byte) {
	for i := range units {
		// the first attribute ending after the unit's start: those before it end at or before the start
		before, _ := slices.BinarySearchFunc(attributes, units[i].start+1, func(a span, at int) int { return a.end - at })
		for j := before - 1; j >= 0 && attributes[j].end <= units[i].start; j-- {
			if len(bytes.TrimSpace(content[attributes[j].end:units[i].start])) > 0 {
				break
			}
			units[i].start = attributes[j].start
		}
	}
}

// prints are the fingerprints of a reading's code, made once per content: of every code token, of those outside pure
// imports, of those outside units and pure imports (the remainder), and of each unit.
type prints struct {
	whole, code, remainder uint64
	units                  []uint64
}

// printsOf tokenizes a content once and gives its prints, with its pure imports left out.
func printsOf(language conventions.Language, content []byte, units []unit, imports []span) prints {
	toks := tokensOf(language.Tokens, content, 1)
	newlines := linesOf(content)
	hash := func(lo, hi int, skip []span, context string) uint64 {
		return hashTokens(language, content, newlines, toks[lo:hi], skip, context)
	}
	n := len(toks)
	unitSpans := make([]span, 0, len(units))
	for _, u := range units {
		unitSpans = append(unitSpans, span{start: u.start, end: u.end - 1})
	}
	p := prints{whole: hash(0, n, nil, ""), code: hash(0, n, imports, ""), remainder: hash(0, n, slices.Concat(unitSpans, imports), "")}
	for _, u := range units {
		lo, hi := tokensIn(toks, u.start, u.end)
		skip := imports // a leaf unit skips only the pure imports; a container also its inner units
		if u.inner != nil {
			skip = slices.Concat(u.inner, imports)
		}
		p.units = append(p.units, hash(lo, hi, skip, u.context))
	}
	return p
}

// hashTokens hashes, after a context, the tokens in no span of skip, with the layout the language reads as code: a
// line break before a token on a new line, and the line's indentation.
func hashTokens(language conventions.Language, content []byte, newlines lineIndex, toks []token, skip []span, context string) uint64 {
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
				_, _ = h.Write(content[lineOffset(newlines, len(content), t.line):t.start])
			}
			line = t.line
		}
		writeUint64(h, t.hash)
	}
	return h.Sum64()
}
