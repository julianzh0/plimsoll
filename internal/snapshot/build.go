package snapshot

import (
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// Build classifies both sides of a source by the rules. It reads nothing and cannot fail. What each code file's content
// says comes from readings, which keep it between questions.
func Build(source Source, rules conventions.Rules, readings *Readings) Snapshot {
	changed := map[string]bool{}
	for _, change := range source.Changes {
		changed[change.Path] = true
		if change.From != "" {
			changed[change.From] = true
		}
	}
	beforeEntries := unchangedFromNow(source.Before, source.Now, changed)
	nowContent, beforeContent := contentOf(source.Now), contentOf(beforeEntries)
	ownNow, ownBefore := readOwnFile(rules, nowContent), readOwnFile(rules, beforeContent)
	tracked := trackedPaths(nowContent, beforeContent)
	layout := ownLayout(declaredLayout(rules, nowContent, beforeContent, tracked), rules, ownNow, tracked)
	rules = withLayout(rules, layout)
	rules = withSharedSuffixes(rules, source.Now)
	limits, declaredLimits := declaredLimits(rules, ownNow, ownBefore)
	rules.Limits = limits
	now := tree(rules, source.Now, nil)
	nowReadings := readings.side(rules, nowContent, now.Files)
	withTestLines(now.Files, nowReadings)
	unchanged := map[string]File{}
	for _, file := range now.Files {
		if !changed[file.Path] {
			unchanged[file.Path] = file
		}
	}
	before := tree(rules, beforeEntries, unchanged)
	beforeReadings := readings.side(rules, beforeContent, before.Files)
	withTestLines(before.Files, beforeReadings)
	unreadable := slices.Clone(source.Unreadable)
	now.Settings, before.Settings = settings(source.Now), settings(beforeEntries)
	var unreadableNow []string
	now.Manifests, unreadableNow = manifests(rules, source.Now)
	before.Manifests, before.Broken = manifests(rules, beforeEntries)
	if now.Settings == nil && tracks(source.Now, SettingsFile) {
		unreadableNow = append(unreadableNow, SettingsFile)
	}
	if before.Settings == nil && tracks(beforeEntries, SettingsFile) {
		before.Broken = append(before.Broken, SettingsFile)
	}
	now.Broken = unreadableNow
	now.Planning, before.Planning = planningFiles(rules, now.Paths), planningFiles(rules, before.Paths)
	now.Compose, before.Compose = composeFiles(rules, source.Now), composeFiles(rules, beforeEntries)
	before.Links = links(rules, beforeContent, before, beforeReadings, nil)
	now.Links = links(rules, nowContent, now, nowReadings, baseImportsOf(source.Changes, before, beforeReadings))
	unreadable = append(unreadable, unreadableNow...)
	slices.Sort(unreadable)
	return Snapshot{
		AskedBase:           source.AskedBase,
		Base:                source.Base,
		Head:                source.Head,
		MergeBase:           source.MergeBase,
		Modified:            source.Modified,
		Limits:              rules.Limits,
		ReportOnly:          rules.ReportOnly,
		BaseDeclared:        source.BaseDeclared,
		OwnFile:             rules.OwnFile,
		DeclaredLimits:      declaredLimits,
		Declarations:        declaredValues(rules, ownNow, ownBefore),
		OwnFileProblems:     ownNow.problems,
		OwnFileUnreadAtBase: ownBefore.unreadable,
		Before:              before,
		Now:                 now,
		Changes:             source.Changes,
		Footprint:           footprint(rules, source.Changes, len(source.Since), now, before),
		History:             history(rules, source.Commits, source.Since),
		Layout:              layout,
		TestRule:            rules.TestRule(),
		PlanningRule:        rules.PlanningRule(),
		Unverified:          unverified(now.Files),
		Untracked:           source.Untracked,
		Submodules:          source.Submodules,
		Unreadable:          slices.Compact(unreadable),
		Missing:             source.Missing,
		Unparsed:            unparsed(nowReadings, beforeReadings),
		Additions:           additions(rules, source.Changes, now, before, source.Now, nowReadings, beforeReadings),
		SyntaxErrors:        syntaxErrors(changed, nowReadings, beforeReadings),
	}
}

// syntaxErrors are the changed code files whose reading on either side met a syntax error, sorted, each once. A file
// the change did not touch loses the same statements on both sides, so its links compare alike and it is not named.
func syntaxErrors(changed map[string]bool, sides ...map[string]reading) []string {
	var found []string
	for _, readings := range sides {
		for path, r := range readings {
			if r.syntaxError && changed[path] {
				found = append(found, path)
			}
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// unparsed are the code files, on either side, that the parser could not read, sorted, each once.
func unparsed(sides ...map[string]reading) []string {
	var found []string
	for _, readings := range sides {
		for path, r := range readings {
			if r.unread {
				found = append(found, path)
			}
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// planningFiles are the paths that lie under a planning or review path.
func planningFiles(rules conventions.Rules, paths []string) []string {
	var found []string
	for _, p := range paths {
		if onPaths(rules, p, rules.PlanningPaths) {
			found = append(found, p)
		}
	}
	return found
}

// tracks says whether a side tracks a path, read or not.
func tracks(entries []Entry, path string) bool {
	return slices.ContainsFunc(entries, func(e Entry) bool { return e.Path == path })
}

// unchangedFromNow gives every base entry that no change names the content it has now: it is the same file.
func unchangedFromNow(before, now []Entry, changed map[string]bool) []Entry {
	current := map[string]Entry{}
	for _, entry := range now {
		current[entry.Path] = entry
	}
	entries := make([]Entry, 0, len(before))
	for _, entry := range before {
		if same, found := current[entry.Path]; found && !changed[entry.Path] {
			entry = same
		}
		entries = append(entries, entry)
	}
	return entries
}

// tree classifies one side. A file in known is already classified, so it is taken as it is.
func tree(rules conventions.Rules, entries []Entry, known map[string]File) Tree {
	var side Tree
	for _, entry := range entries {
		side.Paths = append(side.Paths, entry.Path)
		if file, found := known[entry.Path]; found {
			side.Files = append(side.Files, file)
		} else if file, isCode := codeFile(rules, entry); isCode {
			side.Files = append(side.Files, file)
		}
	}
	slices.Sort(side.Paths)
	slices.SortFunc(side.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return side
}

func codeFile(rules conventions.Rules, entry Entry) (File, bool) {
	if entry.Binary || entry.Content == nil || excluded(rules, entry.Path) || onPaths(rules, entry.Path, rules.GeneratedNames) || generatedOrBinary(rules, entry.Content) {
		return File{}, false
	}
	file := File{Path: entry.Path, Role: role(rules, entry.Path, entry.Content), Lines: countLines(entry.Content), Limit: rules.Limits.FileLines}
	file.Record = onPaths(rules, entry.Path, rules.PlanningPaths) || onPaths(rules, entry.Path, rules.MeasurementPaths)
	if language, found := rules.LanguageOf(entry.Path); found {
		file.Language = language.Name
	}
	return file, true
}

// withTestLines gives each product file the test lines its reading counts.
func withTestLines(files []File, readings map[string]reading) {
	for i, file := range files {
		if file.Role == Product {
			files[i].TestLines = readings[file.Path].testLines
		}
	}
}

func unverified(files []File) []Extension {
	var extensions []Extension
	for _, file := range files {
		if file.Language != "" {
			continue
		}
		suffix := conventions.Suffix(file.Path)
		index := slices.IndexFunc(extensions, func(e Extension) bool { return e.Suffix == suffix })
		if index < 0 {
			extensions = append(extensions, Extension{Suffix: suffix})
			index = len(extensions) - 1
		}
		extensions[index].Files++
		extensions[index].Lines += file.Lines
	}
	slices.SortFunc(extensions, func(a, b Extension) int { return strings.Compare(a.Suffix, b.Suffix) })
	return extensions
}
