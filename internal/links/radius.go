package links

import (
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

// reasonLinksNotUsed is why a changed file of a language whose links are not used is not counted.
const reasonLinksNotUsed = "its language's imports are not used"

// blastRadius counts the radius of the change over the languages whose links are used, and names the changed files
// it could not count.
func blastRadius(s snapshot.Snapshot, used []snapshot.LanguageImports) (*answer.Radius, []answer.Statistic, []string) {
	r := s.Radius
	if len(r.Changed) == 0 && !r.MissingNow {
		return &answer.Radius{State: answer.RadiusCounted, Paths: []string{}}, nil, nil
	}
	languages := map[string]bool{}
	for _, l := range used {
		languages[l.Language] = true
	}
	reached := func(u snapshot.User) bool {
		return slices.ContainsFunc(u.Languages, func(l string) bool { return languages[l] })
	}
	counts := map[snapshot.FileState]int{}
	notCounted := map[string][]string{} // reason -> paths
	for _, c := range r.Changed {
		state, reason := c.State, c.Reason
		if state != snapshot.NothingAltered && state != snapshot.NotCounted && !languages[c.Language] {
			state, reason = snapshot.NotCounted, reasonLinksNotUsed
		}
		counts[state]++
		if state == snapshot.NotCounted {
			p := c.Path
			if p == "" {
				p = c.BasePath
			}
			notCounted[reason] = append(notCounted[reason], p)
		}
	}
	radius := &answer.Radius{Paths: []string{}}
	for _, u := range r.Users {
		if !reached(u) {
			continue
		}
		radius.Files++
		switch {
		case u.ByName:
			radius.ByName++
		default:
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
	counted := len(r.Changed) - counts[snapshot.NotCounted]
	switch {
	case len(r.Changed) > 0 && counted == 0:
		radius = &answer.Radius{State: answer.RadiusNotAnalysed, Paths: []string{}}
	case counts[snapshot.NotCounted] > 0 || radius.Unread > 0 || r.MissingNow:
		radius.State = answer.RadiusAtLeast
	default:
		radius.State = answer.RadiusCounted
	}
	return radius, radiusRows(radius, counts, len(r.Changed)), notCountedSentences(notCounted)
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
	value := float64(r.Files)
	parts := []string{}
	if r.State == answer.RadiusAtLeast {
		parts = append(parts, "at least")
	}
	parts = append(parts, count(r.ByName)+" by name, "+count(r.AsWhole)+" as a whole")
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
	rows := []answer.Statistic{{Name: "blast radius", Value: &value, Unit: "files", Note: strings.Join(parts, "; ")}}
	if r.Files > 0 {
		listed := "`" + strings.Join(r.Paths, "`, `") + "`"
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

// notCountedSentences name the changed code files the radius could not count for a reason no other entry of "not
// analysed" says already: skipped folders, missing content and languages whose imports are not used have their own.
func notCountedSentences(byReason map[string][]string) []string {
	var sentences []string
	for _, reason := range []string{snapshot.ReasonGenerated} {
		paths := byReason[reason]
		if len(paths) == 0 {
			continue
		}
		slices.Sort(paths)
		listed := "`" + strings.Join(paths[:min(len(paths), 3)], "`, `") + "`"
		if len(paths) > 3 {
			listed += " and " + count(len(paths)-3) + " more"
		}
		sentences = append(sentences, "the blast radius leaves out "+answer.Plural(len(paths), "changed code file")+" ("+reason+"): "+listed)
	}
	return sentences
}
