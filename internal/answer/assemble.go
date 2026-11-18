package answer

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// statusMarks is how many marks status shows (requirements, N-04).
const statusMarks = 5

// evidenceLines is how many evidence lines explain shows: its budget of 200 lines less the header, the item and
// the truncation line (requirements, N-04).
const evidenceLines = 180

// NewEvidence caps the evidence of a mark and counts what was cut.
func NewEvidence(mark string, lines []string) Evidence {
	shown := nonNil(lines[:min(len(lines), evidenceLines)])
	return Evidence{Mark: mark, Lines: shown, Omitted: len(lines) - len(shown)}
}

// repositoryEntry names what no detector owns under "not analysed".
const repositoryEntry = "repository"

// Assemble builds the answer to status or report from the detectors' findings, in the detectors' order.
func Assemble(q Question, version string, s snapshot.Snapshot, results []Result) Answer {
	a := Answer{
		Question:    q,
		Version:     version,
		Base:        Base{Asked: s.AskedBase, Commit: s.Base, MergeBase: s.MergeBase, DeclaredIn: declaredIn(s)},
		Head:        Head{Commit: s.Head, Modified: s.Modified},
		Footprint:   footprint(s.Footprint),
		Marks:       []Mark{},
		Resolved:    []Mark{},
		Touched:     []Mark{},
		Summaries:   []string{},
		ReportOnly:  []Mark{},
		Standing:    []Mark{},
		History:     History{Read: s.History.Read, Usable: s.History.Usable},
		Detectors:   []string{},
		NotAnalysed: []NotAnalysed{},
	}
	var standing []Mark
	var first []NotAnalysed
	var ran []string                  // the kinds of the rules that ran, of every detector
	acknowledged := map[string]bool{} // the table's identifiers; true once a detector returned an item with it
	for _, id := range s.Acknowledged.IDs {
		acknowledged[id] = false
	}
	for _, result := range results {
		a.Detectors = append(a.Detectors, result.Detector)
		for _, m := range result.Findings.Marks {
			if _, listed := acknowledged[m.ID]; listed {
				acknowledged[m.ID] = true
			}
			if (m.Change == New || m.Change == Grown) && reportOnly(s.ReportOnly, m) {
				a.ReportOnly = append(a.ReportOnly, m)
				continue
			}
			switch m.Change {
			case New:
				a.Counts.New++
				a.Marks = append(a.Marks, m)
			case Grown:
				a.Counts.Grown++
				a.Marks = append(a.Marks, m)
			case Resolved:
				a.Resolved = append(a.Resolved, m)
			case Touched:
				a.Touched = append(a.Touched, m)
			case Standing:
				standing = append(standing, m)
			}
		}
		a.Summaries = append(a.Summaries, result.Findings.Summaries...)
		ran = append(ran, result.Findings.RanRules...)
		if result.Findings.Radius != nil {
			a.Radius = result.Findings.Radius
		}
		for _, what := range result.Findings.NotAnalysedFirst {
			first = append(first, NotAnalysed{Detector: result.Detector, What: what})
		}
		for _, what := range result.Findings.NotAnalysed {
			a.NotAnalysed = append(a.NotAnalysed, NotAnalysed{Detector: result.Detector, What: what})
		}
		if q == Report {
			a.Sections = append(a.Sections, Section{
				Detector:   result.Detector,
				Statistics: nonNil(result.Findings.Statistics),
				Notes:      nonNil(result.Findings.Notes),
			})
		}
	}
	for _, m := range standing {
		_, listed := acknowledged[m.ID]
		switch {
		case shownAsMark(a.Marks, m.Subject):
		case listed:
			a.Acknowledged = append(a.Acknowledged, m)
		default:
			a.Standing = append(a.Standing, m)
		}
	}
	if len(a.Acknowledged) > 0 {
		a.AcknowledgedIn = s.OwnFile
	}
	a.NotAnalysed = append(a.NotAnalysed, acknowledgedNotAnalysed(s, acknowledged)...)
	a.Counts.Resolved = len(a.Resolved)
	a.Marks = Order(a.Marks)
	a.Headline = headline(s, a.Marks, a.Resolved, ran) // before status cuts the marks to five
	if q == Status && len(a.Marks) > statusMarks {
		a.More = len(a.Marks) - statusMarks
		a.Marks = a.Marks[:statusMarks]
	}
	a.NotAnalysed = nonNil(slices.Concat(first, a.NotAnalysed, repositoryNotAnalysed(s)))
	if len(s.SkippedChanges) > 0 {
		// First: it says what the footprint above leaves out.
		skipped := NotAnalysed{Detector: repositoryEntry, What: "changed files in folders plimsoll skips (generated, vendored or built), not counted: " +
			strings.Join(s.SkippedChanges, ", ")}
		a.NotAnalysed = append([]NotAnalysed{skipped}, a.NotAnalysed...)
	}
	return a
}

