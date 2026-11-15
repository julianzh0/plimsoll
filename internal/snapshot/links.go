package snapshot

import (
	"bytes"
	"cmp"
	"path"
	"slices"
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// Links are one side's imports, as far as they could be followed. They hold paths and counts, never the text of an
// import.
type Links struct {
	Languages []LanguageImports // one per language entry with code files on this side, in the entries' order
	List      []Link            // every import that reaches code, sorted by From, Line and To
	PassOn    []string          // code files that only pass imports on, sorted
}

// LanguageImports counts the import statements of one language's code files by kind.
type LanguageImports struct {
	Language       string // the entry's name
	Resolved       int    // names exactly one tracked file or folder of the repository
	Outside        int
	Ambiguous      int
	Unreadable     int // built at run time
	NotFound       int // own by its form, reaches no tracked file
	PrefixFile     string
	Prefix         PrefixState
	PrefixRequired bool     // without its prefix the language's own imports cannot be told from outside ones
	Folders        bool     // its imports name folders
	NoRings        string   // the entry's reason; "" when rings are looked for
	Roots          []string // for a language that names no prefix file: the folders whose packages are its own
	Implicit       []string // file names the language runs without an import of them
	SourceSets     bool     // its packages lie under source roots, and a test source set is reached only from one
}

// Own is the language's imports of the repository's own code: all but those outside.
func (l LanguageImports) Own() int { return l.Resolved + l.Ambiguous + l.Unreadable + l.NotFound }

// NotFollowed is the language's own imports that are no link: ambiguous, not readable or not found.
func (l LanguageImports) NotFollowed() int { return l.Ambiguous + l.Unreadable + l.NotFound }

// Link is one import of a code file that reaches code.
type Link struct {
	From     string   // the importing code file
	Line     int      // the line that names the target
	To       string   // the tracked file the import names, or the folder ("." is the root)
	Folder   bool     // To is a folder
	Files    []string // the code files the import reaches, sorted: To itself, or the folder's files
	TypeOnly bool     // the statement imports types only
}

// kind is what became of one import statement.
type kind int

const (
	resolved kind = iota + 1
	outside
	ambiguous
	notFound
)

// side is what resolving one side's imports needs: its tracked paths, by folder, and its code files.
type side struct {
	tracked map[string]bool
	folders map[string][]string // the tracked paths directly in each folder, sorted
	code    map[string]Role
}

func sideOf(tree Tree) side {
	s := side{tracked: map[string]bool{}, folders: map[string][]string{}, code: rolesOf(tree.Files)}
	for _, p := range tree.Paths {
		s.tracked[p] = true
		s.folders[path.Dir(p)] = append(s.folders[path.Dir(p)], p)
	}
	return s
}

// contentOf maps each entry's path to its content, a leading byte order mark dropped. A tracked path that was not
// read maps to nil.
func contentOf(entries []Entry) map[string][]byte {
	content := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		content[entry.Path] = bytes.TrimPrefix(entry.Content, []byte(byteOrderMark))
	}
	return content
}

// target is a resolved import: the file or folder it names and the code files it reaches.
type target struct {
	to     string
	folder bool
	files  []string
}

// statement is one import statement as a file writes it. Its target text lives only while the snapshot is built.
type statement struct {
	spec     string
	names    []string // the names it imports out of its target; nil when it has no names group or names only "*"
	line     int
	typeOnly bool
}

// reading is what one file's content says about its imports, before they are resolved against a side.
type reading struct {
	statements []statement
	computed   int  // imports whose target is built at run time
	passesOn   bool // the whole content only passes imports on
}

// readImports reads the imports of a side's code files, once per content: a file that reuse holds keeps that
// reading.
func readImports(rules conventions.Rules, content map[string][]byte, files []File, reuse map[string]reading) map[string]reading {
	found := map[string]reading{}
	for _, file := range files {
		language, known := rules.LanguageOf(file.Path)
		if !known || len(language.Imports.Statements) == 0 {
			continue
		}
		if r, kept := reuse[file.Path]; kept {
			found[file.Path] = r
			continue
		}
		found[file.Path] = scan(language.Imports, content[file.Path])
	}
	return found
}

// tries says whether a form is tried on a text: it has a pattern, and the text holds one of its words when it names any.
func tries(form conventions.ImportForm, text []byte) bool {
	if form.Pattern == nil {
		return false
	}
	return len(form.Words) == 0 || slices.ContainsFunc(form.Words, func(word string) bool { return bytes.Contains(text, []byte(word)) })
}

