package snapshot

import (
	"bytes"
	"hash/fnv"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// FileState is what a change did to one changed code file, for the blast radius (US-63).
type FileState int

// The states of a changed code file.
const (
	NotCounted     FileState = iota + 1 // in a skipped folder, generated, or its content is missing on a side
	NothingAltered                      // no declaration changed or was removed: comments, imports, added code
	Deleted                             // it exists only at the base
	AsAWhole                            // what changed could not be told by name: every user of the file counts
	ByName                              // declarations changed or were removed: the files that use their names count
)

// The reasons a changed code file is not counted, counted as a whole or deleted without names.
const (
	ReasonSkipped      = "in a skipped folder"
	ReasonMissing      = "its content is missing in this partial clone"
	ReasonGenerated    = "generated"
	ReasonSyntaxError  = "its declarations hold a syntax error"
	ReasonNotRead      = "the parser could not read it"
	ReasonLineRead     = "read line by line"
	ReasonRemainder    = "code outside its declarations changed"
	ReasonUnnamed      = "a changed declaration has no name plimsoll reads"
	ReasonNoIdentifier = "a changed declaration's name is no identifier"
)

// Radius is what the change did to the files that use its code: the state of each changed code file and the files
// that use what it altered or removed. It holds no name, token or text of any file (US-63, trust).
type Radius struct {
	Changed []ChangedFile // sorted by path now, then path at the base
	Users   []User        // untouched users, sorted by path
	Touched []User        // users the change itself touched, sorted by path
	Unread  []User        // walked files whose content is missing now, sorted by path
	// MissingNow says a code file's content is missing now: some use may be out of sight.
	MissingNow bool
}

// ChangedFile is one changed code file: its path now ("" when deleted) and at the base ("" when new), its language,
// its state and why.
type ChangedFile struct {
	Path, BasePath, Language string
	State                    FileState
	Reason                   string
}

// User is a file the walk reached: its role, whether it uses a name the change altered or removed (else it counts as
// a user of a file counted as a whole), whether every such use lies in its test code, and the languages of the
// changed files it was reached from.
type User struct {
	Path      string
	Role      Role
	ByName    bool
	TestsOnly bool
	Languages []string
}

// radiusInput is what the radius reads of one question.
type radiusInput struct {
	rules                       conventions.Rules
	changes                     []Change
	nowContent, baseContent     map[string][]byte // every tracked path of a side, nil when its content was not read
	now, before                 Tree
	nowReadings, beforeReadings map[string]reading
	missing                     []string
}

// item is one step of the walk: a file asked for one name, or, with no name, as a user of a file counted as a whole.
type item struct{ file, name string }

// blastRadius decides the state of each changed code file and walks from their seeds to the users.
func blastRadius(in radiusInput) Radius {
	var r Radius
	w := newWalk(in)
	for _, p := range in.missing {
		r.MissingNow = r.MissingNow || w.missingNow(p)
	}
	var all []changed
	for _, ch := range in.changes {
		all = append(all, w.changedFiles(ch)...)
	}
	if len(all) == 0 {
		return r
	}
	for _, c := range all {
		r.Changed = append(r.Changed, c.file)
		w.seed(c)
	}
	slices.SortFunc(r.Changed, func(a, b ChangedFile) int {
		return strings.Compare(a.Path+"\x00"+a.BasePath, b.Path+"\x00"+b.BasePath)
	})
	w.run()
	r.Users, r.Touched, r.Unread = w.results()
	return r
}

// changed is a changed code file with what the walk needs of it.
type changed struct {
	file     ChangedFile
	language conventions.Language
	names    []string // altered or removed
}

// radiusWalk holds the links of both sides and the state of the walk.
type radiusWalk struct {
	in       radiusInput
	touched  map[string]bool
	nowPath  map[string]string // a renamed file's base path to its path now
	to       map[string][]string
	toBase   map[string][]string
	reaches  map[string]bool // files whose links of now reach a code file
	passOn   map[string]bool
	passBase map[string]bool
	roles    map[string]Role
	queue    []item
	from     map[item][]string // the languages of the changed files an item comes from
	visited  map[item]bool
	users    map[string]*User
	unread   map[string]*User
	held     map[string]map[string]int // per file: name -> heldOutsideTests or heldInTestsOnly
	names    map[string]bool           // every name the walk asks for
}

func newWalk(in radiusInput) *radiusWalk {
	w := &radiusWalk{in: in, touched: map[string]bool{}, nowPath: map[string]string{}, to: map[string][]string{},
		toBase: map[string][]string{}, reaches: map[string]bool{}, passOn: map[string]bool{}, passBase: map[string]bool{},
		roles: map[string]Role{}, from: map[item][]string{}, visited: map[item]bool{}, users: map[string]*User{},
		unread: map[string]*User{}, held: map[string]map[string]int{}, names: map[string]bool{}}
	for _, ch := range in.changes {
		w.touched[ch.Path] = true
		if ch.From != "" {
			w.touched[ch.From], w.nowPath[ch.From] = true, ch.Path
		}
	}
	for _, link := range in.now.Links.List {
		for _, file := range link.Files {
			w.to[file] = append(w.to[file], link.From)
		}
		w.reaches[link.From] = w.reaches[link.From] || len(link.Files) > 0
	}
	for _, link := range in.before.Links.List {
		for _, file := range link.Files {
			w.toBase[file] = append(w.toBase[file], link.From)
		}
	}
	for _, p := range in.now.Links.PassOn {
		w.passOn[p] = true
	}
	for _, p := range in.before.Links.PassOn {
		w.passBase[p] = true
	}
	for _, file := range in.now.Files {
		w.roles[file.Path] = file.Role
	}
	return w
}

// changedFiles gives the changed code files of one change: one, or two for a file renamed into another folder of a
// language whose imports name folders (deleted at its old path, new at its new one).
func (w *radiusWalk) changedFiles(ch Change) []changed {
	basePath := ch.From
	if _, tracked := w.in.baseContent[ch.Path]; basePath == "" && tracked {
		basePath = ch.Path
	}
	nowPath := ""
	if _, tracked := w.in.nowContent[ch.Path]; tracked {
		nowPath = ch.Path
	}
	either := cmpOr(nowPath, basePath)
	language, known := w.in.rules.LanguageOf(either)
	if !known || either == "" {
		return nil
	}
	if ch.From != "" && nowPath != "" && language.Imports.Target == conventions.FolderTarget && path.Dir(ch.From) != path.Dir(nowPath) {
		gone := w.changedFiles(Change{Path: ch.From})
		return append(gone, changed{file: ChangedFile{Path: nowPath, Language: language.Name, State: NothingAltered}, language: language})
	}
	c := changed{file: ChangedFile{Path: nowPath, BasePath: basePath, Language: language.Name}, language: language}
	c.file.State, c.file.Reason, c.names = w.state(language, nowPath, basePath)
	return []changed{c}
}

func either(a, b string) string { return cmpOr(a, b) }

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// state decides a changed code file's state, with the names it altered or removed.
func (w *radiusWalk) state(language conventions.Language, nowPath, basePath string) (FileState, string, []string) {
	rules := w.in.rules
	nowContent, baseContent := w.in.nowContent[nowPath], w.in.baseContent[basePath]
	sides := []struct {
		p       string
		content []byte
	}{{nowPath, nowContent}, {basePath, baseContent}}
	for _, side := range sides {
		if side.p != "" && inFolders(side.p, rules.SkippedFolders) {
			return NotCounted, ReasonSkipped, nil
		}
	}
	for _, side := range sides {
		if side.p != "" && side.content == nil {
			return NotCounted, ReasonMissing, nil
		}
	}
	for _, side := range sides {
		if side.p != "" && (onPaths(rules, side.p, rules.GeneratedNames) || generatedOrBinary(rules, side.content)) {
			return NotCounted, ReasonGenerated, nil
		}
	}
	nowReading, baseReading := w.in.nowReadings[nowPath], w.in.beforeReadings[basePath]
	if slices.Contains(language.SurfaceFiles, path.Base(either(nowPath, basePath))) {
		nowReading.pureImports, baseReading.pureImports = nil, nil // what the package offers: no import is pure
	}
	switch {
	case basePath == "":
		return NothingAltered, "", nil // a new file
	case nowPath != "" && sameCode(language, nowContent, baseContent, nowReading, baseReading):
		return NothingAltered, "", nil
	case nowPath == "":
		if reason := unreadUnits(baseReading); reason != "" {
			return Deleted, reason, nil
		}
		var names []string
		for _, u := range baseReading.units {
			names = append(names, u.names...)
		}
		return Deleted, "", sortedNames(names)
	}
	for _, r := range []reading{baseReading, nowReading} {
		if reason := unreadUnits(r); reason != "" {
			return AsAWhole, reason, nil
		}
	}
	return compareUnits(language, nowContent, baseContent, nowReading, baseReading)
}

// unreadUnits is why a reading's units cannot be compared, or "". A syntax error is not one: a unit that holds it is.
func unreadUnits(r reading) string {
	switch {
	case r.unread:
		return ReasonNotRead
	case !r.unitsRead:
		return ReasonLineRead
	}
	return ""
}

// compareUnits pairs the units of both sides by fingerprint, position ignored.
func compareUnits(language conventions.Language, nowContent, baseContent []byte, now, base reading) (FileState, string, []string) {
	if remainder(language, nowContent, now) != remainder(language, baseContent, base) {
		return AsAWhole, ReasonRemainder, nil
	}
	nowPrints := map[uint64]int{}
	for _, u := range now.units {
		nowPrints[unitPrint(language, nowContent, u, now.pureImports)]++
	}
	var names []string
	for _, u := range base.units {
		if print := unitPrint(language, baseContent, u, base.pureImports); nowPrints[print] > 0 {
			nowPrints[print]--
			continue
		}
		if slices.ContainsFunc(base.errors, func(e span) bool { return e.start < u.end && u.start <= e.end }) {
			return AsAWhole, ReasonSyntaxError, nil
		}
		if len(u.names) == 0 {
			return AsAWhole, ReasonUnnamed, nil
		}
		for _, name := range u.names {
			switch {
			case identifier.MatchString(name):
			case u.inner != nil:
				return AsAWhole, ReasonRemainder, nil // a container's head, such as an #if condition, changed
			default:
				return AsAWhole, ReasonNoIdentifier, nil
			}
		}
		names = append(names, u.names...)
	}
	if len(names) == 0 {
		return NothingAltered, "", nil // units only added or reordered
	}
	return ByName, "", sortedNames(names)
}

// unitPrint is a unit's fingerprint: all its tokens, or, for a container, its head without the units inside it.
func unitPrint(language conventions.Language, content []byte, u unit, imports []span) uint64 {
	if u.inner == nil {
		return fingerprint(language, content, u.start, u.end, u.context, nil)
	}
	return fingerprint(language, content, u.start, u.end, u.context, slices.Concat(u.inner, imports))
}

var identifier = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_]*$`)

func sortedNames(names []string) []string {
	slices.Sort(names)
	return slices.Compact(names)
}

// sameCode says whether two contents hold the same code tokens: all of them, or, when both were parsed, those outside
// pure imports.
func sameCode(language conventions.Language, nowContent, baseContent []byte, now, base reading) bool {
	if fingerprint(language, nowContent, 0, len(nowContent), "", nil) == fingerprint(language, baseContent, 0, len(baseContent), "", nil) {
		return true
	}
	if !now.unitsRead || !base.unitsRead {
		return false
	}
	return fingerprint(language, nowContent, 0, len(nowContent), "", now.pureImports) ==
		fingerprint(language, baseContent, 0, len(baseContent), "", base.pureImports)
}

// remainder is the fingerprint of a reading's tokens outside its units and pure imports.
func remainder(language conventions.Language, content []byte, r reading) uint64 {
	outside := slices.Clone(r.pureImports)
	for _, u := range r.units {
		outside = append(outside, span{start: u.start, end: u.end - 1})
	}
	return fingerprint(language, content, 0, len(content), "", outside)
}

// fingerprint hashes, after a context, the code tokens of content[start:end] that lie in no span of skip, with the
// layout the language reads as code: a line break before a token on a new line, and the line's indentation.
func fingerprint(language conventions.Language, content []byte, start, end int, context string, skip []span) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(context)) // a hash's Write never fails
	text := content[start:end]
	line := 0
	for t := range eachToken(language.Tokens, text, 1) {
		if inSpans(skip, start+t.start) {
			continue
		}
		if t.line != line {
			if language.LayoutCode.LineBreaks && line > 0 {
				_, _ = h.Write([]byte{'\n'})
			}
			if language.LayoutCode.Indentation {
				_, _ = h.Write(text[lineStart(text, t.line):t.start])
			}
			line = t.line
		}
		writeUint64(h, t.hash)
	}
	return h.Sum64()
}

// seed puts the first items of one changed file on the walk.
func (w *radiusWalk) seed(c changed) {
	language := c.language.Name
	importers := w.importers(c.file.Path, c.file.BasePath)
	switch c.file.State {
	case NotCounted:
	case NothingAltered:
		w.renameSeeds(c)
	case AsAWhole:
		w.renameSeeds(c)
		w.pushWhole(slices.Concat(importers, w.peers(c)), language)
	case Deleted:
		whole := c.file.Reason != ""
		if c.language.Imports.Target == conventions.FolderTarget && w.folderLives(c.file.BasePath) && !whole {
			w.pushNames(importers, c.names, language)
		} else {
			w.pushWhole(importers, language)
		}
		if whole {
			w.pushWhole(w.peers(c), language)
		} else {
			w.pushNames(w.peers(c), c.names, language)
		}
	case ByName:
		w.renameSeeds(c)
		w.pushNames(slices.Concat(importers, w.peers(c)), c.names, language)
	}
}

// renameSeeds counts as a whole the files that imported a renamed file's old path and do not import it now, where
// imports name files, or where they name folders and the old folder is gone.
func (w *radiusWalk) renameSeeds(c changed) {
	if c.file.BasePath == "" || c.file.Path == "" || c.file.BasePath == c.file.Path {
		return
	}
	if c.language.Imports.Target == conventions.FolderTarget && w.folderLives(c.file.BasePath) {
		return
	}
	var stale []string
	for _, file := range w.toBase[c.file.BasePath] {
		if file = w.pathNow(file); file != "" && !slices.Contains(w.to[c.file.Path], file) {
			stale = append(stale, file)
		}
	}
	w.pushWhole(stale, c.language.Name)
}

// importers are the files that import a changed file now and those that imported its base path at the base, under
// their path now when they exist now.
func (w *radiusWalk) importers(nowPath, basePath string) []string {
	var found []string
	if nowPath != "" {
		found = append(found, w.to[nowPath]...)
	}
	if basePath != "" {
		for _, file := range w.toBase[basePath] {
			if file = w.pathNow(file); file != "" {
				found = append(found, file)
			}
		}
	}
	return found
}

// missingNow says whether a path is tracked now, of a language plimsoll reads, with its content missing.
func (w *radiusWalk) missingNow(p string) bool {
	_, known := w.in.rules.LanguageOf(p)
	content, tracked := w.in.nowContent[p]
	return known && tracked && content == nil
}

// pathNow is a base path's code file now, or "" when it is gone.
func (w *radiusWalk) pathNow(basePath string) string {
	if p, renamed := w.nowPath[basePath]; renamed {
		basePath = p
	}
	if _, code := w.roles[basePath]; !code {
		return ""
	}
	return basePath
}

// folderLives says whether a code file of the same language lies in a base path's folder now.
func (w *radiusWalk) folderLives(basePath string) bool {
	language, _ := w.in.rules.LanguageOf(basePath)
	return slices.ContainsFunc(w.in.now.Files, func(f File) bool {
		return f.Language == language.Name && path.Dir(f.Path) == path.Dir(basePath) && f.Path != basePath
	})
}

// peers are the code files now that use a changed file's declarations without importing it.
func (w *radiusWalk) peers(c changed) []string {
	file, content := c.file.Path, w.in.nowContent[c.file.Path]
	if file == "" {
		file, content = c.file.BasePath, w.in.baseContent[c.file.BasePath]
	}
	same := func(keep func(f File) bool) []string {
		var found []string
		for _, f := range w.in.now.Files {
			if f.Language == c.language.Name && f.Path != file && keep(f) {
				found = append(found, f.Path)
			}
		}
		return found
	}
	switch c.language.Peers {
	case conventions.FolderPeers:
		return same(func(f File) bool { return path.Dir(f.Path) == path.Dir(file) })
	case conventions.PackagePeers:
		line := c.language.Imports.PackageLine
		declared := packageOf(line, content)
		if declared == "" {
			return nil
		}
		return same(func(f File) bool { return packageOf(line, w.in.nowContent[f.Path]) == declared })
	case conventions.TargetPeers:
		target := targetFolder(file)
		return same(func(f File) bool { return targetFolder(f.Path) == target })
	case conventions.HeaderPeers:
		var found []string
		stem := strings.TrimSuffix(file, path.Ext(file))
		for _, suffix := range []string{".h", ".hpp", ".hh", ".hxx"} {
			if header := stem + suffix; header != file {
				if _, code := w.roles[header]; code {
					found = append(append(found, header), w.to[header]...)
				}
			}
		}
		return found
	case conventions.NoPeers:
	}
	return nil
}

// packageOf is the package or namespace a content declares by a package line, or "".
func packageOf(line *regexp.Regexp, content []byte) string {
	if line == nil {
		return ""
	}
	m := line.FindSubmatch(content)
	if m == nil {
		return ""
	}
	return string(m[line.SubexpIndex("package")])
}

// targetFolder is the folder of a Swift file's target: Sources, Tests or Plugins and the folder below, else its folder.
func targetFolder(file string) string {
	parts := strings.Split(file, "/")
	for i, part := range parts[:len(parts)-1] {
		if (part == "Sources" || part == "Tests" || part == "Plugins") && i+2 < len(parts) {
			return strings.Join(parts[:i+2], "/")
		}
	}
	return path.Dir(file)
}

// pushNames asks files for each of some names.
func (w *radiusWalk) pushNames(files, names []string, language string) {
	for _, file := range files {
		for _, name := range names {
			w.names[name] = true
			w.push(item{file: file, name: name}, language)
		}
	}
}

// pushWhole counts files as users of a file counted as a whole.
func (w *radiusWalk) pushWhole(files []string, language string) {
	for _, file := range files {
		w.push(item{file: file}, language)
	}
}

// push puts an item on the walk once and records the language it comes from.
func (w *radiusWalk) push(it item, language string) {
	if !slices.Contains(w.from[it], language) {
		w.from[it] = append(w.from[it], language)
	}
	if !w.visited[it] {
		w.visited[it] = true
		w.queue = append(w.queue, it)
	}
}

// run walks the items: a file whose content is missing now is unread; a file that only passes imports on hands the
// item to its importers and, asked for a name it holds, is a user too; asked for any use, a file is a user; asked for
// a name, a file that holds it is a user and asks its importers for it.
func (w *radiusWalk) run() {
	for len(w.queue) > 0 {
		it := w.queue[0]
		w.queue = w.queue[1:]
		languages := w.from[it]
		if w.missingNow(it.file) {
			w.mark(w.unread, it.file, false, false, languages)
			continue
		}
		if _, code := w.roles[it.file]; !code {
			continue
		}
		passes := w.passOn[it.file] || w.passBase[it.file] && !w.reaches[it.file]
		forward := passes
		switch {
		case it.name == "" && !passes:
			w.mark(w.users, it.file, false, false, languages)
		case it.name != "":
			if held := w.holds(it.file)[it.name]; held != 0 {
				w.mark(w.users, it.file, true, held == heldInTestsOnly, languages)
				forward = true
			}
		}
		if forward {
			for _, importer := range w.to[it.file] {
				for _, language := range languages {
					w.push(item{file: importer, name: it.name}, language)
				}
			}
		}
	}
}

const (
	heldOutsideTests = 1
	heldInTestsOnly  = 2
)

// holds gives, for one file now, the walk's names it holds as identifier tokens: outside its test code, or only in
// it. The file is tokenized once, and only when a plain word search finds one of the names in it.
func (w *radiusWalk) holds(file string) map[string]int {
	if found, read := w.held[file]; read {
		return found
	}
	found := map[string]int{}
	w.held[file] = found
	content := w.in.nowContent[file]
	hit := false
	eachWord(content, func(word []byte, _ int) { hit = hit || w.names[string(word)] })
	language, _ := w.in.rules.LanguageOf(file)
	if !hit || language.Tokens == nil {
		return found
	}
	testSpans := w.in.nowReadings[file].testSpans
	inTests := w.roles[file].IsTest()
	for t := range eachToken(language.Tokens, content, 1) {
		word := content[t.start:t.end]
		if !t.word || bytes.ContainsAny(word[:1], "\"'`") || !w.names[string(word)] {
			continue // not an identifier, or not asked for
		}
		switch {
		case !inTests && !inSpans(testSpans, t.start):
			found[string(word)] = heldOutsideTests
		case found[string(word)] == 0:
			found[string(word)] = heldInTestsOnly
		}
	}
	return found
}

// mark records a user or an unread file.
func (w *radiusWalk) mark(into map[string]*User, file string, byName, testsOnly bool, languages []string) {
	u := into[file]
	if u == nil {
		u = &User{Path: file, Role: w.roles[file], TestsOnly: true}
		into[file] = u
	}
	if byName {
		u.ByName, u.TestsOnly = true, u.TestsOnly && testsOnly
	}
	for _, language := range languages {
		if !slices.Contains(u.Languages, language) {
			u.Languages = append(u.Languages, language)
		}
	}
}

// results are the untouched users, the touched ones and the unread files, sorted by path.
func (w *radiusWalk) results() (users, touched, unread []User) {
	for _, u := range w.users {
		slices.Sort(u.Languages)
		u.TestsOnly = u.TestsOnly && u.ByName
		if w.touched[u.Path] {
			touched = append(touched, *u)
		} else {
			users = append(users, *u)
		}
	}
	for _, u := range w.unread {
		slices.Sort(u.Languages)
		unread = append(unread, *u)
	}
	byPath := func(a, b User) int { return strings.Compare(a.Path, b.Path) }
	slices.SortFunc(users, byPath)
	slices.SortFunc(touched, byPath)
	slices.SortFunc(unread, byPath)
	return users, touched, unread
}
