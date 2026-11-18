// Package links is the link detector: what imports between the repository's own files show about a change.
package links

import (
	"fmt"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/answer"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

const name = "links"

const (
	linksNote = "links: imports read from the code; an import is a link only when it names exactly one tracked file " +
		"of the repository, or the folders of one package where a language imports folders; outside packages are not compared with local names"
	folderNote = "an import of a folder reaches the folder's files, and a package that lies in several folders is reached in each; " +
		"use between files of one folder or one package needs no import and is not seen"
	ringsNote = "rings: among imports that are not type-only"
	// folderCyclesNote says what the folder-cycle rule compares.
	folderCyclesNote = "folder cycles: between the folders directly under one folder, at every depth; " +
		"a folder depends on another when a product file under it imports a product file under the other; " +
		"type-only imports count; an import between two files of one import ring counts for the ring only"
)

// Detector is the link detector.
type Detector struct{}

// Name is the prefix of the detector's identifiers.
func (Detector) Name() string { return name }

// Detect decides which languages' links can be used, then gives their numbers.
func (Detector) Detect(s snapshot.Snapshot) answer.Findings {
	languages, notAnalysed := analysed(s)
	if len(s.Now.Links.Languages) == 0 {
		notAnalysed = []string{"no file of a language whose imports are read"}
	}
	f := answer.Findings{NotAnalysed: notAnalysed}
	now, before := graphsOf(s, languages)
	if len(languages) > 0 {
		f = withLinks(f, s, languages, now, before)
	}
	f = withRules(f, s, now, before)
	if rows, summary := additionRows(s.Additions); len(rows) > 0 {
		f.Statistics = append(f.Statistics, rows...)
		f.Summaries = append(f.Summaries, summary)
		at := slices.Index(f.Notes, linksNote) + 1
		f.Notes = slices.Insert(f.Notes, at, additionsNote)
	}
	f.Statistics = append(f.Statistics, namedRows(s, now, before)...)
	f.NotAnalysed = append(f.NotAnalysed, answer.RootsNotUsed(s.Layout)...)
	return f
}

// withLinks adds what the links of the analysed languages show: the rows, the marks and the notes.
func withLinks(f answer.Findings, s snapshot.Snapshot, languages []snapshot.LanguageImports, now, before graph) answer.Findings {
	f.Notes = append(f.Notes, linksNote, reachNote)
	if slices.ContainsFunc(languages, func(l snapshot.LanguageImports) bool { return l.Folders }) {
		f.Notes = append(f.Notes, folderNote)
	}
	for _, now := range languages {
		before, atBase := languageAt(s.Before.Links, now.Language)
		f.Statistics = append(f.Statistics, followedRow(now, before, atBase))
	}
	f.Marks = testImportMarks(s, now, before)
	f.Statistics = append(f.Statistics, reachRows(s, now)...)
	nowRings, baseRings := now.rings(false), before.rings(false)
	if slices.ContainsFunc(languages, func(l snapshot.LanguageImports) bool { return l.NoRings == "" }) {
		basePath := func(file string) []string { return []string{s.BasePath(file)} }
		ringCreated := func(ring []string) bool { return added(now.ringLinks(ring)) }
		f.Marks = append(f.Marks, withClosingImports(cycleMarks(ruleCycle, nowRings, baseRings, basePath, ringCreated), now)...)
		f.Statistics = append(f.Statistics, ringRows(nowRings, baseRings, now.rings(true), before.rings(true))...)
		f.Notes = append(f.Notes, ringsNote)
	}
	nowImports := now.siblingImports(nowRings)
	nowFolders, baseFolders := folderCycles(nowImports), folderCycles(before.siblingImports(baseRings))
	folderCreated := func(cycle []string) bool {
		return slices.ContainsFunc(nowImports, func(i siblingImport) bool {
			return i.link.Added && slices.Contains(cycle, i.from) && slices.Contains(cycle, i.to)
		})
	}
	f.Marks = append(f.Marks, cycleMarks(ruleFolderCycle, nowFolders, baseFolders, folderAtBase(s), folderCreated)...)
	f.Statistics = append(f.Statistics, folderRow(nowFolders, baseFolders, len(nowImports)))
	f.Notes = append(f.Notes, folderCyclesNote)
	f.Notes = append(f.Notes, noRingsNotes(languages)...)
	f.Notes = append(f.Notes, entryNotes(languages)...)
	f.Notes = append(f.Notes, answer.RootNotes(s.Layout)...)
	f.Statistics = append(f.Statistics, mostImportsRows(s, now, before)...)
	f.Statistics = append(f.Statistics, newFileRows(s, now, before)...)
	return f
}

// analysed gives the languages with code files now whose links are used, in the entries' order, and the "not
// analysed" sentences: a language that cannot tell its own imports from outside ones, one with too many own imports
// not followed on either side, and a prefix file that was read only in part.
func analysed(s snapshot.Snapshot) ([]snapshot.LanguageImports, []string) {
	limit := s.Limits.ImportsNotFollowed
	var languages []snapshot.LanguageImports
	var notAnalysed []string
	for _, now := range s.Now.Links.Languages {
		before, atBase := languageAt(s.Before.Links, now.Language)
		if blocked := prefixBlocks(now, before, atBase); blocked != "" {
			notAnalysed = append(notAnalysed, blocked)
			continue
		}
		if sentence := overLimit(now, "now", limit.Value, limit.Source); sentence != "" {
			notAnalysed = append(notAnalysed, sentence)
			continue
		}
		if sentence := overLimit(before, "at the base", limit.Value, limit.Source); atBase && sentence != "" {
			notAnalysed = append(notAnalysed, sentence)
			continue
		}
		if partial := prefixPartly(now); partial != "" {
			notAnalysed = append(notAnalysed, partial)
		}
		languages = append(languages, now)
	}
	return languages, notAnalysed
}

// languageAt finds a language's counts on one side.
func languageAt(links snapshot.Links, language string) (snapshot.LanguageImports, bool) {
	for _, l := range links.Languages {
		if l.Language == language {
			return l, true
		}
	}
	return snapshot.LanguageImports{}, false
}

// prefixBlocks is the sentence for a language that needs its prefix to tell its own imports, when the prefix file is
// missing or unreadable, or no package of its own is under its roots, on a side where it has files: every own import
// would read as outside.
func prefixBlocks(now, before snapshot.LanguageImports, atBase bool) string {
	if !now.PrefixRequired {
		return ""
	}
	state, when := now.Prefix, ""
	if state != snapshot.PrefixAbsent && state != snapshot.PrefixUnreadable && atBase {
		state, when = before.Prefix, " at the base"
	}
	missing := fmt.Sprintf("`%s` at the root", now.PrefixFile)
	switch {
	case len(now.Roots) > 0:
		missing = "package of its own " + places(now.Roots, "or")
	case now.PrefixFile == "": // a language whose files declare its packages, such as namespaces
		missing = "package of its own declared by its files"
	}
	switch state {
	case snapshot.PrefixAbsent:
		return fmt.Sprintf("`%s`: no %s%s, so its own imports cannot be told from outside ones; no link numbers for its files", now.Language, missing, when)
	case snapshot.PrefixUnreadable:
		return fmt.Sprintf("`%s`: `%s` could not be read%s, so its own imports cannot be told from outside ones; no link numbers for its files", now.Language, now.PrefixFile, when)
	case snapshot.PrefixRead, snapshot.PrefixExtends:
	}
	return ""
}

// prefixPartly is the sentence for a prefix file whose aliases were read only in part; the numbers stay.
func prefixPartly(now snapshot.LanguageImports) string {
	switch now.Prefix {
	case snapshot.PrefixExtends:
		return fmt.Sprintf("`%s`: `%s` extends another file; aliases declared there count as outside", now.Language, now.PrefixFile)
	case snapshot.PrefixUnreadable:
		return fmt.Sprintf("`%s`: `%s` could not be read; its aliases count as outside", now.Language, now.PrefixFile)
	case snapshot.PrefixRead, snapshot.PrefixAbsent:
	}
	return ""
}

// overLimit is the sentence for a side whose share of own imports not followed is over the limit; exactly at the
// limit is not over, and no own import is a share of 0.
func overLimit(l snapshot.LanguageImports, when string, limit float64, source string) string {
	own, notFollowed := l.Own(), l.NotFollowed()
	if own == 0 || float64(notFollowed) <= limit*float64(own) {
		return ""
	}
	return fmt.Sprintf("`%s`: %d of %d own imports could not be followed %s (%d ambiguous, %d not readable, %d not found), "+
		"over the limit of %s%% (%s); no link numbers for its files",
		l.Language, notFollowed, own, when, l.Ambiguous, l.Unreadable, l.NotFound, answer.Count(limit*100), source)
}

// followedRow is a language's resolved imports, now and at the base, with its kinds in the note.
func followedRow(now, before snapshot.LanguageImports, atBase bool) answer.Statistic {
	value := float64(now.Resolved)
	row := answer.Statistic{
		Name: "imports followed", Subject: now.Language, Value: &value, Unit: "imports",
		Note: fmt.Sprintf("of %s own: %s ambiguous, %s not readable, %s not found; %s outside", count(now.Own()), count(now.Ambiguous),
			count(now.Unreadable), count(now.NotFound), count(now.Outside)),
	}
	if atBase {
		b := float64(before.Resolved)
		row.Before = &b
	}
	return row
}

func count(n int) string { return answer.Count(float64(n)) }

// noRingsNotes say, per analysed language whose entry gives a reason, why rings are not looked for among its files.
func noRingsNotes(languages []snapshot.LanguageImports) []string {
	var notes []string
	for _, l := range languages {
		if l.NoRings != "" {
			notes = append(notes, fmt.Sprintf("`%s`: import rings are not looked for: %s", l.Language, l.NoRings))
		}
	}
	return notes
}

// entryNotes say, per analysed language, where its own packages are looked for and which files run without an
// import of them.
func entryNotes(languages []snapshot.LanguageImports) []string {
	var notes []string
	for _, l := range languages {
		if len(l.Roots) > 0 {
			notes = append(notes, fmt.Sprintf("`%s`: its own packages are looked for %s; an import of a package elsewhere counts as outside",
				l.Language, places(l.Roots, "and")))
		}
	}
	for _, l := range languages {
		if l.SourceSets {
			notes = append(notes, fmt.Sprintf("`%s`: a package's folders under a test folder directly in `src/` are reached only from files under such a folder", l.Language))
		}
	}
	for _, l := range languages {
		if l.NotSeen != "" {
			notes = append(notes, fmt.Sprintf("`%s`: not seen: %s", l.Language, l.NotSeen))
		}
	}
	for _, l := range languages {
		if len(l.Implicit) > 0 {
			notes = append(notes, fmt.Sprintf("`%s`: %s run without an import of them; a new one is not listed as a file without an importer",
				l.Language, joined(answer.Quoted(l.Implicit), "and")))
		}
	}
	return notes
}

// places names folders as a sentence does: "." is "at the root", any other "under `<folder>/`".
func places(folders []string, conjunction string) string {
	var parts []string
	for _, folder := range folders {
		if folder == "." {
			parts = append(parts, "at the root")
		} else {
			parts = append(parts, "under "+answer.Folder(folder))
		}
	}
	return joined(parts, conjunction)
}

// joined lists parts with commas and the conjunction before the last.
func joined(parts []string, conjunction string) string {
	if len(parts) < 2 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " " + conjunction + " " + parts[len(parts)-1]
}
