package snapshot

import (
	"bytes"
	"cmp"
	"slices"
	"strings"

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
		slices.Sort(u.names)
		u.names = slices.Compact(u.names)
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
	slices.SortFunc(attributes, func(a, b span) int { return b.start - a.start })
	attach := func(units []unit) {
		for i := range units {
			for _, a := range attributes { // the last attribute first
				if a.end <= units[i].start && len(bytes.TrimSpace(content[a.end:units[i].start])) == 0 {
					units[i].start = a.start
				}
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
