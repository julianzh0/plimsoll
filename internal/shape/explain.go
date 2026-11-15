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
	lines := []string{
		fmt.Sprintf("test lines now: %s; at the base: %s", tally(s.Now.Files, testLinesOf), tally(s.Before.Files, testLinesOf)),
		fmt.Sprintf("product lines now: %s; at the base: %s", tally(s.Now.Files, productLinesOf), tally(s.Before.Files, productLinesOf)),
		"other code, outside the ratio: " + tallyLines(s.Now.Files, otherLinesOf),
		"test code recognized by: " + s.TestRule,
	}
	lines = append(lines, answer.TestFolderNotes(s.Layout)...)
	lines = append(lines, fmt.Sprintf("limit: %s (%s)", answer.Decimal(*m.Measured.Limit), m.Measured.LimitSource))
	var tests []snapshot.File
	for _, file := range s.Now.Files {
		if _, holds := testLinesOf(file); holds {
			tests = append(tests, file)
		}
	}
	measured := func(f snapshot.File) int { n, _ := testLinesOf(f); return n }
	slices.SortFunc(tests, func(a, b snapshot.File) int {
		return cmp.Or(cmp.Compare(measured(b), measured(a)), strings.Compare(a.Path, b.Path))
	})
	for _, file := range tests {
		kind := map[snapshot.Role]string{
			snapshot.Test: "defines a test", snapshot.TestInfrastructure: "test infrastructure", snapshot.Product: "test code inside a product file",
		}[file.Role]
		lines = append(lines, fmt.Sprintf("`%s`: %s, %s", file.Path, answer.Plural(measured(file), "line"), kind))
	}
	return lines
}

// The lines a file adds to one sum of the ratio, as lines() counts them, and whether it adds to that sum at all.
func testLinesOf(f snapshot.File) (int, bool) {
	if f.Role.IsTest() {
		return f.Lines, true
	}
	return f.TestLines, f.Role == snapshot.Product && f.TestLines > 0
}

func productLinesOf(f snapshot.File) (int, bool) {
	return f.Lines - f.TestLines, f.Role == snapshot.Product
}

func otherLinesOf(f snapshot.File) (int, bool) { return f.Lines, f.Role == snapshot.Other }

// sum is the lines and the number of the files that a measure takes.
func sum(files []snapshot.File, measure func(snapshot.File) (int, bool)) (lines, count int) {
	for _, file := range files {
		if n, takes := measure(file); takes {
			lines += n
			count++
		}
	}
	return lines, count
}

// tally is "<lines> in <files>" for the files that match.
func tally(files []snapshot.File, measure func(snapshot.File) (int, bool)) string {
	lines, count := sum(files, measure)
	return answer.Count(float64(lines)) + " in " + answer.Plural(count, "file")
}

// tallyLines is "<n> line(s) in <files>" for the files that match.
func tallyLines(files []snapshot.File, measure func(snapshot.File) (int, bool)) string {
	lines, count := sum(files, measure)
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
