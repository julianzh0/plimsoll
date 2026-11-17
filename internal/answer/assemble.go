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
		ReportOnly:  []Mark{},
		Standing:    []Mark{},
		History:     History{Read: s.History.Read, Usable: s.History.Usable},
		Detectors:   []string{},
		NotAnalysed: []NotAnalysed{},
	}
	remarks := map[string][]string{}
	for _, result := range results {
		for _, remark := range result.Findings.Remarks {
			remarks[remark.Mark] = append(remarks[remark.Mark], remark.Text)
		}
	}
	var standing []Mark
	for _, result := range results {
		a.Detectors = append(a.Detectors, result.Detector)
		for _, m := range result.Findings.Marks {
			for _, text := range remarks[m.ID] {
				m.Hint += " " + text
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
		if !shownAsMark(a.Marks, m.Subject) {
			a.Standing = append(a.Standing, m)
		}
	}
	a.Counts.Resolved = len(a.Resolved)
	backed := map[string]bool{}
	for id := range remarks {
		backed[id] = true
	}
	a.Marks = Order(a.Marks, backed)
	if q == Status && len(a.Marks) > statusMarks {
		a.More = len(a.Marks) - statusMarks
		a.hidden = a.Marks[statusMarks:]
		a.Marks = a.Marks[:statusMarks]
	}
	a.NotAnalysed = append(a.NotAnalysed, repositoryNotAnalysed(s)...)
	return a
}

// reportOnly says whether a mark is of a kind status only counts: its detector and rule, with or without its change.
func reportOnly(kinds []string, m Mark) bool {
	kind := DetectorOf(m.ID) + "." + RuleOf(m.ID)
	return slices.Contains(kinds, kind) || slices.Contains(kinds, kind+" "+string(m.Change))
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
	var added []string
	for _, value := range now {
		if !slices.Contains(before, value) && !slices.Contains(added, value) {
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
