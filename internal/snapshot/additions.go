package snapshot

import (
	"cmp"
	"path"
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
	Used  bool     // a file that is not test code holds the name, its declaring file more often than it declares it
	Tests bool     // test code holds the name
	Docs  []string // the documentation files that name it, sorted: a mention, no use
}

// topName is one top-level name a file declares, as its entry's query reads it.
type topName struct {
	name     string
	kind     AdditionKind
	implicit bool // a runtime or framework calls it without its being written
	exported bool // public to code outside the file
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
	tracked, byFolder := map[string]bool{}, map[string][]string{}
	for _, p := range now.Paths {
		tracked[p] = true
		byFolder[path.Dir(p)] = append(byFolder[path.Dir(p)], p)
	}
	content := map[string][]byte{}
	for _, entry := range entries {
		content[entry.Path] = entry.Content
	}
	var found []Addition
	for _, change := range changes {
		r, read := nowReadings[change.Path]
		basePath := basePaths[change.Path]
		if roles[change.Path] != Product || !read || !r.parsed || !mayDeclare(rules.Additions, change.Path) ||
			basePath != "" && !beforeReadings[basePath].parsed {
			continue
		}
		language, _ := rules.LanguageOf(change.Path)
		library := inLibrary(language.Library, byFolder, content, change.Path)
		for _, d := range r.declarations {
			if d.kind == 0 || d.implicit || atBase[d.name] || !listable(rules.Additions, d.name) ||
				loadedByConvention(language, tracked, change.Path, d.name) || d.exported && library {
				continue
			}
			found = append(found, Addition{Path: change.Path, Name: d.name, Kind: d.kind})
		}
	}
	if len(found) == 0 {
		return nil
	}
	withUses(rules, found, entries, roles, nowReadings)
	for i := range found {
		slices.Sort(found[i].Docs)
	}
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
		mention := slices.Contains(rules.Additions.Docs, conventions.Suffix(entry.Path))
		if !isCode && excluded(rules, entry.Path) && !wordsFile(rules, entry.Path) {
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
				case mention:
					found[i].Docs = append(found[i].Docs, entry.Path)
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

// wordsFile says whether a file is read for its words alone: documentation or configuration outside skipped folders,
// and no data file.
func wordsFile(rules conventions.Rules, filePath string) bool {
	if inFolders(filePath, rules.SkippedFolders) || inFoldersMatching(filePath, rules.Additions.SkippedFolders) ||
		isDataFile(rules.Additions, filePath) {
		return false
	}
	suffix := conventions.Suffix(filePath)
	return slices.Contains(rules.Additions.Docs, suffix) || slices.Contains(rules.Additions.Config, suffix) ||
		slices.Contains(rules.Additions.ConfigNames, path.Base(filePath))
}

// loadedByConvention says whether a framework of the language loads a declaration by where its file lies or what it is
// called.
func loadedByConvention(language conventions.Language, tracked map[string]bool, filePath, name string) bool {
	return slices.ContainsFunc(language.Loaded, func(c conventions.Convention) bool {
		if c.File != "" && !nameMatches(c.File, filePath) {
			return false
		}
		if c.Names != "" {
			if matched, _ := path.Match(c.Names, name); !matched {
				return false
			}
		}
		if c.Beside == nil {
			return true
		}
		for folder := path.Dir(filePath); ; folder = path.Dir(folder) {
			if slices.ContainsFunc(c.Beside, func(file string) bool { return tracked[path.Join(folder, file)] }) {
				below := strings.TrimPrefix(path.Dir(filePath), folder)
				return c.Folder == "" || slices.Contains(strings.Split(below, "/"), c.Folder)
			}
			if folder == "." || folder == "/" {
				return false
			}
		}
	})
}

// inLibrary says whether a file belongs to a published library: the nearest folder above it that holds a manifest of
// the language decides, by the manifest's content; a manifest that was not read makes no library. byFolder lists the
// tracked paths of each folder.
func inLibrary(library *conventions.Library, byFolder map[string][]string, content map[string][]byte, filePath string) bool {
	if library == nil {
		return false
	}
	for folder := path.Dir(filePath); ; folder = path.Dir(folder) {
		for _, p := range byFolder[folder] {
			if !nameMatches(library.Manifest, p) {
				continue
			}
			text := content[p]
			if text == nil {
				return false
			}
			return (library.Is == nil || library.Is.Match(text)) && (library.Not == nil || !library.Not.Match(text))
		}
		if folder == "." || folder == "/" {
			return false
		}
	}
}
