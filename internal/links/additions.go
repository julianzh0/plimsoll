package links

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// addedCodeRow names the rows of code a change added that nothing uses.
const addedCodeRow = "added code without a use"

// additionsNote says what the rows of added code without a use hold and leave out.
const additionsNote = "added code without a use: functions, types and values this change added at the top level of a product file whose " +
	"name no code or configuration file of the repository holds outside its own declaration; a use by test code alone, or a mention " +
	"in documentation alone, is said on the line; left out: what a decorator or an annotation registers, default exports, files a " +
	"framework loads by convention, generated code, example projects and the exported names of a published library"

// The reasons of a row, in the order a file's rows are printed.
const (
	noUse      = "no use in the repository's code files"
	onlyByTest = "used only by tests"
)

// additionRows lists the additions without a use outside tests: one row per file and reason, files by path, and the
// one status sentence that counts them; nothing when every addition is used.
func additionRows(additions []snapshot.Addition) (rows []answer.Statistic, summary string) {
	type group struct {
		path, reason string
		order        int
		names        []snapshot.Addition
	}
	var groups []*group
	files := map[string]bool{}
	names := 0
	for _, a := range additions {
		if a.Used {
			continue
		}
		reason, order := noUse, 0
		switch {
		case len(a.Docs) > 0 && !a.Tests:
			reason, order = noUse+", named only in "+documents(a.Docs), 1
		case a.Tests:
			reason, order = onlyByTest, 2
		}
		i := slices.IndexFunc(groups, func(g *group) bool { return g.path == a.Path && g.reason == reason })
		if i < 0 {
			groups, i = append(groups, &group{path: a.Path, reason: reason, order: order}), len(groups)
		}
		groups[i].names = append(groups[i].names, a)
		files[a.Path] = true
		names++
	}
	if names == 0 {
		return nil, ""
	}
	slices.SortStableFunc(groups, func(a, b *group) int {
		if c := strings.Compare(a.path, b.path); c != 0 {
			return c
		}
		return a.order - b.order
	})
	for _, g := range groups {
		rows = append(rows, answer.Statistic{Name: addedCodeRow, Subject: g.path, Note: byKind(g.names) + "; " + g.reason})
	}
	them := "them"
	if names == 1 {
		them = "it"
	}
	return rows, fmt.Sprintf("Added code without a non-test use: %s in %s; `report` lists %s.",
		answer.Plural(names, "name"), answer.Plural(len(files), "file"), them)
}

// byKind names the additions sorted and grouped by kind, functions first: "`a`, `b` (functions), `C` (type)".
func byKind(additions []snapshot.Addition) string {
	var parts []string
	for _, kind := range []snapshot.AdditionKind{snapshot.AddedFunction, snapshot.AddedType, snapshot.AddedValue} {
		var names []string
		for _, a := range additions {
			if a.Kind == kind {
				names = append(names, a.Name)
			}
		}
		if len(names) == 0 {
			continue
		}
		slices.SortFunc(names, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
		word := kind.String()
		if len(names) > 1 {
			word += "s"
		}
		parts = append(parts, answer.QuotedList(names)+" ("+word+")")
	}
	return strings.Join(parts, ", ")
}

// documents names the first documentation file by path and counts the rest.
func documents(paths []string) string {
	text := "`" + paths[0] + "`"
	if len(paths) > 1 {
		text += " and " + answer.Plural(len(paths)-1, "more document")
	}
	return text
}
