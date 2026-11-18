package links

import (
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const radiusNote = "blast radius: the files the change did not touch that use, as a code identifier, a name a changed file " +
	"altered or removed (directly, through files that pass imports on, through files that use it, in its package and " +
	"through its header), and the importers of a changed file whose changed code has no name plimsoll reads; " +
	"a changed comment, an added declaration and a re-indented line outside Python alter nothing; a docstring is code; " +
	"a name used through an alias is not seen"

// blastRadius counts the radius of the change over the languages whose links are used, and names the changed files
// it could not count.
func blastRadius(s snapshot.Snapshot, used []snapshot.LanguageImports) (*answer.Radius, []answer.Statistic, []string) {
	r := s.Radius
	languages := map[string]bool{}
	for _, l := range used {
		languages[l.Language] = true
	}
	reached := func(u snapshot.User) bool {
		return slices.ContainsFunc(u.Languages, func(l string) bool { return languages[l] })
	}
	counts := map[snapshot.FileState]int{}
	var generated []string // the changed files left out for a reason no other "not analysed" entry names
	for _, c := range r.Changed {
		state := c.State
		if state != snapshot.NothingAltered && state != snapshot.NotCounted && !languages[c.Language] {
			state = snapshot.NotCounted // its language's links are not used; the gate's own sentence names it
		}
		counts[state]++
		if state == snapshot.NotCounted && c.Reason == snapshot.ReasonGenerated {
			generated = append(generated, cmp.Or(c.Path, c.BasePath))
		}
	}
	var notAnalysed []string
	if len(generated) > 0 {
		notAnalysed = []string{"the blast radius leaves out " + answer.Plural(len(generated), "changed code file") +
			" (generated): " + named(generated)}
	}
	if len(r.Changed) > 0 && counts[snapshot.NotCounted] == len(r.Changed) {
		radius := &answer.Radius{State: answer.RadiusNotAnalysed, Paths: []string{}}
		return radius, radiusRows(radius, counts, len(r.Changed)), notAnalysed
	}
	radius := &answer.Radius{Paths: []string{}}
	for _, u := range r.Users {
		if !reached(u) {
			continue
		}
		radius.Files++
		if u.ByName {
			radius.ByName++
		} else {
			radius.AsWhole++
		}
		switch {
		case u.Role == snapshot.Product && !u.TestsOnly:
			radius.Product++
		case u.Role.IsTest() || u.TestsOnly:
			radius.Tests++
		default:
			radius.Other++
		}
		if len(radius.Paths) < int(s.Limits.RadiusPaths.Value) {
			radius.Paths = append(radius.Paths, u.Path)
		} else {
			radius.Omitted++
		}
	}
	for _, u := range r.Touched {
		if reached(u) {
			radius.InChange++
		}
	}
	for _, u := range r.Unread {
		if reached(u) {
			radius.Unread++
		}
	}
	radius.State = answer.RadiusCounted
	if counts[snapshot.NotCounted] > 0 || radius.Unread > 0 || r.MissingNow {
		radius.State = answer.RadiusAtLeast
	}
	return radius, radiusRows(radius, counts, len(r.Changed)), notAnalysed
}

// radiusRows are the report rows of the radius: its number with its parts, and the files it lists.
func radiusRows(r *answer.Radius, counts map[snapshot.FileState]int, changed int) []answer.Statistic {
	changedNote := answer.Plural(changed, "changed code file")
	var states []string
	for _, s := range []struct {
		state snapshot.FileState
		label string
	}{{snapshot.ByName, "by name"}, {snapshot.AsAWhole, "as a whole"}, {snapshot.Deleted, "deleted"},
		{snapshot.NothingAltered, "nothing altered"}, {snapshot.NotCounted, "not counted"}} {
		if counts[s.state] > 0 {
			states = append(states, count(counts[s.state])+" "+s.label)
		}
	}
	if len(states) > 0 {
		changedNote += ": " + strings.Join(states, ", ")
	}
	if r.State == answer.RadiusNotAnalysed {
		return []answer.Statistic{{Name: "blast radius", Note: "not analysed; " + changedNote}}
	}
	files := answer.Plural(r.Files, "file")
	if r.State == answer.RadiusAtLeast {
		files = "at least " + files
	}
	parts := []string{files, count(r.ByName) + " by name, " + count(r.AsWhole) + " as a whole"}
	if r.Files > 0 {
		parts = append(parts, count(r.Product)+" product, "+count(r.Tests)+" test"+otherPart(r.Other))
	}
	parts = append(parts, changedNote)
	if r.InChange > 0 {
		parts = append(parts, answer.Agree(r.InChange, "more user is", "more users are")+" part of the change")
	}
	if r.Unread > 0 {
		parts = append(parts, answer.Agree(r.Unread, "file it reached has", "files it reached have")+" no content in this clone")
	}
	rows := []answer.Statistic{{Name: "blast radius", Note: strings.Join(parts, "; ")}}
	if r.Files > 0 {
		listed := answer.QuotedList(r.Paths)
		if r.Omitted > 0 {
			listed += " and " + count(r.Omitted) + " more"
		}
		rows = append(rows, answer.Statistic{Name: "blast radius files", Note: listed})
	}
	return rows
}

func otherPart(n int) string {
	if n == 0 {
		return ""
	}
	return ", " + count(n) + " other"
}
