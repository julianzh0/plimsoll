package snapshot

import (
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/parse"
)

// unit is one comparison unit of a changed file (US-63): a declaration compared on its own, the names it declares
// directly, and the names of the containers it stands in.
type unit struct {
	start, end int // byte offsets, end excluded
	names      []string
	context    string // the names of its containers, outermost first, each followed by a newline
}

// decodeUnits turns the unit captures of a reading into the file's units, outermost only, sorted by where they start,
// and its pure imports.
func decodeUnits(result parse.Result, content []byte) ([]unit, []span) {
	byStart := map[[2]int]*unit{}
	var imports, reexports []span
	type container struct {
		start, end int
		name       string
	}
	var containers []container
	for _, match := range result.Matches {
		var u *unit
		var names []string
		var c container
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
			case "container":
				c.start, c.end = capture.Start, capture.End
			case "container.name":
				c.name = string(content[capture.Start:capture.End])
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
	slices.SortFunc(units, func(a, b unit) int { return cmp.Or(a.start-b.start, b.end-a.end) })
	outer := units[:0]
	for _, u := range units {
		if len(outer) > 0 && u.start < outer[len(outer)-1].end {
			continue // inside the unit before it
		}
		outer = append(outer, u)
	}
	slices.SortFunc(containers, func(a, b container) int { return a.start - b.start })
	for i, u := range outer {
		var context strings.Builder
		for _, c := range containers {
			if c.start <= u.start && u.end <= c.end {
				context.WriteString(c.name + "\n")
			}
		}
		outer[i].context = context.String()
	}
	imports = slices.DeleteFunc(imports, func(s span) bool { return slices.Contains(reexports, s) })
	return outer, imports
}
