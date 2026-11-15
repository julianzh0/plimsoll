package links

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// shown is how many files the ranked rows name.
const shown = 3

const reachNote = "reach: files that import the changed files directly and that the change did not touch; " +
	"importers behind a file that only passes imports on are not followed"

// importersOf are the distinct importers of some files, sorted, and how many of them the set holds.
func importersOf(g graph, files []string, left map[string]bool) (outside []string, inside int) {
	seen := map[string]bool{}
	for _, file := range files {
		for _, importer := range g.importers(file) {
			switch {
			case seen[importer]:
			case left[importer]:
				inside++
			default:
				outside = append(outside, importer)
			}
			seen[importer] = true
		}
	}
	slices.Sort(outside)
	return outside, inside
}

// atBase is a count at the base for a file now, through its base path; nil when the file did not exist there.
func atBase(s snapshot.Snapshot, before graph, file string, count func(path string) int) *float64 {
	basePath := s.BasePath(file)
	if !before.has(basePath) {
		return nil
	}
	value := float64(count(basePath))
	return &value
}

// reachRows are the reach of the change and the changed files with the widest reach. The changed files are the code
// files of the analysed languages that a change names on each side; their importers that the change touched are
// counted apart.
func reachRows(s snapshot.Snapshot, now graph) []answer.Statistic {
	touched := map[string]bool{}
	for _, ch := range s.Changes {
		touched[ch.Path] = true
		if ch.From != "" {
			touched[ch.From] = true
		}
	}
	changed := func(g graph) []string {
		return slices.DeleteFunc(slices.Clone(g.files), func(file string) bool { return !touched[file] })
	}
	changedNow := changed(now)
	outside, inside := importersOf(now, changedNow, touched)
	parts := []string{answer.Plural(len(outside), "file"), answer.Plural(len(changedNow), "changed code file")}
	if len(outside) > 0 {
		parts = append(parts, split(now, outside))
	}
	if inside > 0 {
		parts = append(parts, answer.Agree(inside, "more importer is", "more importers are")+" part of the change")
	}
	passing := slices.DeleteFunc(slices.Clone(outside), func(file string) bool { return !slices.Contains(s.Now.Links.PassOn, file) })
	if len(passing) > 0 {
		parts = append(parts, answer.Agree(len(passing), "only passes", "only pass")+" imports on")
	}
	// A sentence, not a number compared with the base: importers the change adds are touched by it, so they count on
	// neither side, and the value at the base would differ only through renames and deletions.
	rows := []answer.Statistic{{Name: "reach of the change", Note: strings.Join(parts, "; ")}}

	return append(rows, widestRows(now, changedNow, touched)...)
}

// widestRows give each changed file its importers that the change did not touch. Where a language imports folders,
// every file of a folder has the same importers, so the folder is one row.
func widestRows(now graph, changed []string, touched map[string]bool) []answer.Statistic {
	var subjects []string
	group := map[string][]string{}
	for _, file := range changed {
		subject := file
		if now.byDir[file] {
			subject = path.Dir(file) + "/"
		}
		if _, seen := group[subject]; !seen {
			subjects = append(subjects, subject)
		}
		group[subject] = append(group[subject], file)
	}
	slices.Sort(subjects)
	type reach struct {
		subject   string
		importers []string
	}
	var ranked []reach
	for _, subject := range subjects {
		if importers, _ := importersOf(now, group[subject], touched); len(importers) > 0 {
			ranked = append(ranked, reach{subject, importers})
		}
	}
	slices.SortStableFunc(ranked, func(a, b reach) int { return cmp.Compare(len(b.importers), len(a.importers)) })
	var rows []answer.Statistic
	for _, r := range ranked[:min(shown, len(ranked))] {
		rows = append(rows, answer.Statistic{
			Name: "widest reach", Subject: r.subject, Note: answer.Plural(len(r.importers), "file") + "; " + split(now, r.importers),
		})
	}
	return rows
}

// top are the rows with the highest values, at most shown of them; the rows come sorted by path, so a tie keeps it.
func top(rows []answer.Statistic) []answer.Statistic {
	slices.SortStableFunc(rows, func(a, b answer.Statistic) int { return cmp.Compare(*b.Value, *a.Value) })
	return rows[:min(shown, len(rows))]
}

// split names how many importers are product, test and other code.
func split(g graph, importers []string) string {
	product, test, other := 0, 0, 0
	for _, importer := range importers {
		switch role := g.roles[importer]; {
		case role == snapshot.Product:
			product++
		case role.IsTest():
			test++
		default:
			other++
		}
	}
	parts := []string{count(product) + " product", count(test) + " test"}
	if other > 0 {
		parts = append(parts, count(other)+" other")
	}
	return strings.Join(parts, ", ")
}

