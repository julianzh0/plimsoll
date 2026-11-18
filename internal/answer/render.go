package answer

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// Caps of the status text (requirements, N-04).
const (
	statusResolved    = 3
	statusNotAnalysed = 5
)

// Caps of the report's standing list: items of one rule listed before the rest are counted, and folders named in
// that count. A rule can stand for hundreds of files (664 planning records on one repository), which would bury the
// other items.
const (
	reportStanding = 5
	reportFolders  = 3
	groupLevels    = 2
)

// commitShown is how many characters of a commit name the text shows; git's own abbreviation depends on the
// object count and could differ between two clones.
const commitShown = 10

// Render writes the text of an answer from the answer alone and the detectors' phrases.
func Render(a Answer, w Wording) string {
	var text strings.Builder
	header(&text, a)
	text.WriteString("\n")
	if a.Question == Explain {
		explain(&text, a, w)
		return text.String()
	}
	marks(&text, a, w)
	if a.Question == Report {
		report(&text, a, w)
	} else {
		status(&text, a, w)
	}
	return text.String()
}

func header(text *strings.Builder, a Answer) {
	var about []string
	if a.Base.DeclaredIn != "" {
		about = append(about, fmt.Sprintf("declared in `%s`", a.Base.DeclaredIn))
	}
	if a.Base.MergeBase {
		about = append(about, "merge base with the head")
	}
	base := fmt.Sprintf("base `%s` %s", a.Base.Asked, Short(a.Base.Commit))
	if len(about) > 0 {
		base = fmt.Sprintf("base `%s` (%s) %s", a.Base.Asked, strings.Join(about, "; "), Short(a.Base.Commit))
	}
	modified := ""
	if a.Head.Modified {
		modified = " (working tree modified)"
	}
	fmt.Fprintf(text, "plimsoll %s %s: %s, head %s%s\n", a.Version, a.Question, base, Short(a.Head.Commit), modified)
	if a.Question != Explain {
		counts := fmt.Sprintf("%d new, %d grown, %d resolved", a.Counts.New, a.Counts.Grown, a.Counts.Resolved)
		if len(a.ReportOnly) > 0 {
			counts += fmt.Sprintf("; %d more in `report` only", len(a.ReportOnly))
		}
		text.WriteString(counts + "\n")
	}
	if f := a.Footprint; f != nil {
		ratio := "no product line added"
		if f.TestPerProduct != nil {
			ratio = Decimal(*f.TestPerProduct) + " test lines added per product line added"
		}
		fmt.Fprintf(text, "Footprint of the change since the base: %s, %s; product %s, test %s, documentation %s; %s.\n",
			Plural(f.Commits, "commit"), Plural(f.Files, "file"), lines(f.Product), lines(f.Test), lines(f.Docs), ratio)
	}
}

func marks(text *strings.Builder, a Answer, w Wording) {
	if len(a.Marks) == 0 {
		commits, files := 0, 0
		if a.Footprint != nil {
			commits, files = a.Footprint.Commits, a.Footprint.Files
		}
		if a.Question == Status && len(a.ReportOnly)+len(a.Touched)+len(a.Summaries) > 0 {
			fmt.Fprintf(text, "No new or grown mark to list since `%s` (%s, %s compared); the lines below count what else the change did, and `report` lists it.\n",
				a.Base.Asked, Plural(commits, "commit"), Plural(files, "file"))
		} else {
			fmt.Fprintf(text, "No new or grown marks since `%s` (%s, %s compared).\n", a.Base.Asked, Plural(commits, "commit"), Plural(files, "file"))
		}
		if a.Question == Report {
			text.WriteString("\n")
		}
		return
	}
	for i, m := range a.Marks {
		fmt.Fprintf(text, "%d. [%s, %s] %s\n   verify: %s id: %s\n", i+1, m.Change, m.Confidence, sentence(w, m), m.Hint, m.ID)
	}
	if a.More > 0 {
		fmt.Fprintf(text, "%d more: `report` lists them.\n", a.More)
	}
	text.WriteString("\n")
}