// headline builds the areas' states from every new and grown mark, the resolved items and the rules that ran.
// A snapshot without areas, built by a test without conventions, has no headline.
func headline(s snapshot.Snapshot, marks, resolved []Mark, ran []string) *Headline {
	if len(s.Areas) == 0 {
		return nil
	}
	h := &Headline{Areas: []HeadlineArea{}}
	for _, area := range s.Areas {
		deciding := func(kind string) bool {
			return slices.Contains(area.Kinds, kind) && !slices.Contains(s.ReportOnly, kind)
		}
		ha := HeadlineArea{Name: area.Name, State: AreaPass, Marks: []string{}, Resolved: []string{}}
		for _, m := range marks {
			if deciding(RuleKindOf(m.ID)) {
				ha.Marks = append(ha.Marks, m.ID)
			}
		}
		for _, m := range resolved {
			if deciding(RuleKindOf(m.ID)) {
				ha.Resolved = append(ha.Resolved, m.ID)
			}
		}
		slices.Sort(ha.Resolved)
		switch {
		case len(ha.Marks) > 0:
			ha.State = AreaFail
			h.Worse++
		case !slices.ContainsFunc(ran, deciding):
			ha.State = AreaNotAnalysed
		case len(ha.Resolved) > 0:
			h.Better++
		}
		h.Areas = append(h.Areas, ha)
	}
	return h
}

// reportOnly says whether a mark is of a kind status only counts: its detector and rule.
func reportOnly(kinds []string, m Mark) bool {
	return slices.Contains(kinds, DetectorOf(m.ID)+"."+RuleOf(m.ID))
}

func footprint(f snapshot.Footprint) *Footprint {
	return &Footprint{
		Commits: f.Commits, Files: f.Files,
		Product: Lines(f.Product), Test: Lines(f.Test), Docs: Lines(f.Docs),
		TestPerProduct: f.TestPerProduct,
	}
}

func shownAsMark(marks []Mark, subject Subject) bool {
	for _, m := range marks {
		if reflect.DeepEqual(m.Subject, subject) {
			return true
		}
	}
	return false
}

// repositoryNotAnalysed is what the reading itself could not see.
func repositoryNotAnalysed(s snapshot.Snapshot) []NotAnalysed {
	var entries []NotAnalysed
	add := func(what string) { entries = append(entries, NotAnalysed{Detector: repositoryEntry, What: what}) }
	for _, extension := range s.Unverified {
		add(fmt.Sprintf("not verified for `%s` (%s in %s)", extension.Suffix, Plural(extension.Lines, "line"), Plural(extension.Files, "file")))
	}
	if s.Untracked > 0 {
		add(Plural(s.Untracked, "untracked file"))
	}
	for _, path := range s.Submodules {
		add(fmt.Sprintf("submodule `%s`", path))
	}
	for _, path := range s.Unreadable {
		add(fmt.Sprintf("`%s` could not be read", path))
	}
	if len(s.Missing) > 0 {
		add(fmt.Sprintf("content not in this partial clone, so its lines and history are not counted: %s (%s)",
			Plural(len(s.Missing), "file"), QuotedList(s.Missing)))
	}
	if len(s.Unparsed) > 0 {
		add(fmt.Sprintf("the parser could not read %s, so their imports, test code and settings reads are not counted "+
			"(a file may need at most %d MiB to parse): %s", Plural(len(s.Unparsed), "file"), snapshot.ParserCapMiB,
			QuotedList(s.Unparsed)))
	}
	if len(s.SyntaxErrors) > 0 {
		verb := "hold"
		if len(s.SyntaxErrors) == 1 {
			verb = "holds"
		}
		add(fmt.Sprintf("%s %s a syntax error, so an import after the error may be missed: %s",
			Plural(len(s.SyntaxErrors), "changed file"), verb, QuotedList(s.SyntaxErrors)))
	}
	return entries
}

func nonNil[T any](list []T) []T {
	if list == nil {
		return []T{}
	}
	return list
}

// Quoted puts each name in backticks, as an answer prints names and paths.
func Quoted(names []string) []string {
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		quoted = append(quoted, "`"+n+"`")
	}
	return quoted
}

// QuotedList is names each in backticks, joined by commas: "`a`, `b`".
func QuotedList(names []string) string { return strings.Join(Quoted(names), ", ") }

// Added are the values now that before does not hold, each once, in the order of now: what a change added.
func Added(before, now []string) []string {
	seen := make(map[string]bool, len(before)+len(now))
	for _, value := range before {
		seen[value] = true
	}
	var added []string
	for _, value := range now {
		if !seen[value] {
			seen[value] = true
			added = append(added, value)
		}
	}
	return added
}

// Agree puts a count before the singular or the plural phrase: "1 file is", "3 files are".
func Agree(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return Count(float64(n)) + " " + many
}

// Plural prints a count with its noun: "1 file", "3 files".
func Plural(n int, noun string) string {
	if n == 1 {
		return Count(float64(n)) + " " + noun
	}
	return Count(float64(n)) + " " + noun + "s"
}

// declaredIn is the file that declares the base of a question that named none, or "".
func declaredIn(s snapshot.Snapshot) string {
	if s.BaseDeclared {
		return s.OwnFile
	}
	return ""
}

// acknowledgedNotAnalysed are the "not analysed" entries of the acknowledged table: its problems, then each entry
// whose identifier no detector returned for this question (stale against this base), in file order.
func acknowledgedNotAnalysed(s snapshot.Snapshot, returned map[string]bool) []NotAnalysed {
	prefix := fmt.Sprintf("`%s` `[%s]`: ", s.OwnFile, s.Acknowledged.Table)
	var entries []NotAnalysed
	for _, problem := range s.Acknowledged.Problems {
		entries = append(entries, NotAnalysed{Detector: repositoryEntry, What: prefix + problem})
	}
	for _, id := range s.Acknowledged.IDs {
		if !returned[id] {
			entries = append(entries, NotAnalysed{Detector: repositoryEntry, What: prefix + fmt.Sprintf("`%s` matches no item of this answer; it acknowledges nothing", id)})
		}
	}
	return entries
}
