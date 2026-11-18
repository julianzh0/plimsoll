package snapshot

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"

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
	importRules, ruleRecords := importRules(rules, ownNow)
	layout.Rules = ruleRecords
	rules = withLayout(rules, layout)
	rules = withSharedSuffixes(rules, source.Now)
	limits, declaredLimits := declaredLimits(rules, ownNow, ownBefore)
	rules.Limits = limits
	question := readings.begin()
	now := tree(rules, source.Now, nil)
	nowMeasured := measuredFiles(rules, source.Changes, now, Tree{}, nowContent, nil)
	nowReadings := readings.side(rules, nowContent, now.Files, question, changed, nowMeasured.now)
	withTestLines(now.Files, nowReadings)
	unchanged := map[string]File{}
	for _, file := range now.Files {
		if !changed[file.Path] {
			unchanged[file.Path] = file
		}
	}
	before := tree(rules, beforeEntries, unchanged)
	measured := measuredFiles(rules, source.Changes, now, before, nowContent, beforeContent)
	beforeReadings := readings.side(rules, beforeContent, before.Files, question, changed, measured.base)
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
	atBase := map[string]bool{}
	for _, p := range before.Paths {
		atBase[p] = true
	}
	now.Links = links(rules, nowContent, now, nowReadings, baseImportsOf(source.Changes, before, atBase, beforeReadings))
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
		ImportRules:         importRules,
		OwnFileProblems:     ownNow.problems,
		Acknowledged:        Acknowledged{Table: rules.AcknowledgedTable, IDs: ownNow.ack.applied, Problems: ownNow.ack.problems()},
		OwnFileUnreadAtBase: ownBefore.unreadable,
		Before:              before,
		Now:                 now,
		Changes:             source.Changes,
		Footprint:           footprint(rules, source.Changes, len(source.Since), now, before),
		History:             history(rules, source.Commits, source.Since),
		Layout:              layout,
		TestRule:            rules.TestRule(),
		PlanningRule:        rules.PlanningRule(),
		PlanningMatches:     planningMatches(rules, now.Planning, before.Planning),
		Unverified:          unverified(now.Files),
		Untracked:           source.Untracked,
		Submodules:          source.Submodules,
		Unreadable:          slices.Compact(unreadable),
		Missing:             source.Missing,
		Unparsed:            unparsed(nowReadings, beforeReadings),
		SkippedChanges:      skippedChanges(rules, source.Changes),
		Additions:           additions(rules, source.Changes, now, atBase, source.Now, nowReadings, beforeReadings),
		SyntaxErrors:        syntaxErrors(changed, nowReadings, beforeReadings),
		Functions:           functionChanges(rules, source.Changes, measured, source.Missing, atBase, now, before, nowContent, beforeContent, nowReadings, beforeReadings),
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

// planningMatches are the entries the planning files of both sides match.
func planningMatches(rules conventions.Rules, sides ...[]string) map[string]PlanningMatch {
	matches := map[string]PlanningMatch{}
	for _, paths := range sides {
		for _, p := range paths {
			matches[p], _ = planningMatch(rules, p)
		}
	}
	return matches
}

// planningFiles are the paths that lie under a planning or review path.
func planningFiles(rules conventions.Rules, paths []string) []string {
	var found []string
	for _, p := range paths {
		if planningPath(rules, p) {
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
	files := codeFiles(rules, entries, known)
	var side Tree
	for i, entry := range entries {
		side.Paths = append(side.Paths, entry.Path)
		if files[i].Path != "" {
			side.Files = append(side.Files, files[i])
		}
	}
	slices.Sort(side.Paths)
	slices.SortFunc(side.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return side
}

// codeFiles classifies the entries, one goroutine per processor, each taking every n-th entry: the role patterns run
// over every test file's content, which on a repository of tens of thousands of files takes seconds by one (swc: 3 s).
// An entry that is no code file gives an empty File; a file in known is taken as it is.
func codeFiles(rules conventions.Rules, entries []Entry, known map[string]File) []File {
	files := make([]File, len(entries))
	workers := runtime.GOMAXPROCS(0)
	var group sync.WaitGroup
	for worker := range workers {
		group.Go(func() {
			for i := worker; i < len(entries); i += workers {
				if file, found := known[entries[i].Path]; found {
					files[i] = file
				} else if file, isCode := codeFile(rules, entries[i]); isCode {
					files[i] = file
				}
			}
		})
	}
	group.Wait()
	return files
}

func codeFile(rules conventions.Rules, entry Entry) (File, bool) {
	if entry.Binary || entry.Content == nil || excluded(rules, entry.Path) || onPaths(rules, entry.Path, rules.GeneratedNames) || generatedOrBinary(rules, entry.Content) {
		return File{}, false
	}
	file := File{Path: entry.Path, Role: role(rules, entry.Path, entry.Content), Lines: countLines(entry.Content), Limit: rules.Limits.FileLines}
	file.Record = recordPath(rules, entry.Path)
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

// skippedChanges names the skipped folders that hold changed files, by the first skipped folder of each path, with
// how many changed files each holds.
func skippedChanges(rules conventions.Rules, changes []Change) []string {
	counts := map[string]int{}
	for _, change := range changes {
		parts := strings.Split(change.Path, "/")
		for i, folder := range parts[:len(parts)-1] {
			if slices.Contains(rules.SkippedFolders, folder) {
				counts[strings.Join(parts[:i+1], "/")+"/"]++
				break
			}
		}
	}
	var found []string
	for _, folder := range slices.Sorted(maps.Keys(counts)) {
		noun := "files"
		if counts[folder] == 1 {
			noun = "file"
		}
		found = append(found, fmt.Sprintf("`%s` (%d %s)", folder, counts[folder], noun))
	}
	return found
}
