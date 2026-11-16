// Package settings is the settings audit: settings no product code names, names defined twice, and compose
// variables that are not settings. Every mark is a candidate: it says what was searched and what was found.
package settings

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const name = "settings"

// The rules of the detector.
const (
	ruleNoReader     = "no-reader"
	ruleDefinedTwice = "defined-twice"
	ruleComposeOnly  = "compose-only"
)

const (
	noReaderSource     = "plimsoll's rule: a setting has a reader"
	definedTwiceSource = "plimsoll's rule: one name, one setting"
	composeOnlySource  = "the three lists agree"
)

// namedShown is how many files or names a hint or an entry names before it counts the rest.
const namedShown = 3

// Detector is the settings audit.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

// Detect compares the settings, the names in code and the compose keys on both sides.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	file := snapshot.SettingsFile
	if slices.Contains(s.Now.Broken, file) || slices.Contains(s.Before.Broken, file) {
		return answer.Findings{NotAnalysed: []string{fmt.Sprintf("`%s` could not be read, so no settings were compared", file)}}
	}
	if s.Now.Settings == nil {
		return answer.Findings{NotAnalysed: []string{fmt.Sprintf("no `%s`, so no settings were compared", file)}}
	}
	f := answer.Findings{Notes: []string{"a setting is read when a product file holds its name as a word, or declares a prefix of the name and holds the rest in lower case as a word; forms searched: as written, lower case, camel case"}}
	existed := s.Before.Settings != nil

	reads, files := computedReads(s.Now)
	if reads > 0 {
		through := "them"
		if reads == 1 {
			through = "it"
		}
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf(
			"%s with a name computed at run time in product code (%s); a setting read only through %s shows as a candidate",
			answer.Plural(reads, "read"), named(files), through))
	}
	now, before := unread(s.Now), unread(s.Before)
	f.Statistics = append(f.Statistics, count("settings without a reader in product code", "settings", len(now), len(before), existed))
	f.Marks = append(f.Marks, noReaderMarks(s, now, before, computedHint(reads, files))...)
	decided := undecided(s.Now)
	f.Statistics = append(f.Statistics, count("settings not decided", "settings", len(decided), len(undecided(s.Before)), existed))
	if len(decided) > 0 {
		f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf(
			"not decided: %s whose name without its first part occurs in product code, so the name may be built from a prefix: %s",
			answer.Plural(len(decided), "setting"), named(decided)))
	}

	twiceNow, twiceBefore := definedTwice(s.Now.Settings), definedTwice(s.Before.Settings)
	f.Statistics = append(f.Statistics, count("names defined on more than one line", "names", len(twiceNow), len(twiceBefore), existed))
	f.Marks = append(f.Marks, definedTwiceMarks(s, existed)...)

	if len(s.Now.Compose) == 0 {
		f.NotAnalysed = append(f.NotAnalysed, "no compose file tracked, so no compose variable was compared")
		return f
	}
	for _, compose := range s.Now.Compose {
		if compose.Unread > 0 {
			f.NotAnalysed = append(f.NotAnalysed, fmt.Sprintf("`%s`: %s of `environment` could not be read", compose.Path, answer.Plural(compose.Unread, "line")))
		}
	}
	onlyNow, onlyBefore := composeOnly(s.Now), composeOnly(s.Before)
	f.Statistics = append(f.Statistics, count("compose variables without a setting", "variables", len(onlyNow), len(onlyBefore), len(s.Before.Compose) > 0))
	f.Marks = append(f.Marks, composeOnlyMarks(onlyNow, onlyBefore)...)
	return f
}

func count(statName, unit string, now, before int, existed bool) answer.Statistic {
	value := float64(now)
	row := answer.Statistic{Name: statName, Value: &value, Unit: unit}
	if existed {
		b := float64(before)
		row.Before = &b
	}
	return row
}

// read says whether a product file holds a setting's name, or declares its prefix and holds the rest.
func read(use snapshot.SettingUse) bool { return len(use.Product)+len(use.Prefixed) > 0 }

// unread are the uses of a side that no product file reads and that are not built from a prefix.
func unread(t snapshot.Tree) map[string]snapshot.SettingUse {
	found := map[string]snapshot.SettingUse{}
	for _, use := range t.Uses {
		if !read(use) && use.Rest == "" {
			found[use.Name] = use
		}
	}
	return found
}

func undecided(t snapshot.Tree) []string {
	var names []string
	for _, use := range t.Uses {
		if use.Rest != "" {
			names = append(names, use.Name)
		}
	}
	return names
}

func productFiles(t snapshot.Tree) float64 {
	n := 0
	for _, file := range t.Files {
		if file.Role == snapshot.Product {
			n++
		}
	}
	return float64(n)
}

// computedHint is what a no-reader hint adds when product code computes settings names at run time; "" when it does not.
func computedHint(reads int, files []string) string {
	if reads == 0 {
		return ""
	}
	return fmt.Sprintf(" %s at run time: %s.", answer.Agree(reads, "read in product code computes its name", "reads in product code compute their name"), named(files))
}

