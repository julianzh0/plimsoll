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
	case ruleComplexity, ruleNesting:
		return functionEvidence(s, m)
	case ruleDuplicate:
		return duplicateEvidence(s, m)
	}
	return nil
}

// functionEvidence is what a function item was measured and compared with: its lines, both measures, how the
// complexity counts, its reason, and the checks that did not leave it out. Only lines, counts, shares and paths.
func functionEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	rule := answer.RuleOf(m.ID)
	var f snapshot.FunctionChange
	found := false
	for _, c := range slices.Concat(s.Functions.Complex, s.Functions.Deeper) {
		if answer.ID(name, rule, functionID(c)) == m.ID {
			f, found = c, true
			break
		}
	}
	if !found {
		return nil
	}
	fs, complexity, nesting := s.Functions, s.Limits.FunctionComplexity, s.Limits.FunctionNesting
	where := fmt.Sprintf("lines now: %d–%d (%s); new in this change", f.Start, f.End, answer.Plural(f.End-f.Start+1, "line"))
	if !f.New {
		how := "its name"
		if f.ByBody {
			how = "its body"
		}
		where = fmt.Sprintf("lines now: %d–%d (%s); at the base: lines %d–%d of `%s`, matched by %s",
			f.Start, f.End, answer.Plural(f.End-f.Start+1, "line"), f.BaseStart, f.BaseEnd, f.BasePath, how)
	}
	before := func(n int) string {
		if f.New {
			return ""
		}
		return fmt.Sprintf("; at the base: %d", n)
	}
	complexityLine := fmt.Sprintf("cognitive complexity now: %d%s", f.Complexity, before(f.ComplexityBefore))
	nestingLine := fmt.Sprintf("deepest nesting now: %s at line %d%s", answer.Plural(f.Nesting, "level"), f.NestingLine, before(f.NestingBefore))
	counted := complexityCounting
	lines := []string{where}
	if rule == ruleComplexity {
		lines = append(lines,
			fmt.Sprintf("%s; limit %s (%s); a function that existed counts from a rise of %s", complexityLine, answer.Count(complexity.Value), complexity.Source, answer.Count(complexity.Step)),
			counted, nestingLine)
	} else {
		lines = append(lines, fmt.Sprintf("%s; limit %s (%s)", nestingLine, answer.Count(nesting.Value), nesting.Source), complexityLine, counted)
	}
	if len(f.Repeated) == 2 {
		lines = append(lines, fmt.Sprintf("repeated block: lines %d–%d and %d–%d, written by this change", f.Repeated[0].Start, f.Repeated[0].End, f.Repeated[1].Start, f.Repeated[1].End))
	}
	percent := func(share float64) string { return fmt.Sprintf("%d%%", int(math.Round(100*share))) }
	lines = append(lines,
		fmt.Sprintf("lines of it this change added: %d of %d", f.AddedLines, f.End-f.Start+1),
		fmt.Sprintf("found in lines this change removed: %s of its tokens (%s or more counts as moved code)", percent(f.MovedShare), percent(fs.MovedShare)))
	compared := answer.Agree(f.CopyFiles, "file at the base that holds", "files at the base that hold")
	switch {
	case f.CopyFiles == 0:
		lines = append(lines, "no file at the base holds its name or its longest names")
	case f.ClosestFile == "":
		lines = append(lines, fmt.Sprintf("compared with %s its name or its longest names; none holds a run of %d of its tokens", compared, fs.Run))
	default:
		lines = append(lines, fmt.Sprintf("compared with %s its name or its longest names; the closest, `%s`, holds %s of its tokens (%s or more counts as a copy)",
			compared, f.ClosestFile, percent(f.ClosestShare), percent(fs.CopyShare)))
	}
	header := fmt.Sprintf("its first %d lines name no other project", fs.HeaderLines)
	if f.AddedWhole {
		header = "its file is new in this change; " + header
	}
	lines = append(lines, header)
	return lines
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
	lines = append(lines,
		fmt.Sprintf("counted as: %s (%s)", file.Role, language),
		fmt.Sprintf("limit: %s lines (%s)", answer.Count(file.Limit.Value), file.Limit.Source),
		"a code file by the shared rules: not documentation or data by suffix, not in a skipped folder, not binary, not generated",
	)
	if s.History.Usable > 0 {
		lines = append(lines, fmt.Sprintf("changes to it in the %s read: %d", answer.Plural(s.History.Usable, "usable commit"), s.History.Touches()[path].All))
	}
	return lines
}

func ratioEvidence(s snapshot.Snapshot, m answer.Mark) []string {
	lines := []string{
		fmt.Sprintf("test lines now: %s; at the base: %s", tally(s.Now.Files, testSideOf), tally(s.Before.Files, testSideOf)),
		fmt.Sprintf("product lines now: %s; at the base: %s", tally(s.Now.Files, productSideOf), tally(s.Before.Files, productSideOf)),
		"other code, outside the ratio: " + tallyLines(s.Now.Files, otherLinesOf),
		"test code recognized by: " + s.TestRule,
	}
	lines = append(lines, answer.TestFolderNotes(s.Layout)...)
	lines = append(lines, answer.OwnNotes(s.Layout.Own)...)
	lines = append(lines, fmt.Sprintf("limit: %s (%s)", answer.Decimal(*m.Measured.Limit), m.Measured.LimitSource))
	var tests []snapshot.File
	for _, file := range s.Now.Files {
		if _, holds := testSideOf(file); holds {
			tests = append(tests, file)
		}
	}
	measured := func(f snapshot.File) int { n, _ := testSideOf(f); return n }
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

// testSideOf and productSideOf are the lines a file adds to one side of the ratio, and whether it adds to it at all.
func testSideOf(f snapshot.File) (int, bool) {
	_, test := ratioLines(f)
	return test, test > 0 || f.Role.IsTest()
}

func productSideOf(f snapshot.File) (int, bool) {
	product, _ := ratioLines(f)
	return product, f.Role == snapshot.Product
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
