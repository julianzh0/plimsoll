package snapshot

import (
	"bytes"
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
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
	Unreadable     int // built at run time, or written once for every file of a project
	NotFound       int // own by its form, reaches no tracked file
	PrefixFile     string
	Prefix         PrefixState
	PrefixRequired bool     // without its prefix the language's own imports cannot be told from outside ones
	Folders        bool     // its imports name folders
	NoRings        string   // the entry's reason; "" when rings are looked for
	Roots          []string // for a language that names no prefix file: the folders whose packages are its own
	Implicit       []string // file names, or patterns of them, the language runs without an import of them
	SourceSets     bool     // its packages lie under source roots, and a test source set is reached only from one
	NotSeen        string   // the entry's sentence on use between files without an import statement; "" for none
	NoImporter     string   // the entry's files that hold an import and are no importer of it; "" for none
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
	Declares bool     // the statement declares a module in its own file (Rust `mod x;`): containment, not use
	Added    bool     // the change wrote the statement of this import; set on the links of now only
}

// Uses says whether the importing file uses what the link reaches: a module's declaration in its parent is
// containment, not use, so it makes no importer and no user.
func (l Link) Uses() bool { return !l.Declares }

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
	paths   []string // every tracked path, sorted
	tracked map[string]bool
	folders map[string][]string // the tracked paths directly in each folder, sorted
	code    map[string]Role
}

func sideOf(tree Tree) side {
	s := side{paths: slices.Sorted(slices.Values(tree.Paths)), tracked: map[string]bool{}, folders: map[string][]string{}, code: rolesOf(tree.Files)}
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
	within   []string // the inline modules it stands in, the outermost first
	// everywhere is a statement written once for every file of its project.
	everywhere bool
}

// reading is what one file's content says: its imports, before they are resolved against a side, its test code and
// its settings reads.
type reading struct {
	statements  []statement
	computed    int  // imports whose target is built at run time
	passesOn    bool // the whole content only passes imports on
	testLines   int  // lines of test code inside the file, by the entry's blocks
	unread      bool // the parser could not read the file
	syntaxError bool // the parser met a syntax error: a statement next to it may be missing
	parsed      bool // the entry's grammar read the file
	// declarations are the top-level names the file declares outside its test code, when it was read with them.
	declarations []topName
	// units are the file's comparison units and pure imports (US-63), when it was read with its declarations.
	units       []unit
	pureImports []span
	unitsRead   bool   // the grammar read the file for its units, with or without errors
	errors      []span // where the grammar met a syntax error
	testSpans   []span // the test code inside the file, by the entry's blocks
	// functions are the file's measured functions, when it was read with them; functionsUnread when the walk failed.
	functions       []function
	functionsUnread bool
}

// scan finds the import statements of one file's content, its literals blanked.
func scan(imports conventions.Imports, text []byte) reading {
	var r reading
	if imports.Computed.Pattern != nil {
		r.computed = len(imports.Computed.Pattern.FindAllIndex(text, -1))
	}
	for _, form := range imports.Statements {
		spec := form.Pattern.SubexpIndex("spec")
		for _, m := range form.Pattern.FindAllSubmatchIndex(text, -1) {
			at, written := m[2*spec], string(text[m[2*spec]:m[2*spec+1]])
			st := statement{line: 1 + bytes.Count(text[:at], []byte("\n"))}
			targets := []string{written}
			if imports.Grouped {
				targets = grouped(written)
			}
			for _, target := range targets {
				st.spec = target
				r.statements = append(r.statements, st)
			}
		}
	}
	return r
}

// modulesAt are the names of the module spans that hold an offset, the outermost first.
func modulesAt(modules []span, at int) []string {
	var names []string
	for _, m := range modules {
		if m.start < at && at <= m.end {
			names = append(names, m.name)
		}
	}
	return names
}

// grouped expands a target that groups paths in braces into its leaves: the text before a "{" is the prefix of every
// part up to the brace that closes it, parts split at commas outside inner braces. "a::{b, c::{d, e}}" gives a::b,
// a::c::d and a::c::e. A leaf is trimmed; an empty part is dropped.
func grouped(target string) []string {
	target = strings.TrimSpace(target)
	open := strings.IndexByte(target, '{')
	if open < 0 {
		return []string{target}
	}
	end := len(target)
	if at := closing([]byte(target), open); target[at] == '}' {
		end = at
	}
	prefix := strings.TrimSpace(target[:open])
	var leaves []string
	for _, part := range topLevelParts(target[open+1 : end]) {
		if part = strings.TrimSpace(part); part != "" {
			for _, leaf := range grouped(part) {
				leaves = append(leaves, prefix+leaf)
			}
		}
	}
	return leaves
}

