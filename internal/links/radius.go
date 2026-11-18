package links

import (
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const radiusNote = "blast radius: untouched files that use a name the change altered or removed (directly, through " +
	"re-exports, in the same package or header), and the importers of a changed file whose altered names could not be " +
	"read; comments, added declarations and uses through an alias are not counted"

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
		return radius, radiusRows(radius, nil, counts, len(r.Changed)), notAnalysed
	}
	users := slices.DeleteFunc(slices.Clone(r.Users), func(u snapshot.User) bool { return !reached(u) })
	// Product files first, then tests, then the rest, each in path order: the reader checks product code first.
	slices.SortStableFunc(users, func(a, b snapshot.User) int { return roleOf(a) - roleOf(b) })
	radius := &answer.Radius{Files: len(users), Paths: []string{}}
	var omitted answer.Radius // the parts of the files left out of the list
	for _, u := range users {
		if u.ByName {
			radius.ByName++
		} else {
			radius.AsWhole++
		}
		addRole(radius, u)
		if len(radius.Paths) < int(s.Limits.RadiusPaths.Value) {
			radius.Paths = append(radius.Paths, u.Path)
		} else {
			radius.Omitted++
			addRole(&omitted, u)
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
	return radius, radiusRows(radius, &omitted, counts, len(r.Changed)), notAnalysed
}

// The roles a user counts under, in the order the list shows them.
const (
	productUser = iota
	testUser
	otherUser
)

func roleOf(u snapshot.User) int {
	switch {
	case u.Role == snapshot.Product && !u.TestsOnly:
		return productUser
	case u.Role.IsTest() || u.TestsOnly:
		return testUser
	}
	return otherUser
}

// addRole counts a user under its role.
func addRole(r *answer.Radius, u snapshot.User) {
	switch roleOf(u) {
	case productUser:
		r.Product++
	case testUser:
		r.Tests++
	default:
		r.Other++
	}
}

// altered says whether a changed code file altered or removed something, or could not be told: then the importers of
// the change are worth a look.
func altered(r snapshot.Radius) bool {
	return slices.ContainsFunc(r.Changed, func(c snapshot.ChangedFile) bool { return c.State != snapshot.NothingAltered })
}

// radiusRows are the report rows of the radius: its number with its parts, and the files it lists. omitted holds the
// parts of the files left out of the list.
func radiusRows(r, omitted *answer.Radius, counts map[snapshot.FileState]int, changed int) []answer.Statistic {
	if changed == 0 {
		note := "0 files; no code file changed"
		if r.State == answer.RadiusAtLeast { // a code file's content is missing now
			note = "0 files found, an incomplete count; no code file changed"
		}
		return []answer.Statistic{{Name: "blast radius", Note: note}}
	}
	changedNote := answer.Plural(changed, "changed code file")
	var states []string
	for _, s := range []struct {
		state snapshot.FileState
		label string
	}{{snapshot.ByName, "altered names"}, {snapshot.AsAWhole, "names unreadable"}, {snapshot.Deleted, "deleted"},
		{snapshot.NothingAltered, "altered no name"}, {snapshot.NotCounted, "not counted"}} {
		if counts[s.state] > 0 {
			states = append(states, count(counts[s.state])+" "+s.label)
		}
	}
	changedNote += ": " + strings.Join(states, ", ")
	if r.State == answer.RadiusNotAnalysed {
		return []answer.Statistic{{Name: "blast radius", Note: "not analysed; " + changedNote}}
	}
	files := "0 files"
	switch {
	case r.Files > 0:
		files = answer.Plural(r.Files, "file") + " (" + r.Split() + ")"
		if r.State == answer.RadiusAtLeast {
			files = "at least " + files
		}
	case r.State == answer.RadiusAtLeast:
		files = "0 files found, an incomplete count"
	}
	parts := []string{files}
	if r.AsWhole > 0 {
		parts = append(parts, answer.Agree(r.AsWhole, "of them imports", "of them import")+" a changed file whose altered names could not be read")
	}
	parts = append(parts, changedNote)
	if r.InChange > 0 {
		parts = append(parts, answer.Agree(r.InChange, "more user is a file", "more users are files")+" the change touched")
	}
	if r.Unread > 0 {
		parts = append(parts, answer.Agree(r.Unread, "file it reached has", "files it reached have")+" no content in this clone")
	}
	rows := []answer.Statistic{{Name: "blast radius", Note: strings.Join(parts, "; ")}}
	if r.Files > 0 {
		listed := answer.QuotedList(r.Paths)
		if r.Omitted > 0 {
			listed += " and " + count(r.Omitted) + " more (" + omitted.Split() + ")"
		}
		rows = append(rows, answer.Statistic{Name: "blast radius files", Note: listed})
	}
	return rows
}