// mostImportsRows are the files that import the most own targets now, with their count at the base.
func mostImportsRows(s snapshot.Snapshot, now, before graph) []answer.Statistic {
	var rows []answer.Statistic
	for _, file := range now.files {
		if targets := now.targets(file); targets > 0 {
			value := float64(targets)
			rows = append(rows, answer.Statistic{
				Name: "most imports", Subject: file, Value: &value, Unit: "own targets", Before: atBase(s, before, file, before.targets),
			})
		}
	}
	return top(rows)
}

// newFileRows list each product file new since the base that no link reaches.
func newFileRows(s snapshot.Snapshot, now, before graph) []answer.Statistic {
	var rows []answer.Statistic
	for _, file := range now.files {
		if now.roles[file] != snapshot.Product || now.runs[file] || before.has(s.BasePath(file)) || len(now.to[file]) > 0 {
			continue
		}
		rows = append(rows, answer.Statistic{
			Name: "new product file without an importer", Subject: file,
			Note: "no import of it in the " + answer.Plural(len(now.files), "code file") + " read",
		})
	}
	return rows
}

// namedRows are the reach of the files the caller named, then one row per named path: its importers that are not
// named, or why it has no number.
func namedRows(s snapshot.Snapshot, now, before graph) []answer.Statistic {
	if len(s.Named) == 0 {
		return nil
	}
	tracked := map[string]bool{}
	for _, p := range s.Now.Paths {
		tracked[p] = true
	}
	named := map[string]bool{}
	for _, p := range s.Named {
		named[p], named[s.BasePath(p)] = true, true
	}
	var counted []string
	notFound := 0
	var rows []answer.Statistic
	for _, p := range s.Named {
		row := answer.Statistic{Name: "named file", Subject: p}
		_, isCode := now.roles[p]
		switch {
		case !tracked[p]:
			notFound++
			row.Note = "not found among the tracked files"
		case !isCode:
			row.Note = "tracked, but not a code file"
		case !now.analysed(p):
			row.Note = "its language is not analysed"
		default:
			counted = append(counted, p)
			importers, _ := importersOf(now, []string{p}, named)
			value := float64(len(importers))
			row.Value, row.Unit, row.Note = &value, "files", split(now, importers)
			row.Before = atBase(s, before, p, func(basePath string) int {
				importersBefore, _ := importersOf(before, []string{basePath}, named)
				return len(importersBefore)
			})
		}
		rows = append(rows, row)
	}
	countedAtBase := make([]string, 0, len(counted))
	for _, p := range counted {
		countedAtBase = append(countedAtBase, s.BasePath(p))
	}
	importers, _ := importersOf(now, counted, named)
	importersBefore, _ := importersOf(before, countedAtBase, named)
	value, valueBefore := float64(len(importers)), float64(len(importersBefore))
	note := count(len(s.Named)) + " named"
	if notFound > 0 {
		note += ", " + count(notFound) + " not found"
	}
	if len(importers) > 0 {
		note += "; " + split(now, importers)
	}
	summary := answer.Statistic{Name: "reach of the named files", Note: note}
	if len(counted) > 0 {
		summary.Value, summary.Before, summary.Unit = &value, &valueBefore, "files"
	}
	return append([]answer.Statistic{summary}, rows...)
}

// folderRow is the folder cycles now and at the base, with what was compared.
func folderRow(now, before [][]string, compared int) answer.Statistic {
	cyclesNow, cyclesBefore := float64(len(now)), float64(len(before))
	row := answer.Statistic{Name: "folder cycles", Value: &cyclesNow, Before: &cyclesBefore, Unit: "cycles"}
	switch {
	case compared == 0:
		row.Note = "no import between sibling folders of product code"
	case len(now) == 0:
		row.Note = "among " + answer.Plural(compared, "import") + " between sibling folders"
	default:
		row.Note = answer.Plural(members(now), "folder") + ", among " + answer.Plural(compared, "import") + " between sibling folders"
	}
	return row
}

// ringRows are the rings now and at the base, and the files that are in a ring only through type-only imports.
func ringRows(now, before, nowWithTypes, beforeWithTypes [][]string) []answer.Statistic {
	ringsNow, ringsBefore := float64(len(now)), float64(len(before))
	rings := answer.Statistic{Name: "import rings", Value: &ringsNow, Before: &ringsBefore, Unit: "rings"}
	if files := members(now); files > 0 {
		rings.Note = answer.Plural(files, "file")
	}
	typesNow, typesBefore := float64(members(nowWithTypes)-members(now)), float64(members(beforeWithTypes)-members(before))
	return []answer.Statistic{
		rings,
		{Name: "files in a ring only through type-only imports", Value: &typesNow, Before: &typesBefore, Unit: "files"},
	}
}

// members counts the members of some groups: the files of rings, or the folders of folder cycles.
func members(rings [][]string) int {
	n := 0
	for _, ring := range rings {
		n += len(ring)
	}
	return n
}
