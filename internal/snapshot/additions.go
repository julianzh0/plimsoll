package snapshot

import (
	"cmp"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

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
func additions(rules conventions.Rules, changes []Change, now Tree, atBasePath map[string]bool, entries []Entry, nowReadings, beforeReadings map[string]reading) []Addition {
	// A name is new in a file whose base declared no such name. It is left out as moved only when, among the changed
	// files of its language, exactly one base declaration of the name disappeared and exactly one appeared, in another
	// file; each of several namesakes is listed.
	basePaths := map[string]string{}
	atBase := map[string]map[string]bool{} // a changed file to the names its base declared
	type moves struct{ gone, came int }
	moved := map[string]map[string]*moves{} // per language, per name
	for _, change := range changes {
		basePath := change.From
		if basePath == "" && atBasePath[change.Path] {
			basePath = change.Path
		}
		basePaths[change.Path] = basePath
		before, now := map[string]bool{}, map[string]bool{}
		if basePath != "" {
			for _, d := range beforeReadings[basePath].declarations {
				before[d.name] = true
			}
		}
		for _, d := range nowReadings[change.Path].declarations {
			now[d.name] = true
		}
		atBase[change.Path] = before
		language, _ := rules.LanguageOf(cmp.Or(change.Path, basePath))
		if moved[language.Name] == nil {
			moved[language.Name] = map[string]*moves{}
		}
		count := func(name string) *moves {
			if moved[language.Name][name] == nil {
				moved[language.Name][name] = &moves{}
			}
			return moved[language.Name][name]
		}
		for name := range before {
			if !now[name] {
				count(name).gone++
			}
		}
		for name := range now {
			if !before[name] {
				count(name).came++
			}
		}
	}
	movedHere := func(language, name string) bool {
		m := moved[language][name]
		return m != nil && m.gone == 1 && m.came == 1
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
			if d.kind == 0 || d.implicit || atBase[change.Path][d.name] || movedHere(language.Name, d.name) || !listable(rules.Additions, d.name) ||
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
// of letters, digits and underscores, of any script.
func listable(rules conventions.Additions, name string) bool {
	if utf8.RuneCountInString(name) < 2 || slices.Contains(rules.SkippedNames, name) || strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool { return r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// withUses marks each addition used when a file that is not test code holds its name, and its declaring file when the
// name occurs there more often than the file declares it; test code holding it marks it held by tests.
// Every entry with content is scanned once per question that has an addition: 8 ms over 263 files on bff, 7.6 ms over
// 714 on nowinandroid, 1.35 s over 83,139 on swc (0.67 s of it in documentation and configuration files), measured
// 2026-11-20 on a cold status with base HEAD~30. Kept: stopping early would change which uses an addition's evidence
// names.
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
		eachIdentifierWord(entry.Content, func(word []byte, at int) {
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
			declared := 0 // a declaration of an added name is no use of it, in its own file or a copy's
			if slices.ContainsFunc(byName[name], func(i int) bool { return found[i].Path == entry.Path }) {
				declared = 1
			}
			for _, i := range byName[name] {
				switch {
				case mention:
					found[i].Docs = append(found[i].Docs, entry.Path)
				case role.IsTest():
					found[i].Tests = true
				case n > declared:
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

// eachIdentifierWord calls found with every word of content and its offset: a run of letters, numbers, `_` and `$`,
// read as runes, the characters the languages' identifier tokens are made of, so every identifier token is one word.
func eachIdentifierWord(content []byte, found func(word []byte, at int)) {
	start := -1
	for i := 0; i <= len(content); {
		r, size := utf8.DecodeRune(content[i:])
		if i < len(content) && (r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsNumber(r)) {
			if start < 0 {
				start = i
			}
			i += size
			continue
		}
		if start >= 0 {
			found(content[start:i], start)
			start = -1
		}
		i += max(size, 1)
	}
}

func wordByte(r rune) bool {
	return r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

// wordsFile says whether a file is read for its words alone: documentation or configuration outside skipped folders,
// and no data file.
func wordsFile(rules conventions.Rules, filePath string) bool {
	if inFolders(filePath, rules.SkippedFolders) || ignored(rules, filePath) || inFoldersMatching(filePath, rules.Additions.SkippedFolders) ||
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