func status(text *strings.Builder, a Answer, w Wording) {
	if len(a.Resolved) > 0 {
		ids := make([]string, 0, len(a.Resolved))
		for _, m := range a.Resolved {
			ids = append(ids, m.ID)
		}
		fmt.Fprintf(text, "Resolved: %s.\n", capped(ids, statusResolved))
	}
	if len(a.ReportOnly) > 0 {
		fmt.Fprintf(text, "In `report` only (not counted here): %s.\n", byDetector(a.ReportOnly, a, w, "; "))
	}
	if len(a.Touched) > 0 {
		fmt.Fprintf(text, "Touched but not created by this change (`report` lists them): %s.\n", byDetector(a.Touched, a, w, "; "))
	}
	for _, summary := range a.Summaries {
		text.WriteString(summary + "\n")
	}
	fmt.Fprintf(text, "Standing (already at the base, not grown): %s.\n", standingLine(a, w))
	trailer(text, a)
	whats := make([]string, 0, len(a.NotAnalysed))
	for _, entry := range a.NotAnalysed {
		whats = append(whats, entry.What)
	}
	if len(whats) == 0 {
		text.WriteString("Not analysed: none.\n")
		return
	}
	fmt.Fprintf(text, "Not analysed: %s.\n", capped(whats, statusNotAnalysed))
}

func report(text *strings.Builder, a Answer, w Wording) {
	if len(a.Resolved) == 0 {
		text.WriteString("Resolved: none.\n")
	} else {
		text.WriteString("Resolved:\n")
		for _, m := range a.Resolved {
			fmt.Fprintf(text, "- %s\n  verify: %s id: %s\n", sentence(w, m), m.Hint, m.ID)
		}
	}
	if len(a.ReportOnly) > 0 {
		text.WriteString("In `report` only (not counted in `status`):\n")
		for _, m := range a.ReportOnly {
			fmt.Fprintf(text, "- %s\n  verify: %s id: %s\n", sentence(w, m), m.Hint, m.ID)
		}
	}
	if len(a.Touched) == 0 {
		text.WriteString("Touched but not created by this change: none.\n")
	} else {
		text.WriteString("Touched but not created by this change:\n")
		for _, m := range a.Touched {
			fmt.Fprintf(text, "- %s\n  verify: %s id: %s\n", sentence(w, m), m.Hint, m.ID)
		}
	}
	text.WriteString("\n")
	for _, section := range a.Sections {
		text.WriteString(section.Detector + "\n")
		for _, s := range section.Statistics {
			text.WriteString(statistic(s) + "\n")
		}
		for _, note := range section.Notes {
			text.WriteString("  " + note + "\n")
		}
		text.WriteString("\n")
	}
	if len(a.Standing) == 0 {
		text.WriteString("Standing (already at the base, not grown): none.\n")
	} else {
		text.WriteString("Standing (already at the base, not grown):\n")
		standingList(text, a.Standing, w)
	}
	trailer(text, a)
	if len(a.NotAnalysed) == 0 {
		text.WriteString("Not analysed: none.\n")
		return
	}
	text.WriteString("Not analysed:\n")
	for _, entry := range a.NotAnalysed {
		fmt.Fprintf(text, "- %s: %s\n", entry.Detector, entry.What)
	}
}

func explain(text *strings.Builder, a Answer, w Wording) {
	for _, m := range slices.Concat(a.Marks, a.ReportOnly, a.Resolved, a.Touched, a.Standing) {
		fmt.Fprintf(text, "[%s, %s] %s\n   verify: %s id: %s\n", m.Change, m.Confidence, sentence(w, m), m.Hint, m.ID)
	}
	if a.Evidence == nil {
		return
	}
	text.WriteString("\nEvidence:\n")
	for _, line := range a.Evidence.Lines {
		text.WriteString("- " + line + "\n")
	}
	if a.Evidence.Omitted > 0 {
		fmt.Fprintf(text, "... %d more evidence lines not shown.\n", a.Evidence.Omitted)
	}
}

func trailer(text *strings.Builder, a Answer) {
	fmt.Fprintf(text, "History: %s read, %s usable.\n", Plural(a.History.Read, "commit"), Count(float64(a.History.Usable)))
	fmt.Fprintf(text, "Detectors run: %s.\n", strings.Join(a.Detectors, ", "))
}

func standingLine(a Answer, w Wording) string {
	if line := byDetector(a.Standing, a, w, "; "); line != "" {
		return line
	}
	return "none"
}

