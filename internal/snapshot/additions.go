package snapshot

import (
	"cmp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// AdditionKind is what a change declared: a function, a type or a value.
type AdditionKind int

// The three kinds of declaration.
const (
	AddedFunction AdditionKind = iota + 1
	AddedType
	AddedValue
)

func (k AdditionKind) String() string {
	switch k {
	case AddedFunction:
		return "function"
	case AddedType:
		return "type"
	case AddedValue:
		return "value"
	}
	return ""
}

// Addition is one top-level name a change declared in a product file, and where the repository's files use it.
type Addition struct {
	Path  string
	Name  string
	Kind  AdditionKind
	Used  bool // a file that is not test code holds the name, its declaring file more often than it declares it
	Tests bool // test code holds the name
}

// topName is one top-level name a file declares, as its entry's query reads it.
type topName struct {
	name     string
	kind     AdditionKind
	implicit bool // a runtime or framework calls it without its being written
}

// additions are the names the changes declared in product files that the base did not declare, each with its uses.
// A file the grammar could not read on either side declares nothing for the list.
func additions(rules conventions.Rules, changes []Change, now, before Tree, entries []Entry, nowReadings, beforeReadings map[string]reading) []Addition {
	atBase := map[string]bool{}
	basePaths := map[string]string{}
	for _, change := range changes {
		basePath := change.From
		if basePath == "" && slices.Contains(before.Paths, change.Path) {
			basePath = change.Path
		}
		basePaths[change.Path] = basePath
		if basePath != "" {
			for _, d := range beforeReadings[basePath].declarations {
				atBase[d.name] = true
			}
		}
	}
	roles := rolesOf(now.Files)
	var found []Addition
	for _, change := range changes {
		r, read := nowReadings[change.Path]
		basePath := basePaths[change.Path]
		if roles[change.Path] != Product || !read || !r.parsed || !mayDeclare(rules.Additions, change.Path) ||
			basePath != "" && !beforeReadings[basePath].parsed {
			continue
		}
		for _, d := range r.declarations {
			if !d.implicit && !atBase[d.name] && listable(rules.Additions, d.name) {
				found = append(found, Addition{Path: change.Path, Name: d.name, Kind: d.kind})
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	withUses(rules, found, entries, roles, nowReadings)
	slices.SortFunc(found, func(a, b Addition) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.Name, b.Name))
	})
	return found
}

// mayDeclare says whether a product file's declarations count: not in a skipped folder, not of a skipped name.
func mayDeclare(rules conventions.Additions, filePath string) bool {
	return !inFoldersMatching(filePath, rules.SkippedFolders) &&
		!slices.ContainsFunc(rules.SkippedFiles, func(pattern string) bool { return nameMatches(pattern, filePath) })
}

// listable says whether a name may be listed: not one a runtime calls, longer than one letter, not __x__, and made only
// of ASCII letters, digits and underscores.
func listable(rules conventions.Additions, name string) bool {
	if len(name) < 2 || slices.Contains(rules.SkippedNames, name) || strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return !wordByte(r) })
}

// withUses marks each addition used when a file that is not test code holds its name, and its declaring file when the
// name occurs there more often than the file declares it; test code holding it marks it held by tests.
// NOTE: every entry with content is scanned once per question that has an addition. Upgrade: stop once each addition
// has a use, scanning the declaring and changed files first.
func withUses(rules conventions.Rules, found []Addition, entries []Entry, roles map[string]Role, readings map[string]reading) {
	byName := map[string][]int{}
	for i, a := range found {
		byName[a.Name] = append(byName[a.Name], i)
	}
	for _, entry := range entries {
		if entry.Content == nil || isDataFile(rules.Additions, entry.Path) {
			continue
		}
		role, isCode := roles[entry.Path]
		if !isCode && excluded(rules, entry.Path) {
			continue
		}
		counts, inTests := map[string]int{}, map[string]bool{}
		testSpans := readings[entry.Path].testSpans
		eachWord(entry.Content, func(word []byte, at int) {
			if _, wanted := byName[string(word)]; !wanted {
				return
			}
			if inSpans(testSpans, at) {
				inTests[string(word)] = true
				return
			}
			counts[string(word)]++
		})
		for name := range inTests {
			for _, i := range byName[name] {
				found[i].Tests = true
			}
		}
		for name, n := range counts {
			declared := 0
			for _, d := range readings[entry.Path].declarations {
				if d.name == name {
					declared++
				}
			}
			for _, i := range byName[name] {
				switch {
				case role.IsTest():
					found[i].Tests = true
				case entry.Path != found[i].Path || n > declared:
					found[i].Used = true
				}
			}
		}
	}
}

// isDataFile says whether a file's words are no use of a name: a lockfile, minified code, a source map.
func isDataFile(rules conventions.Additions, filePath string) bool {
	return slices.ContainsFunc(rules.DataFiles, func(pattern string) bool { return nameMatches(pattern, filePath) })
}

// eachWord calls found with every run of ASCII letters, digits and underscores in content, and its offset.
func eachWord(content []byte, found func(word []byte, at int)) {
	start := -1
	for i := 0; i <= len(content); i++ {
		if i < len(content) && wordByte(rune(content[i])) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			found(content[start:i], start)
			start = -1
		}
	}
}

func wordByte(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}