// topLevelParts splits a text at the commas outside its braces.
func topLevelParts(text string) []string {
	var parts []string
	depth, start := 0, 0
	for i, c := range text {
		switch {
		case c == '{':
			depth++
		case c == '}':
			depth--
		case c == ',' && depth == 0:
			parts, start = append(parts, text[start:i]), i+1
		}
	}
	return append(parts, text[start:])
}

// links resolves the readings of one side's code files against that side's tracked paths and prefix files. base,
// given for the side of now, says which imports the change wrote.
func links(rules conventions.Rules, content map[string][]byte, tree Tree, readings map[string]reading, base *baseImports) Links {
	s := sideOf(tree)
	var found Links
	for _, language := range rules.Languages {
		imports := language.Imports
		var files []File
		for _, file := range tree.Files {
			if file.Language == language.Name {
				files = append(files, file)
			}
		}
		if len(files) == 0 {
			continue
		}
		languageReadings := map[string]reading{}
		for _, file := range files {
			languageReadings[file.Path] = readings[file.Path]
		}
		own := readPrefix(rules, language, content, s.tracked, languageReadings)
		counts := LanguageImports{
			Language: language.Name, PrefixFile: imports.Prefix.File, Prefix: own.state,
			PrefixRequired: imports.Prefix.Required, Folders: imports.Target == conventions.FolderTarget,
			NoRings: imports.NoRings, Roots: imports.Prefix.Roots, Implicit: imports.Implicit,
			SourceSets: imports.Prefix.Format == conventions.SourceRoots, NotSeen: imports.NotSeen, NoImporter: imports.NoImporter,
		}
		for _, file := range files {
			r := readings[file.Path]
			counts.Unreadable += r.computed
			fileLinks := s.resolveAll(language, own, file.Path, r.statements, &counts, base)
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
func (s side) resolveAll(language conventions.Language, own prefix, from string, statements []statement, counts *LanguageImports, base *baseImports) []Link {
	var found []Link
	for _, st := range statements {
		reached, outcome := s.resolve(language, own, from, st)
		if st.everywhere && outcome == resolved {
			// It serves every file of its project, which no link can say: counted as not readable, so the share of
			// own imports not followed shows it.
			counts.Unreadable++
			continue
		}
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
			// A grouped statement gives several targets on one line, and two of them may reach one file.
			repeated := slices.ContainsFunc(found, func(l Link) bool { return l.Line == st.line && l.To == t.to && l.TypeOnly == st.typeOnly })
			if len(files) > 0 && !repeated {
				found = append(found, Link{From: from, Line: st.line, To: t.to, Folder: t.folder, Files: files, TypeOnly: st.typeOnly,
					Declares: declaresModule(st), Added: base.added(from, t.to, st)})
			}
		}
	}
	return found
}

// resolve decides what a statement names: whether it is the project's own by its form, and which tracked targets it
// reaches, distinct. The statement's text is not kept.
func (s side) resolve(language conventions.Language, own prefix, from string, st statement) ([]target, kind) {
	imports := language.Imports
	bases, catchAll, outcome := s.basesOf(imports, own, from, st)
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
func (s side) basesOf(imports conventions.Imports, own prefix, from string, st statement) (bases []string, catchAll bool, outcome kind) {
	spec := st.spec
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
	bases, catchAll = own.own(from, spec, st.within)
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
	for _, p := range s.filesOf(folder, language.Imports.FolderTree) {
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

// filesOf are the tracked paths directly in a folder, or, for a tree, at any depth below it, sorted. A tree's paths
// are a part of the side's sorted paths, shared: the caller only reads them.
func (s side) filesOf(folder string, tree bool) []string {
	switch {
	case !tree:
		return s.folders[folder]
	case folder == ".":
		return s.paths
	}
	start, _ := slices.BinarySearch(s.paths, folder+"/")
	end := start
	for end < len(s.paths) && strings.HasPrefix(s.paths[end], folder+"/") {
		end++
	}
	return s.paths[start:end]
}