// byDetector sums items up in each detector's own words, in the detectors' order, or is "" when there are none.
func byDetector(items []Mark, a Answer, w Wording, separator string) string {
	var fragments []string
	for _, detector := range a.Detectors {
		var own []Mark
		for _, m := range items {
			if DetectorOf(m.ID) == detector {
				own = append(own, m)
			}
		}
		if phrases, found := w[detector]; found && len(own) > 0 {
			fragments = append(fragments, phrases.Standing(own))
		}
	}
	return strings.Join(fragments, separator)
}

func statistic(s Statistic) string {
	label := "  " + s.Name
	if s.Subject != "" {
		label += " `" + s.Subject + "`"
	}
	if s.Value == nil {
		return label + ": " + s.Note
	}
	value, difference := number(s.Unit, *s.Value), "not at the base"
	if s.Unit != "" && s.Unit != ratioUnit {
		unit := s.Unit
		if *s.Value == 1 {
			unit = strings.TrimSuffix(unit, "s")
		}
		value += " " + unit
	}
	if s.Before != nil {
		difference = "net " + signed(number(s.Unit, *s.Value-*s.Before), *s.Value-*s.Before)
	}
	text := fmt.Sprintf("%s: %s (%s)", label, value, difference)
	if s.Note != "" {
		text += "; " + s.Note
	}
	return text
}

// ratioUnit marks a statistic that prints with one decimal and without its unit.
const ratioUnit = "ratio"

func number(unit string, value float64) string {
	if unit == ratioUnit {
		return Decimal(value)
	}
	return Count(value)
}

func signed(printed string, value float64) string {
	if value < 0 {
		return printed
	}
	return "+" + printed
}

func sentence(w Wording, m Mark) string {
	if phrases, found := w[DetectorOf(m.ID)]; found {
		return phrases.Mark(m)
	}
	return m.ID
}

func lines(l Lines) string {
	return fmt.Sprintf("+%s -%s", Count(float64(l.Added)), Count(float64(l.Removed)))
}

// Short is a commit name as the text shows it.
func Short(commit string) string {
	return commit[:min(len(commit), commitShown)]
}

// capped joins at most limit items and names the rest.
func capped(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("%s and %d more in `report`", strings.Join(items[:limit], "; "), len(items)-limit)
}

// standingList writes the standing items, up to reportStanding of one rule; the rest of that rule become one line,
// counted by folder, after its last listed item. The answer itself keeps every mark.
func standingList(text *strings.Builder, standing []Mark, w Wording) {
	ruleOf := func(m Mark) string { prefix, _, _ := strings.Cut(m.ID, ":"); return prefix }
	listed, rest := map[string]int{}, map[string][]Mark{}
	for _, m := range standing {
		if rule := ruleOf(m); listed[rule] < reportStanding {
			listed[rule]++
		} else {
			rest[rule] = append(rest[rule], m)
		}
	}
	written := map[string]int{}
	for _, m := range standing {
		rule := ruleOf(m)
		if written[rule] == reportStanding {
			continue
		}
		written[rule]++
		fmt.Fprintf(text, "- %s id: %s\n", sentence(w, m), m.ID)
		if written[rule] == reportStanding && len(rest[rule]) > 0 {
			fmt.Fprintf(text, "- %d more `%s` items%s\n", len(rest[rule]), rule, inFolders(rest[rule]))
		}
	}
}

// inFolders counts items by the first two folders of their first path, the largest first: ", in `src/` 3,
// `docs/plans/` 1"; "" when they have no path. Two levels keep a tree of records per story as one count.
func inFolders(items []Mark) string {
	counts := map[string]int{}
	for _, m := range items {
		if len(m.Subject.Paths) == 0 {
			continue
		}
		folders := strings.Split(path.Dir(m.Subject.Paths[0]), "/")
		folder := "`" + strings.Join(folders[:min(len(folders), groupLevels)], "/") + "/`"
		if folder == "`./`" {
			folder = "the root"
		}
		counts[folder]++
	}
	if len(counts) == 0 {
		return ""
	}
	folders := slices.SortedFunc(maps.Keys(counts), func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(a, b)) })
	parts := make([]string, 0, reportFolders+1)
	others := 0
	for i, folder := range folders {
		if i < reportFolders {
			parts = append(parts, fmt.Sprintf("%s %d", folder, counts[folder]))
		} else {
			others += counts[folder]
		}
	}
	if others > 0 {
		parts = append(parts, fmt.Sprintf("%d in other folders", others))
	}
	return ", in " + strings.Join(parts, ", ")
}