// scan finds the import statements of one file's content.
func scan(imports conventions.Imports, text []byte) reading {
	r := reading{passesOn: imports.PassesOn != nil && imports.PassesOn.Match(text)}
	if imports.Until != nil {
		if at := imports.Until.FindIndex(text); at != nil {
			text = text[:at[0]]
		}
	}
	if tries(imports.Computed, text) {
		r.computed = len(imports.Computed.Pattern.FindAllIndex(text, -1))
	}
	var typeOnlyBlocks [][]int
	if tries(imports.TypeOnlyBlock, text) {
		typeOnlyBlocks = imports.TypeOnlyBlock.Pattern.FindAllIndex(text, -1)
	}
	for _, form := range imports.Statements {
		if !tries(form, text) {
			continue
		}
		spec, typeGroup, namesGroup := form.Pattern.SubexpIndex("spec"), form.Pattern.SubexpIndex("type"), form.Pattern.SubexpIndex("names")
		for _, m := range form.Pattern.FindAllSubmatchIndex(text, -1) {
			st := statement{
				spec: string(text[m[2*spec]:m[2*spec+1]]), line: 1 + bytes.Count(text[:m[2*spec]], []byte("\n")),
				typeOnly: typeGroup >= 0 && m[2*typeGroup] >= 0 ||
					slices.ContainsFunc(typeOnlyBlocks, func(block []int) bool { return block[0] <= m[2*spec] && m[2*spec] < block[1] }),
			}
			if namesGroup >= 0 && m[2*namesGroup] >= 0 {
				st.names = importedNames(string(text[m[2*namesGroup]:m[2*namesGroup+1]]))
			}
			r.statements = append(r.statements, st)
		}
	}
	return r
}

// links resolves the readings of one side's code files against that side's tracked paths and prefix files.
func links(rules conventions.Rules, content map[string][]byte, tree Tree, readings map[string]reading) Links {
	s := sideOf(tree)
	var found Links
	for _, language := range rules.Languages {
		imports := language.Imports
		if len(imports.Statements) == 0 {
			continue
		}
		var files []File
		for _, file := range tree.Files {
			if file.Language == language.Name {
				files = append(files, file)
			}
		}
		if len(files) == 0 {
			continue
		}
		own := readPrefix(rules, language, content, s.tracked)
		counts := LanguageImports{
			Language: language.Name, PrefixFile: imports.Prefix.File, Prefix: own.state,
			PrefixRequired: imports.Prefix.Required, Folders: imports.Target == conventions.FolderTarget,
			NoRings: imports.NoRings, Roots: imports.Prefix.Roots, Implicit: imports.Implicit,
			SourceSets: imports.Prefix.Format == conventions.SourceRoots,
		}
		for _, file := range files {
			r := readings[file.Path]
			counts.Unreadable += r.computed
			fileLinks := s.resolveAll(language, own, file.Path, r.statements, &counts)
			found.List = append(found.List, fileLinks...)
			if len(fileLinks) > 0 && r.passesOn {
				found.PassOn = append(found.PassOn, file.Path)
			}
		}
		found.Languages = append(found.Languages, counts)
	}
	slices.SortFunc(found.List, func(a, b Link) int {
		return cmp.Or(strings.Compare(a.From, b.From), cmp.Compare(a.Line, b.Line), strings.Compare(a.To, b.To))
	})
	slices.Sort(found.PassOn)
	return found
}

// resolveAll counts each statement of one file by kind and returns the links of those that resolve.
func (s side) resolveAll(language conventions.Language, own prefix, from string, statements []statement, counts *LanguageImports) []Link {
	var found []Link
	for _, st := range statements {
		reached, outcome := s.resolve(language, own, from, st)
		switch outcome {
		case resolved:
			counts.Resolved++
		case outside:
			counts.Outside++
		case ambiguous:
			counts.Ambiguous++
		case notFound:
			counts.NotFound++
		}
		if outcome != resolved {
			continue
		}
		for _, t := range reached {
			files := slices.DeleteFunc(t.files, func(p string) bool { return p == from })
			if len(files) > 0 {
				found = append(found, Link{From: from, Line: st.line, To: t.to, Folder: t.folder, Files: files, TypeOnly: st.typeOnly})
			}
		}
	}
	return found
}

// resolve decides what a statement names: whether it is the project's own by its form, and which tracked targets it
// reaches, distinct. The statement's text is not kept.
func (s side) resolve(language conventions.Language, own prefix, from string, st statement) ([]target, kind) {
	imports := language.Imports
	bases, catchAll, outcome := s.basesOf(imports, own, from, st.spec)
	if outcome != 0 {
		return nil, outcome
	}
	if imports.Target == conventions.FolderTarget {
		var reached []target
		for _, base := range bases {
			if t, outcome := s.folder(language, base); outcome == resolved {
				reached = append(reached, t)
			}
		}
		if len(reached) == 0 {
			return nil, notFound
		}
		return reached, resolved
	}
	module, moduleOutcome := s.file(imports.Candidates, bases, catchAll)
	if st.names == nil {
		return []target{module}, moduleOutcome
	}
	counted := map[kind]int{}
	var reached []target
	for _, name := range st.names {
		named := make([]string, len(bases))
		for i, base := range bases {
			named[i] = path.Join(base, name)
		}
		t, outcome := s.file(imports.Candidates, named, false)
		if outcome == notFound {
			t, outcome = module, moduleOutcome
		}
		counted[outcome]++
		if outcome == resolved && !slices.ContainsFunc(reached, func(r target) bool { return r.to == t.to }) {
			reached = append(reached, t)
		}
	}
	// Names that are partly resolved and partly outside, or any name not found, make the statement not found.
	switch {
	case counted[resolved] == len(st.names):
		return reached, resolved
	case counted[outside] == len(st.names):
		return nil, outside
	case counted[ambiguous] > 0 && counted[notFound] == 0:
		return nil, ambiguous
	}
	return nil, notFound
}

