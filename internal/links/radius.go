package links

import (
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const radiusNote = "blast radius: untouched files that use a name the change altered or removed (directly, through " +
	"re-exports, in the same package or header), and the importers of a changed file counted as a whole (its changed code " +
	"has no name plimsoll reads, its code outside declarations changed, or it holds a syntax error); comments, added " +
	"declarations and uses through an alias are not counted"

// blastRadius counts the radius of the change over the languages whose links are used: a user counts when its own
// language's links are used and a changed file of such a language reached it. It names the changed files left out for
// a reason no other "not analysed" entry names.
// A user the gate drops for its own language although a used language reached it makes the number a lower bound.
func blastRadius(s snapshot.Snapshot, used []snapshot.LanguageImports, now graph) (*answer.Radius, []answer.Statistic, []string) {
	r := s.Radius
	languages := map[string]bool{}
	for _, l := range used {
		languages[l.Language] = true
	}
	fromUsed := func(u snapshot.User) bool {
		return slices.ContainsFunc(u.Languages, func(l string) bool { return languages[l] })
	}
	reached := func(u snapshot.User) bool { return now.analysed(u.Path) && fromUsed(u) }
	radius := &answer.Radius{Paths: []string{}, Folders: []answer.FolderCount{}}
	leftOut := map[string][]string{} // reason -> paths
	for _, c := range r.Changed {
		state := c.State
		if state != snapshot.NothingAltered && state != snapshot.NotCounted && !languages[c.Language] {
			state = snapshot.NotCounted // its language's links are not used; the gate's own sentence names it
		}
		addState(&radius.Changed, state)
		if state == snapshot.NotCounted && (c.Reason == snapshot.ReasonGenerated || c.Reason == snapshot.ReasonIgnored) {
			leftOut[c.Reason] = append(leftOut[c.Reason], cmp.Or(c.Path, c.BasePath))
		}
	}
	var notAnalysed []string
	for _, reason := range []string{snapshot.ReasonGenerated, snapshot.ReasonIgnored} {
		if paths := leftOut[reason]; len(paths) > 0 {
			notAnalysed = append(notAnalysed, "the blast radius leaves out "+answer.Plural(len(paths), "changed code file")+
				" ("+reason+"): "+named(paths))
		}
	}
	if radius.Changed.All > 0 && radius.Changed.NotCounted == radius.Changed.All {
		radius.State = answer.RadiusNotAnalysed
		return radius, radiusRows(radius, nil), notAnalysed
	}
	gated := slices.ContainsFunc(r.Users, func(u snapshot.User) bool { return fromUsed(u) && !now.analysed(u.Path) })
	users := slices.DeleteFunc(slices.Clone(r.Users), func(u snapshot.User) bool { return !reached(u) })
	// Product files first, then tests, then the rest, each in path order: the reader checks product code first.
	slices.SortStableFunc(users, func(a, b snapshot.User) int { return roleOf(a) - roleOf(b) })
	radius.Files = len(users)
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
	radius.State = answer.RadiusCounted
	if radius.Changed.NotCounted > 0 || r.MissingNow || gated {
		radius.State = answer.RadiusAtLeast
	}
	return radius, radiusRows(radius, &omitted), notAnalysed
}

// addState counts a changed code file under its state.
func addState(c *answer.ChangedFiles, state snapshot.FileState) {
	c.All++
	switch state {
	case snapshot.ByName:
		c.ByName++
	case snapshot.AsAWhole:
		c.AsWhole++
	case snapshot.Deleted:
		c.Deleted++
	case snapshot.NothingAltered:
		c.NothingAltered++
	case snapshot.NotCounted:
		c.NotCounted++
	}
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

// altered says whether a counted changed code file altered, removed or could not tell what it changed: then the
// importers of the change are worth a look.
func altered(r *answer.Radius) bool {
	return r.Changed.ByName+r.Changed.AsWhole+r.Changed.Deleted > 0
}

// radiusRows are the report rows of the radius: its number with its parts, and the files it lists. omitted holds the
// parts of the files left out of the list.
func radiusRows(r, omitted *answer.Radius) []answer.Statistic {
	if r.Changed.All == 0 {
		note := "0 files; no code file changed"
		if r.State == answer.RadiusAtLeast { // a code file's content is missing now
			note = "0 files found, an incomplete count; no code file changed"
		}
		return []answer.Statistic{{Name: "blast radius", Note: note}}
	}
	var states []string
	for _, s := range []struct {
		n     int
		label string
	}{{r.Changed.ByName, "altered names"}, {r.Changed.AsWhole, "as a whole"}, {r.Changed.Deleted, "deleted"},
		{r.Changed.NothingAltered, "altered no name"}, {r.Changed.NotCounted, "not counted"}} {
		if s.n > 0 {
			states = append(states, count(s.n)+" "+s.label)
		}
	}
	changedNote := answer.Plural(r.Changed.All, "changed code file") + ": " + strings.Join(states, ", ")
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
		parts = append(parts, answer.Agree(r.AsWhole, "of them counts", "of them count")+" because a changed file counts as a whole")
	}
	parts = append(parts, changedNote)
	if r.InChange > 0 {
		parts = append(parts, answer.Agree(r.InChange, "more user is a file", "more users are files")+" the change touched")
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
