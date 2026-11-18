package links

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Explain gives the imports a mark rests on, by file and line, computed again. It never quotes an import.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	languages, _ := analysed(s)
	now, before := graphsOf(s, languages)
	path := m.Subject.Paths[0]
	switch answer.RuleOf(m.ID) {
	case ruleTestImport:
		var lines []string
		for _, link := range now.testImports(path) {
			lines = append(lines, importLine(now, link, "imports"))
		}
		basePath := s.BasePath(path)
		if m.Change == answer.Resolved {
			for _, link := range before.testImports(basePath) {
				lines = append(lines, "at the base "+importLine(before, link, "imported"))
			}
		}
		atBase := "at the base: the file did not exist"
		if m.Measured.Before != nil {
			atBase = "at the base: " + answer.Plural(int(*m.Measured.Before), "import") + " of test code"
		}
		return append(lines, atBase, fmt.Sprintf("searched: the import lines of `%s`", path))
	case ruleCycle:
		g, line, prefix, verb := now, "ring of %s; at the base: %s", "", "imports"
		atBase := "none"
		if m.Measured.Before != nil {
			atBase = answer.Count(*m.Measured.Before)
		}
		if m.Change == answer.Resolved {
			g, line, prefix, verb = before, "no ring among these files now; at the base: a ring of %s", "at the base ", "imported"
		}
		header := fmt.Sprintf(line, answer.Plural(len(m.Subject.Paths), "file"), atBase)
		if m.Change == answer.Resolved {
			header = fmt.Sprintf(line, answer.Plural(len(m.Subject.Paths), "file"))
		}
		lines := []string{header}
		for _, link := range g.ringLinks(m.Subject.Paths) {
			lines = append(lines, prefix+linkLine(link, verb))
		}
		return append(lines, "type-only imports are left out")
	case ruleFolderCycle:
		return folderEvidence(m, now, before)
	case ruleNever, ruleOnly:
		return ruleEvidence(s, m, languages, now, before)
	}
	return nil
}

// folderEvidence names a folder cycle's members, how many imports go each way, the fewest first, and each import by
// file and line; for a resolved cycle, the imports of the base.
func folderEvidence(m answer.Mark, now, before graph) []string {
	members := m.Subject.Paths
	g, prefix, has, verb := now, "", "has", "imports"
	atBase := "none"
	if m.Measured.Before != nil {
		atBase = answer.Count(*m.Measured.Before)
	}
	header := fmt.Sprintf("cycle of %s %s: %s; at the base: %s", answer.Plural(len(members), "folder"), parentShown(members[0]), folderList(members), atBase)
	if m.Change == answer.Resolved {
		g, prefix, has, verb = before, "at the base ", "had", "imported"
		header = fmt.Sprintf("no cycle among these folders now; at the base: a cycle of %s %s", answer.Plural(len(members), "folder"), parentShown(members[0]))
	}
	type direction struct{ from, to string }
	byDirection := map[direction][]snapshot.Link{}
	for _, imp := range g.siblingImports(g.rings(false)) {
		if slices.Contains(members, imp.from) && slices.Contains(members, imp.to) {
			d := direction{imp.from, imp.to}
			byDirection[d] = append(byDirection[d], imp.link)
		}
	}
	directions := slices.SortedFunc(maps.Keys(byDirection), func(a, b direction) int {
		return cmp.Or(cmp.Compare(len(byDirection[a]), len(byDirection[b])), strings.Compare(a.from, b.from), strings.Compare(a.to, b.to))
	})
	lines := []string{header}
	for _, d := range directions {
		lines = append(lines, fmt.Sprintf("%s%s %s %s of %s", prefix, answer.Folder(d.from), has, answer.Plural(len(byDirection[d]), "import"), answer.Folder(d.to)))
	}
	for _, d := range directions {
		for _, link := range byDirection[d] { // siblingImports gives them by file and line
			lines = append(lines, prefix+linkLine(link, verb))
		}
	}
	return append(lines, "imports between two files of one import ring are left out")
}

// importLine is one import by file, line and target, with the target's role; never its text.
func importLine(g graph, link snapshot.Link, verb string) string {
	role := g.roles[link.To].String()
	if link.Folder {
		role = answer.Plural(len(link.Files), "file") + " of test code"
	}
	return importedAs(link, verb) + " (" + role + ")" + suffixes(link)
}

// linkLine is one import by file, line and target; never its text.
func linkLine(link snapshot.Link, verb string) string {
	return importedAs(link, verb) + suffixes(link)
}

// importedAs names an import by file, line and target.
func importedAs(link snapshot.Link, verb string) string {
	return fmt.Sprintf("`%s` line %d %s %s", link.From, link.Line, verb, targetShown(link))
}

// suffixes say whether an import names types only and whether the change wrote it.
func suffixes(link snapshot.Link) string {
	var ends string
	if link.TypeOnly {
		ends += typeOnlySuffix
	}
	if link.Added {
		ends += addedSuffix
	}
	return ends
}

// addedSuffix marks an import the change wrote.
const addedSuffix = ", added by this change"

// typeOnlySuffix marks an import that names types only.
const typeOnlySuffix = ", type-only"

// targetShown names an import's target: a file, or the folder a language imports.
func targetShown(link snapshot.Link) string {
	if link.Folder {
		return "the folder `" + link.To + "`"
	}
	return "`" + link.To + "`"
}

// folderList prints folders as a sentence lists them: "`src/a/`, `src/b/`".
func folderList(folders []string) string {
	shown := make([]string, 0, len(folders))
	for _, folder := range folders {
		shown = append(shown, answer.Folder(folder))
	}
	return strings.Join(shown, ", ")
}