// basesOf finds the paths an own import may mean, by its form, and whether it is a catch-all; or, when it is no own
// import with a path, the kind it already is.
func (s side) basesOf(imports conventions.Imports, own prefix, from, spec string) (bases []string, catchAll bool, outcome kind) {
	var relative string
	switch {
	case imports.Relative == conventions.DotSlash &&
		(spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../")):
		relative = path.Join(path.Dir(from), spec)
	case imports.Relative == conventions.NoScheme && !strings.Contains(spec, ":"):
		relative = path.Join(path.Dir(from), spec)
	case imports.Relative == conventions.LeadingDots && strings.HasPrefix(spec, "."):
		rest := strings.TrimLeft(spec, ".")
		up := strings.Repeat("../", len(spec)-len(rest)-1)
		relative = path.Join(path.Dir(from), up, dottedPath(rest))
	case own.own == nil:
		return nil, false, outside
	}
	if relative != "" {
		if leaves(relative) {
			return nil, false, notFound
		}
		return []string{relative}, false, 0
	}
	bases, catchAll = own.own(from, spec)
	if len(bases) == 0 {
		return nil, false, outside
	}
	return bases, catchAll, 0
}

// file resolves a file target: the path as written when it is tracked, otherwise its candidates, pooled over every
// base path.
func (s side) file(candidates conventions.Candidates, bases []string, catchAll bool) (target, kind) {
	var found []string
	for _, base := range bases {
		if s.tracked[base] {
			found = append(found, base)
			continue
		}
		for _, candidate := range candidatesOf(candidates, base) {
			if s.tracked[candidate] {
				found = append(found, candidate)
			}
		}
	}
	slices.Sort(found)
	found = slices.Compact(found)
	switch {
	case len(found) == 1:
		reached := target{to: found[0]}
		if _, isCode := s.code[found[0]]; isCode {
			reached.files = []string{found[0]}
		}
		return reached, resolved
	case len(found) > 1:
		return target{}, ambiguous
	case catchAll:
		return target{}, outside
	}
	return target{}, notFound
}

// candidatesOf are the paths an import of base may mean: with an extension added, swapped or an index file inside.
func candidatesOf(candidates conventions.Candidates, base string) []string {
	var found []string
	for _, extension := range candidates.Add {
		found = append(found, base+extension)
	}
	for _, swap := range candidates.Swap {
		if stem, swapped := strings.CutSuffix(base, swap.From); swapped {
			for _, extension := range swap.To {
				found = append(found, stem+extension)
			}
		}
	}
	for _, name := range candidates.InFolder {
		found = append(found, path.Join(base, name))
	}
	return found
}

// folder resolves a folder target: the tracked files directly in it with one of the language's extensions that the
// entry does not leave out.
func (s side) folder(language conventions.Language, folder string) (target, kind) {
	reached := target{to: folder, folder: true}
	inFolder := false
	for _, p := range s.folders[folder] {
		suffix := conventions.Suffix(p)
		if !slices.Contains(language.Extensions, suffix) && !slices.Contains(language.Imports.FolderAlso, suffix) ||
			language.Imports.FolderExcept != nil && language.Imports.FolderExcept.MatchString(path.Base(p)) {
			continue
		}
		inFolder = true
		if _, isCode := s.code[p]; isCode {
			reached.files = append(reached.files, p)
		}
	}
	if !inFolder {
		return target{}, notFound
	}
	return reached, resolved
}

// importedNames are the names a statement imports out of its target, as its "names" group writes them: comments,
// parentheses and line continuations dropped, split at commas, each name's first word; "*" names nothing. Nil when it
// names none.
func importedNames(text string) []string {
	var names []string
	for line := range strings.SplitSeq(text, "\n") {
		line, _, _ = strings.Cut(line, "#")
		for part := range strings.SplitSeq(line, ",") {
			if words := strings.FieldsFunc(part, isNameNoise); len(words) > 0 && words[0] != "*" {
				names = append(names, words[0])
			}
		}
	}
	return names
}

// isNameNoise is a character between imported names: white space, a parenthesis or a line continuation.
func isNameNoise(r rune) bool { return unicode.IsSpace(r) || r == '(' || r == ')' || r == '\\' }
