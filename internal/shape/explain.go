package shape

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/juzhxng/plimsoll/internal/answer"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Explain gives the evidence of one mark, computed again from the snapshot: names, paths and counts, never a
// file's content or a setting's value.
func (Detector) Explain(s snapshot.Snapshot, m answer.Mark) []string {
	switch answer.RuleOf(m.ID) {
	case ruleSize:
		return sizeEvidence(s, m)
	case ruleRatio:
		return ratioEvidence(s, m)
	case ruleDependencies:
		return dependencyEvidence(s, m)
	case ruleSettings:
		return settingsEvidence(s)
	}
	return nil
}

func sizeEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	path := m.Subject.Paths[0]
	lines := []string{"not tracked now"}
	file, nowFound := fileAt(s.Now.Files, path)
	if nowFound {
		lines[0] = "lines now: " + answer.Count(float64(file.Lines))
	}
	previous, existed := baseFile(s, path)
	switch {
	case !existed:
		lines = append(lines, "not tracked at the base")
	case previous.Path != path:
		lines = append(lines, fmt.Sprintf("lines at the base: %s, as `%s`", answer.Count(float64(previous.Lines)), previous.Path))
	default:
		lines = append(lines, "lines at the base: "+answer.Count(float64(previous.Lines)))
	}
	if !nowFound {
		file = previous
	}
	language := file.Language
	if language == "" {
		language = "no language entry"
	}
	return append(lines,
		fmt.Sprintf("counted as: %s (%s)", file.Role, language),
		fmt.Sprintf("limit: %s lines (%s)", answer.Count(file.Limit.Value), file.Limit.Source),
		"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
	)
}

func ratioEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	isTest := func(f snapshot.File) bool { return f.Role.IsTest() }
	isProduct := func(f snapshot.File) bool { return f.Role == snapshot.Product }
	isOther := func(f snapshot.File) bool { return f.Role == snapshot.Other }
	lines := []string{
		fmt.Sprintf("test lines now: %s; at the base: %s", tally(s.Now.Files, isTest), tally(s.Before.Files, isTest)),
		fmt.Sprintf("product lines now: %s; at the base: %s", tally(s.Now.Files, isProduct), tally(s.Before.Files, isProduct)),
		"other code, outside the ratio: " + tallyLines(s.Now.Files, isOther),
		"test code recognized by: " + s.TestRule,
	}
	lines = append(lines, answer.TestFolderNotes(s.Layout)...)
	lines = append(lines, fmt.Sprintf("limit: %s (%s)", answer.Decimal(*m.Measured.Limit), m.Measured.LimitSource))
	var tests []snapshot.File
	for _, file := range s.Now.Files {
		if isTest(file) {
			tests = append(tests, file)
		}
	}
	slices.SortFunc(tests, func(a, b snapshot.File) int {
		return cmp.Or(cmp.Compare(b.Lines, a.Lines), strings.Compare(a.Path, b.Path))
	})
	for _, file := range tests {
		kind := "defines a test"
		if file.Role == snapshot.TestInfrastructure {
			kind = "test infrastructure"
		}
		lines = append(lines, fmt.Sprintf("`%s`: %s, %s", file.Path, answer.Plural(file.Lines, "line"), kind))
	}
	return lines
}

// sum is the lines and the number of the files that match.
func sum(files []snapshot.File, match func(snapshot.File) bool) (lines, count int) {
	for _, file := range files {
		if match(file) {
			lines += file.Lines
			count++
		}
	}
	return lines, count
}

// tally is "<lines> in <files>" for the files that match.
func tally(files []snapshot.File, match func(snapshot.File) bool) string {
	lines, count := sum(files, match)
	return answer.Count(float64(lines)) + " in " + answer.Plural(count, "file")
}

// tallyLines is "<n> line(s) in <files>" for the files that match.
func tallyLines(files []snapshot.File, match func(snapshot.File) bool) string {
	lines, count := sum(files, match)
	return answer.Plural(lines, "line") + " in " + answer.Plural(count, "file")
}

func dependencyEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	path := m.Subject.Paths[0]
	now, _ := manifestAt(s.Now, path)
	before := fmt.Sprintf("`%s` not tracked at the base", path)
	if previous, existed := manifestAt(s.Before, path); existed {
		before = fmt.Sprintf("`%s` at the base: %s", path, strings.Join(previous.Dependencies, ", "))
	}
	return []string{before, fmt.Sprintf("`%s` now: %s", path, strings.Join(now.Dependencies, ", ")), "added: " + strings.Join(m.Subject.Names, ", ")}
}

func settingsEvidence(s snapshot.Snapshot) []string {
	var previous []string
	if s.Before.Settings != nil {
		previous = s.Before.Settings.Names
	}
	return []string{
		fmt.Sprintf("names at the base: %d", len(previous)),
		fmt.Sprintf("names now: %d", len(s.Now.Settings.Names)),
		"added: " + strings.Join(addedNames(previous, s.Now.Settings.Names), ", "),
	}
}
