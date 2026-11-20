package links

import (
	"cmp"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// shown is how many files the ranked rows name.
const shown = 3

const importersNote = "importers of the change: untouched files that import a changed code file, directly or at any " +
	"distance; a deleted or renamed file counts the importers of its old path; the share is of the product code files " +
	"whose imports are read"

// importersOf are the distinct importers of some files that the set left out does not hold, sorted.
func importersOf(g graph, files []string, left map[string]bool) []string {
	seen := map[string]bool{}
	var outside []string
	for _, file := range files {
		for _, importer := range g.importers(file) {
			if !seen[importer] && !left[importer] {
				outside = append(outside, importer)
			}
			seen[importer] = true
		}
	}
	slices.Sort(outside)
	return outside
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

// importers of the change: the direct ones, those behind them at any distance, and the files the change touched
// among them. The changed files are the code files of the analysed languages that a change names now; a deleted or
// renamed file adds the files that imported its base path at the base and exist now, under their path now.
type changeImporters struct {
	changedNow    []string        // the changed code files now, sorted
	touched       map[string]bool // the paths a change names, on either side
	direct, total []string        // untouched, sorted
	inside        int             // importers the change touched
}

func importersOfChange(s snapshot.Snapshot, now, before graph) changeImporters {
	r := changeImporters{touched: map[string]bool{}}
	nowPath := map[string]string{} // base path -> path now, for the files a change renamed
	for _, ch := range s.Changes {
		r.touched[ch.Path] = true
		if ch.From != "" {
			r.touched[ch.From], nowPath[ch.From] = true, ch.Path
		}
	}
	for _, file := range now.files {
		if r.touched[file] {
			r.changedNow = append(r.changedNow, file)
		}
	}
	reached, inside := map[string]bool{}, map[string]bool{}
	var frontier []string
	reach := func(file string) {
		if reached[file] || inside[file] {
			return
		}
		if r.touched[file] {
			inside[file] = true
		} else {
			reached[file] = true
		}
		frontier = append(frontier, file)
	}
	for _, file := range r.changedNow {
		for _, importer := range now.importers(file) {
			reach(importer)
		}
	}
	for _, file := range before.files {
		if !r.touched[file] || now.analysed(file) {
			continue // unchanged, or still here under the same path
		}
		for _, importer := range before.importers(file) {
			if moved, renamed := nowPath[importer]; renamed {
				importer = moved
			}
			if now.has(importer) {
				reach(importer)
			}
		}
	}
	r.direct = slices.Sorted(maps.Keys(reached))
	r.inside = len(inside)
	for len(frontier) > 0 {
		file := frontier[0]
		frontier = frontier[1:]
		for _, importer := range now.importers(file) {
			reach(importer)
		}
	}
	r.total = slices.Sorted(maps.Keys(reached))
	return r
}

// importerRows put the importers of the change into the radius and give its row and the changed files with the most
// direct importers.
func importerRows(s snapshot.Snapshot, now, before graph, radius *answer.Radius) []answer.Statistic {
	r := importersOfChange(s, now, before)
	radius.Direct, radius.Total = importersCount(now, r.direct), importersCount(now, r.total)
	radius.Folders, radius.OtherFolders = answer.Folders(r.total)
	radius.ImportersInChange = r.inside
	direct := answer.Plural(radius.Direct.Files, "file") + " directly"
	if radius.Direct.Files > 0 {
		direct += " (" + radius.Direct.Split() + ")"
	}
	total := count(radius.Total.Files) + " at any distance"
	if radius.Total.Files > radius.Direct.Files {
		total += " (" + radius.Total.Split() + ")"
	}
	parts := []string{direct + ", " + total}
	if radius.Total.Files > 0 {
		parts = append(parts, "in "+answer.FoldersText(radius.Folders, radius.OtherFolders))
		if radius.ProductCodeFiles > 0 {
			parts = append(parts, count(radius.Total.Product)+" of "+answer.Plural(radius.ProductCodeFiles, "product code file"))
		}
	}
	if radius.ImportersInChange > 0 {
		parts = append(parts, answer.Agree(radius.ImportersInChange, "more importer is a file", "more importers are files")+" the change touched")
	}
	// A sentence, not a number compared with the base: importers the change adds are touched by it, so they count on
	// neither side, and the value at the base would differ only through renames and deletions.
	rows := []answer.Statistic{{Name: "importers of the change", Note: strings.Join(parts, "; ")}}
	return append(rows, mostDirectRows(now, r.changedNow, r.touched)...)
}

// mostDirectRows give each changed file its importers that the change did not touch. Where a language imports folders,
// every file of a folder has the same importers, so the folder is one row.
func mostDirectRows(now graph, changed []string, touched map[string]bool) []answer.Statistic {
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
	type importerRow struct {
		subject   string
		importers []string
	}
	var ranked []importerRow
	for _, subject := range subjects {
		if importers := importersOf(now, group[subject], touched); len(importers) > 0 {
			ranked = append(ranked, importerRow{subject, importers})
		}
	}
	slices.SortStableFunc(ranked, func(a, b importerRow) int { return cmp.Compare(len(b.importers), len(a.importers)) })
	var rows []answer.Statistic
	for _, r := range ranked[:min(shown, len(ranked))] {
		rows = append(rows, answer.Statistic{
			Name: "direct importers of", Subject: r.subject, Note: answer.Plural(len(r.importers), "file") + "; " + split(now, r.importers),
		})
	}
	return rows
}

// top are the rows with the highest values, at most shown of them; the rows come sorted by path, so a tie keeps it.
func top(rows []answer.Statistic) []answer.Statistic {
	slices.SortStableFunc(rows, func(a, b answer.Statistic) int { return cmp.Compare(*b.Value, *a.Value) })
	return rows[:min(shown, len(rows))]
}

// roles counts files by role.
type roles struct{ product, test, other int }

func rolesOf(g graph, files []string) roles {
	var r roles
	for _, file := range files {
		switch role := g.roles[file]; {
		case role == snapshot.Product:
			r.product++
		case role.IsTest():
			r.test++
		default:
			r.other++
		}
	}
	return r
}

// importers counts files by role.
func importersCount(g graph, files []string) answer.Importers {
	r := rolesOf(g, files)
	return answer.Importers{Files: len(files), Product: r.product, Tests: r.test, Other: r.other}
}

// split names how many importers are product, test and other code.
func split(g graph, importers []string) string {
	return importersCount(g, importers).Split()
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

// newFileRows list each product file new since the base that no link reaches; a file of a package an import names is
// reached, as its peers use it without one.
func newFileRows(s snapshot.Snapshot, now, before graph) []answer.Statistic {
	var rows []answer.Statistic
	for _, file := range now.files {
		if now.roles[file] != snapshot.Product || now.runs[file] || before.has(s.BasePath(file)) || len(now.to[file]) > 0 ||
			now.byDir[file] && now.reached[path.Dir(file)] {
			continue
		}
		rows = append(rows, answer.Statistic{
			Name: "new product file without an importer", Subject: file,
			Note: "no import of it in the " + answer.Plural(len(now.files), "code file") + " read",
		})
	}
	return rows
}

// namedRows are the importers of the files the caller named, then one row per named path: its importers that are not
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
			importers := importersOf(now, []string{p}, named)
			value := float64(len(importers))
			row.Value, row.Unit, row.Note = &value, "files", split(now, importers)
			row.Before = atBase(s, before, p, func(basePath string) int {
				importersBefore := importersOf(before, []string{basePath}, named)
				return len(importersBefore)
			})
		}
		rows = append(rows, row)
	}
	countedAtBase := make([]string, 0, len(counted))
	for _, p := range counted {
		countedAtBase = append(countedAtBase, s.BasePath(p))
	}
	importers := importersOf(now, counted, named)
	importersBefore := importersOf(before, countedAtBase, named)
	value, valueBefore := float64(len(importers)), float64(len(importersBefore))
	note := count(len(s.Named)) + " named"
	if notFound > 0 {
		note += ", " + count(notFound) + " not found"
	}
	if len(importers) > 0 {
		note += "; " + split(now, importers)
	}
	summary := answer.Statistic{Name: "importers of the named files", Note: note}
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
