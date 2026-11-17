// Package shape is the shape detector: product, test and other lines and their ratio, file sizes, settings,
// runtime dependencies and churn, each against the base.
package shape

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const name = "shape"

// The rules of the detector, the middle part of their identifiers.
const (
	ruleRatio        = "test-ratio"
	ruleSize         = "file-size"
	ruleDependencies = "dependencies"
	ruleSettings     = "settings"
)

// churnWindow is how many of the newest non-merge commits the churn statistic covers, so months stay comparable.
const churnWindow = 150

// largestShown is how many of the largest files the report names.
const largestShown = 3

// additionSource is the rule of a mark that fires on any addition.
const additionSource = "any addition"

// Detector is the shape detector.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

// Detect measures both sides of the snapshot.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	now, before := lines(s.Now.Files), lines(s.Before.Files)
	f := answer.Findings{
		Notes:       slices.Concat([]string{"test code recognized by: " + s.TestRule}, answer.TestFolderNotes(s.Layout), answer.OwnNotes(s.Layout.Own)),
		NotAnalysed: append(answer.TestFoldersNotUsed(s.Layout), answer.OwnNotUsed(s.Layout.Own)...),
	}
	f.Notes = append(f.Notes, testCodeInsideNotes(s.Now.Files)...)
	if n := s.Footprint.HoldsTestCode; n > 0 {
		f.NotAnalysed = append(f.NotAnalysed, "the footprint counts, in "+answer.Agree(n, "changed product file that holds", "changed product files that hold")+
			" test code, only the growth of that test code as test lines and every other changed line as a product line")
	}
	f.Statistics = append(f.Statistics,
		stat("product lines", "", now.product, before.product, true, "lines"),
		stat("test lines", "", now.test(), before.test(), true, "lines"),
		stat("other code lines", "", now.other, before.other, true, "lines"),
	)
	if now.product == 0 {
		f.NotAnalysed = append(f.NotAnalysed, "no product code, so no test ratio")
	} else {
		if now.test() == 0 {
			f.NotAnalysed = append(f.NotAnalysed, "no test code recognized by: "+s.TestRule)
		}
		f.Statistics = append(f.Statistics, stat("test ratio", "", now.ratio(), before.ratio(), before.product > 0, "ratio"))
		f.Marks = append(f.Marks, ratioMark(s, now, before)...)
		if now.ratio() > s.Limits.TestRatio.Value {
			f.Statistics = append(f.Statistics, split(s, now, before)...)
		}
	}
	f.Statistics = append(f.Statistics, largest(s, s.Now.Files, "largest code file")...)
	f.Marks = append(f.Marks, sizeMarks(s)...)

	switch {
	case slices.Contains(s.Now.Broken, snapshot.SettingsFile):
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s` could not be read, so no settings count", snapshot.SettingsFile))
	case s.Now.Settings == nil:
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("no `%s`, so no settings were counted or compared", snapshot.SettingsFile))
	case slices.Contains(s.Before.Broken, snapshot.SettingsFile):
		f.Statistics = append(f.Statistics, stat("settings", "", float64(len(s.Now.Settings.Names)), 0, false, ""))
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s` could not be read at the base, so no settings were compared", snapshot.SettingsFile))
	default:
		count := float64(len(s.Now.Settings.Names))
		var beforeCount float64
		if s.Before.Settings != nil {
			beforeCount = float64(len(s.Before.Settings.Names))
		}
		f.Statistics = append(f.Statistics, stat("settings", "", count, beforeCount, s.Before.Settings != nil, ""))
		f.Marks = append(f.Marks, settingsMark(s)...)
	}

	if len(s.Now.Manifests) == 0 {
		f.NotAnalysed = append(f.NotAnalysed, "no known dependency manifest, at the root or as a project file outside test and other code, so no dependency count")
	}
	for _, manifest := range s.Now.Manifests {
		if slices.Contains(s.Before.Broken, manifest.Path) {
			f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s` could not be read at the base, so no dependencies were compared", manifest.Path))
			continue
		}
		previous, existed := manifestAt(s.Before, manifest.Path)
		row := stat("runtime dependencies", manifest.Path, float64(len(manifest.Dependencies)), float64(len(previous.Dependencies)), existed, "")
		if !existed || !slices.Equal(manifest.Dependencies, previous.Dependencies) {
			row.Note = strings.Join(manifest.Dependencies, ", ")
		}
		f.Statistics = append(f.Statistics, row)
		f.Marks = append(f.Marks, dependencyMark(manifest, previous, existed)...)
	}

	if len(s.History.Commits) == 0 {
		f.NotAnalysed = append(f.NotAnalysed, "no commit in the history, so no churn")
		return f
	}
	f.Statistics = append(f.Statistics, churn(s.History.Commits)...)
	if len(s.History.Commits) < churnWindow {
		f.Notes = append(f.Notes, fewerNote(len(s.History.Commits)))
	} else {
		f.Notes = append(f.Notes, fmt.Sprintf("churn over the last %d non-merge commits", churnWindow))
	}
	return f
}

// fewerNote says the churn covers the whole history, which is shorter than the window.
func fewerNote(commits int) string {
	if commits == 1 {
		return fmt.Sprintf("churn over the 1 non-merge commit (fewer than %d)", churnWindow)
	}
	return fmt.Sprintf("churn over all %d non-merge commits (fewer than %d)", commits, churnWindow)
}

// totals are the code lines of one side by role.
type totals struct {
	product, tests, infrastructure, other float64
}

func (t totals) test() float64 { return t.tests + t.infrastructure }

// ratio is test lines per product line, rounded to one decimal; 0 without product code.
func (t totals) ratio() float64 {
	if t.product == 0 {
		return 0
	}
	return math.Round(10*t.test()/t.product) / 10
}

// ratioLines are the lines a file adds to the product side and to the test side of the ratio: test code inside a
// product file counts as test lines.
func ratioLines(f snapshot.File) (product, test int) {
	switch {
	case f.Role == snapshot.Product:
		return f.Lines - f.TestLines, f.TestLines
	case f.Role.IsTest():
		return 0, f.Lines
	}
	return 0, 0
}

func lines(files []snapshot.File) totals {
	var t totals
	for _, file := range files {
		switch file.Role {
		case snapshot.Product:
			product, test := ratioLines(file)
			t.product += float64(product)
			t.tests += float64(test)
		case snapshot.Test:
			t.tests += float64(file.Lines)
		case snapshot.TestInfrastructure:
			t.infrastructure += float64(file.Lines)
		case snapshot.Other:
			t.other += float64(file.Lines)
		}
	}
	return t
}

// testCodeInsideNotes says, per language in the order its files come, how much test code its product files hold.
func testCodeInsideNotes(files []snapshot.File) []string {
	var languages []string
	holding, testLines := map[string]int{}, map[string]int{}
	for _, file := range files {
		if file.TestLines == 0 { // only a product file holds test lines
			continue
		}
		if holding[file.Language] == 0 {
			languages = append(languages, file.Language)
		}
		holding[file.Language]++
		testLines[file.Language] += file.TestLines
	}
	var notes []string
	for _, language := range languages {
		notes = append(notes, fmt.Sprintf("`%s`: test code inside %s counts as test lines, not as product lines: %s",
			language, answer.Plural(holding[language], "product file"), answer.Plural(testLines[language], "line")))
	}
	return notes
}

func stat(statName, subject string, now, before float64, existed bool, unit string) answer.Statistic {
	row := answer.Statistic{Name: statName, Subject: subject, Value: &now, Unit: unit}
	if existed {
		row.Before = &before
	}
	return row
}

func ratioMark(s snapshot.Snapshot, now, before totals) []answer.Mark {
	limit := s.Limits.TestRatio
	change, found := answer.Compare(
		answer.Reading{Holds: before.product > 0 && before.ratio() > limit.Value, Value: before.ratio()},
		answer.Reading{Holds: now.ratio() > limit.Value, Value: now.ratio()},
		limit.Step,
	)
	if !found {
		return nil
	}
	m := answer.Mark{
		ID:         answer.ID(name, ruleRatio, "repository"),
		Change:     change,
		Subject:    answer.Subject{Kind: "repository"},
		Confidence: answer.Exact,
		Measured:   answer.Measured{Value: now.ratio(), Limit: &limit.Value, LimitSource: limit.Source, Unit: "test lines per product line"},
		Hint:       fmt.Sprintf("%s test lines over %s product lines; `report` shows the split.", answer.Count(now.test()), answer.Count(now.product)),
	}
	if before.product > 0 {
		value := before.ratio()
		m.Measured.Before = &value
	}
	return []answer.Mark{m}
}

// split is the test code by content, when the ratio is over its limit: tests, infrastructure and its largest files.
func split(s snapshot.Snapshot, now, before totals) []answer.Statistic {
	var rows []answer.Statistic
	if now.tests > 0 {
		rows = append(rows, stat("tests", "", now.tests, before.tests, true, "lines"))
	}
	if now.infrastructure > 0 {
		rows = append(rows, stat("test infrastructure", "", now.infrastructure, before.infrastructure, true, "lines"))
		var infrastructure []snapshot.File
		for _, file := range s.Now.Files {
			if file.Role == snapshot.TestInfrastructure {
				infrastructure = append(infrastructure, file)
			}
		}
		rows = append(rows, largest(s, infrastructure, "largest test infrastructure file")...)
	}
	return rows
}

// largest names the largest files, ties by path, each with its lines at the base.
func largest(s snapshot.Snapshot, files []snapshot.File, statName string) []answer.Statistic {
	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b snapshot.File) int {
		return cmp.Or(cmp.Compare(b.Lines, a.Lines), strings.Compare(a.Path, b.Path))
	})
	var rows []answer.Statistic
	for _, file := range sorted[:min(len(sorted), largestShown)] {
		previous, existed := baseFile(s, file.Path)
		rows = append(rows, stat(statName, file.Path, float64(file.Lines), float64(previous.Lines), existed, "lines"))
	}
	return rows
}

// baseFile is a file at the base under the same path, or under its old name when a change renamed it. The files
// of a side are sorted by path, so it searches them by halves.
func baseFile(s snapshot.Snapshot, path string) (snapshot.File, bool) {
	return fileAt(s.Before.Files, s.BasePath(path))
}

func fileAt(files []snapshot.File, path string) (snapshot.File, bool) {
	index, found := slices.BinarySearchFunc(files, path, func(f snapshot.File, p string) int { return strings.Compare(f.Path, p) })
	if !found {
		return snapshot.File{}, false
	}
	return files[index], true
}

// sizeMarks compares every code file of both sides with its line limit. A renamed file is measured under its new
// name against its old one; the old name gives no item.
func sizeMarks(s snapshot.Snapshot) []answer.Mark {
	var marks []answer.Mark
	renamed := map[string]bool{}
	for _, file := range s.Now.Files {
		previous, existed := baseFile(s, file.Path)
		if existed && previous.Path != file.Path {
			renamed[previous.Path] = true
		}
		marks = append(marks, sizeMark(file, previous, existed, file.Lines)...)
	}
	for _, file := range s.Before.Files {
		if _, stillThere := fileAt(s.Now.Files, file.Path); !renamed[file.Path] && !stillThere {
			marks = append(marks, sizeMark(file, file, true, 0)...)
		}
	}
	return marks
}

func sizeMark(file, previous snapshot.File, existed bool, linesNow int) []answer.Mark {
	limit := file.Limit
	change, found := answer.Compare(
		answer.Reading{Holds: existed && float64(previous.Lines) > limit.Value, Value: float64(previous.Lines)},
		answer.Reading{Holds: float64(linesNow) > limit.Value, Value: float64(linesNow)},
		limit.Step,
	)
	if !found {
		return nil
	}
	m := answer.Mark{
		ID:         answer.ID(name, ruleSize, file.Path),
		Change:     change,
		Subject:    answer.Subject{Kind: "file", Paths: []string{file.Path}},
		Confidence: answer.Exact,
		Measured:   answer.Measured{Value: float64(linesNow), Limit: &limit.Value, LimitSource: limit.Source, Unit: "lines"},
		Hint:       "count the file's lines at the base and now.",
	}
	if existed {
		value := float64(previous.Lines)
		m.Measured.Before = &value
	}
	return []answer.Mark{m}
}

func manifestAt(t snapshot.Tree, path string) (snapshot.Manifest, bool) {
	index := slices.IndexFunc(t.Manifests, func(m snapshot.Manifest) bool { return m.Path == path })
	if index < 0 {
		return snapshot.Manifest{}, false
	}
	return t.Manifests[index], true
}

func dependencyMark(manifest, previous snapshot.Manifest, existed bool) []answer.Mark {
	added := addedNames(previous.Dependencies, manifest.Dependencies)
	if len(added) == 0 {
		return nil
	}
	hint := fmt.Sprintf("compare the runtime dependencies in `%s` at the base and now.", manifest.Path) + gone(previous.Dependencies, manifest.Dependencies)
	if unlocked := slices.DeleteFunc(slices.Clone(added), func(name string) bool { return !slices.Contains(manifest.Unlocked, name) }); len(unlocked) > 0 {
		hint += fmt.Sprintf(" Not in the lockfile %s: %s.", strings.Join(answer.Quoted(manifest.Lockfiles), " or "), strings.Join(answer.Quoted(unlocked), ", "))
	}
	return []answer.Mark{additionMark(ruleDependencies, manifest.Path, added, len(manifest.Dependencies), len(previous.Dependencies), existed,
		"runtime dependencies", hint)}
}

func settingsMark(s snapshot.Snapshot) []answer.Mark {
	var previous []string
	if s.Before.Settings != nil {
		previous = s.Before.Settings.Names
	}
	added := addedNames(previous, s.Now.Settings.Names)
	if len(added) == 0 {
		return nil
	}
	return []answer.Mark{additionMark(ruleSettings, s.Now.Settings.Path, added, len(s.Now.Settings.Names), len(previous), s.Before.Settings != nil,
		"settings", fmt.Sprintf("compare the names in `%s` at the base and now.", s.Now.Settings.Path)+gone(previous, s.Now.Settings.Names))}
}

// gone names what the base held and now does not, as a sentence that follows a hint, or is "" when nothing went.
func gone(before, now []string) string {
	removed := addedNames(now, before)
	if len(removed) == 0 {
		return ""
	}
	shown := answer.QuotedList(removed[:min(len(removed), goneShown)])
	if len(removed) > goneShown {
		shown += fmt.Sprintf(" and %d more", len(removed)-goneShown)
	}
	return " Gone since the base: " + shown + "."
}

// goneShown is how many removed names an addition names before it counts the rest.
const goneShown = 5

func additionMark(rule, path string, added []string, now, before int, existed bool, unit, hint string) answer.Mark {
	m := answer.Mark{
		ID:         answer.ID(name, rule, path),
		Change:     answer.New,
		Subject:    answer.Subject{Kind: "file", Paths: []string{path}, Names: added},
		Confidence: answer.Exact,
		Measured:   answer.Measured{Value: float64(now), LimitSource: additionSource, Unit: unit},
		Hint:       hint,
	}
	if existed {
		value := float64(before)
		m.Measured.Before = &value
	}
	return m
}

// addedNames are the names now that were not there before, sorted and without repeats.
func addedNames(before, now []string) []string {
	added := answer.Added(before, now)
	slices.Sort(added)
	return added
}

// churn sums added and removed lines by area over the newest commits, now and as it was at the base.
func churn(commits []snapshot.Commit) []answer.Statistic {
	var atBase []snapshot.Commit
	for _, commit := range commits {
		if !commit.Since {
			atBase = append(atBase, commit)
		}
	}
	now, before := areaChurn(commits), areaChurn(atBase)
	areas := []struct {
		label string
		area  snapshot.Area
	}{{"product", snapshot.AreaProduct}, {"tests", snapshot.AreaTests}, {"docs", snapshot.AreaDocs}, {"other", snapshot.AreaOther}}
	rows := make([]answer.Statistic, 0, len(areas))
	for _, a := range areas {
		rows = append(rows, stat("churn: "+a.label, "", now[a.area], before[a.area], true, "lines"))
	}
	return rows
}

func areaChurn(commits []snapshot.Commit) map[snapshot.Area]float64 {
	churn := snapshot.ChurnOf(commits[:min(len(commits), churnWindow)])
	sums := map[snapshot.Area]float64{}
	for _, area := range []snapshot.Area{snapshot.AreaProduct, snapshot.AreaTests, snapshot.AreaDocs, snapshot.AreaOther} {
		sums[area] = float64(churn.Total(area))
	}
	return sums
}