func noReaderMarks(s snapshot.Snapshot, now, before map[string]snapshot.SettingUse, computed string) []answer.Mark {
	var marks []answer.Mark
	total := productFiles(s.Now)
	for _, settingName := range sortedNames(now, before) {
		use, holdsNow := now[settingName]
		_, holdsBefore := before[settingName]
		change, found := answer.Compare(answer.Reading{Holds: holdsBefore}, answer.Reading{Holds: holdsNow}, 1)
		if !found {
			continue
		}
		hint := "no test file holds it either; search product code for the forms as whole words."
		if len(use.Test) > 0 {
			hint = fmt.Sprintf("held by test files: %s; search product code for the forms as whole words.", named(use.Test))
		}
		m := mark(ruleNoReader, settingName, change, 0, noReaderSource, "product files that hold the name", hint+computed)
		m.Measured.Total = &total
		if holdsBefore {
			zero := 0.0
			m.Measured.Before = &zero
		}
		marks = append(marks, m)
	}
	return marks
}

// definedTwice counts the lines that define each name and are not commented out, for names on two or more.
func definedTwice(settings *snapshot.Settings) map[string]int {
	found := map[string]int{}
	if settings == nil {
		return found
	}
	for _, n := range settings.Active {
		found[n]++
	}
	for n, lines := range found {
		if lines < 2 {
			delete(found, n)
		}
	}
	return found
}

func activeLines(settings *snapshot.Settings, settingName string) int {
	if settings == nil {
		return 0
	}
	n := 0
	for _, active := range settings.Active {
		if active == settingName {
			n++
		}
	}
	return n
}

func definedTwiceMarks(s snapshot.Snapshot, existed bool) []answer.Mark {
	var marks []answer.Mark
	for _, settingName := range sortedNames(definedTwice(s.Now.Settings), definedTwice(s.Before.Settings)) {
		now, before := activeLines(s.Now.Settings, settingName), activeLines(s.Before.Settings, settingName)
		change, found := answer.Compare(answer.Reading{Holds: before >= 2, Value: float64(before)}, answer.Reading{Holds: now >= 2, Value: float64(now)}, 1)
		if !found {
			continue
		}
		m := mark(ruleDefinedTwice, settingName, change, float64(now), definedTwiceSource, "lines that define the name",
			fmt.Sprintf("search `%s` for lines that start with the name and are not commented out.", snapshot.SettingsFile))
		if existed {
			b := float64(before)
			m.Measured.Before = &b
		}
		marks = append(marks, m)
	}
	return marks
}

// composeOnly maps each compose key that is not a setting of the side to the compose files that set it.
func composeOnly(t snapshot.Tree) map[string][]string {
	var names []string
	if t.Settings != nil {
		names = t.Settings.Names
	}
	found := map[string][]string{}
	for _, compose := range t.Compose {
		for _, key := range compose.Keys {
			if !slices.Contains(names, key) {
				found[key] = append(found[key], compose.Path)
			}
		}
	}
	return found
}

func composeOnlyMarks(now, before map[string][]string) []answer.Mark {
	var marks []answer.Mark
	for _, key := range sortedNames(now, before) {
		files, holdsNow := now[key]
		_, holdsBefore := before[key]
		change, found := answer.Compare(answer.Reading{Holds: holdsBefore}, answer.Reading{Holds: holdsNow}, 1)
		if !found {
			continue
		}
		where := named(files)
		if !holdsNow {
			where = named(before[key]) + " at the base"
		}
		m := mark(ruleComposeOnly, key, change, float64(len(files)), composeOnlySource, "compose files that set the name",
			fmt.Sprintf("set under `environment` in %s; `%s` has no line for it.", where, snapshot.SettingsFile))
		if holdsBefore {
			b := float64(len(before[key]))
			m.Measured.Before = &b
		}
		marks = append(marks, m)
	}
	return marks
}

func mark(rule, settingName string, change answer.Change, value float64, source, unit, hint string) answer.Mark {
	return answer.Mark{
		ID:         answer.ID(name, rule, settingName),
		Change:     change,
		Subject:    answer.Subject{Kind: "setting", Names: []string{settingName}},
		Confidence: answer.Candidate,
		Measured:   answer.Measured{Value: value, LimitSource: source, Unit: unit},
		Hint:       hint,
	}
}

// computedReads counts a side's settings reads whose name is computed at run time, and the product files that hold
// them, sorted.
func computedReads(t snapshot.Tree) (reads int, files []string) {
	for _, read := range t.Dynamic {
		reads, files = reads+read.Count, append(files, read.Path)
	}
	return reads, files
}

// named lists up to three names in backticks, then counts the rest.
func named(items []string) string {
	text := strings.Join(answer.Quoted(items[:min(len(items), namedShown)]), ", ")
	if len(items) > namedShown {
		text += fmt.Sprintf(" and %d more", len(items)-namedShown)
	}
	return text
}

func sortedNames[V any](now, before map[string]V) []string {
	var names []string
	for n := range now {
		names = append(names, n)
	}
	for n := range before {
		if _, found := now[n]; !found {
			names = append(names, n)
		}
	}
	slices.Sort(names)
	return names
}
