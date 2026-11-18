package snapshot

import (
	"math/bits"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// radiusWalk holds the links of both sides and the state of the walk.
type radiusWalk struct {
	in       radiusInput
	touched  map[string]bool
	nowPath  map[string]string   // a renamed file's base path to its path now
	to       map[string][]string // per code file: the files whose links of now use it (no module declaration)
	toBase   map[string][]string // the same at the base, for the changed files only
	reaches  map[string]bool     // files whose links of now reach a code file
	passOn   map[string]bool
	passBase map[string]bool
	roles    map[string]Role
	queue    []string              // files with names or a whole file to be asked, in the order they were offered
	files    map[string]*walkState // per file: what it was asked and what it holds
	names    map[string]int        // every name the walk asks for, by its index in the name sets
	nameFrom [][]string            // per name index: the languages of the changed files that altered or removed it
	users    map[string]*User
	unread   map[string]*User
	// nowPrints and basePrints are the fingerprints of the compared changed files on each side.
	nowPrints, basePrints map[string]printPair
	byFolder              map[string][]File // the code files now by folder, made on first use
	byPackage             map[string]map[string][]string
	byTarget              map[string][]string // per file: the imported folders whose links reach it
	folderReach           map[string][]string // per imported folder: the files its links reach
}

func newWalk(in radiusInput) *radiusWalk {
	w := &radiusWalk{in: in, touched: map[string]bool{}, nowPath: map[string]string{}, to: map[string][]string{},
		toBase: map[string][]string{}, reaches: map[string]bool{}, passOn: map[string]bool{}, passBase: map[string]bool{},
		roles: map[string]Role{}, files: map[string]*walkState{}, names: map[string]int{}, users: map[string]*User{},
		unread: map[string]*User{}, nowPrints: map[string]printPair{}, basePrints: map[string]printPair{}}
	for _, ch := range in.changes {
		w.touched[ch.Path] = true
		if ch.From != "" {
			w.touched[ch.From], w.nowPath[ch.From] = true, ch.Path
		}
	}
	// NOTE: the links detector's graph applies the same rule (a module's declaration is no use of it); the two cannot
	// share code across the detector boundary and must agree.
	for _, link := range in.now.Links.List {
		for _, file := range link.Files {
			if !link.Declares {
				w.to[file] = append(w.to[file], link.From)
			}
		}
		w.reaches[link.From] = w.reaches[link.From] || len(link.Files) > 0
	}
	for _, link := range in.before.Links.List {
		for _, file := range link.Files {
			if w.touched[file] && !link.Declares {
				w.toBase[file] = append(w.toBase[file], link.From)
			}
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

// seed puts the first items of one changed file on the walk.
func (w *radiusWalk) seed(c changed) {
	language := c.language.Name
	if c.file.State == NotCounted {
		return
	}
	if c.file.State == Deleted {
		importers, peers := w.importers(c.file.Path, c.file.BasePath), w.peers(c)
		switch {
		case c.file.Reason != "": // its units could not be read: every user counts
			w.pushWhole(slices.Concat(importers, peers), language)
		case c.language.Imports.Target == conventions.FolderTarget && w.folderLives(c.file.BasePath):
			w.pushNames(slices.Concat(importers, peers), c.names, language)
		default: // the path its importers name is gone
			w.pushWhole(importers, language)
			w.pushNames(peers, c.names, language)
		}
		return
	}
	w.renameSeeds(c)
	switch c.file.State {
	case AsAWhole:
		w.pushWhole(slices.Concat(w.importers(c.file.Path, c.file.BasePath), w.peers(c)), language)
	case ByName:
		w.pushNames(slices.Concat(w.importers(c.file.Path, c.file.BasePath), w.peers(c)), c.names, language)
	case NothingAltered, NotCounted, Deleted:
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
	still := map[string]bool{}
	for _, file := range w.to[c.file.Path] {
		still[file] = true
	}
	var stale []string
	for _, file := range w.toBase[c.file.BasePath] {
		if file = w.pathNow(file); file != "" && !still[file] {
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

// folderFiles are the code files now in one folder.
func (w *radiusWalk) folderFiles(folder string) []File {
	if w.byFolder == nil {
		w.byFolder = map[string][]File{}
		for _, f := range w.in.now.Files {
			w.byFolder[path.Dir(f.Path)] = append(w.byFolder[path.Dir(f.Path)], f)
		}
	}
	return w.byFolder[folder]
}

// folderLives says whether a code file of the same language lies in a base path's folder now.
func (w *radiusWalk) folderLives(basePath string) bool {
	language, _ := w.in.rules.LanguageOf(basePath)
	return slices.ContainsFunc(w.folderFiles(path.Dir(basePath)), func(f File) bool {
		return f.Language == language.Name && f.Path != basePath
	})
}

// peers are the code files now that use a changed file's declarations without importing it.
func (w *radiusWalk) peers(c changed) []string {
	file, content := c.file.Path, w.in.nowContent[c.file.Path]
	if file == "" {
		file, content = c.file.BasePath, w.in.baseContent[c.file.BasePath]
	}
	var found []string
	switch c.language.Peers {
	case conventions.FolderPeers:
		for _, f := range w.folderFiles(path.Dir(file)) {
			if f.Language == c.language.Name {
				found = append(found, f.Path)
			}
		}
	case conventions.PackagePeers:
		found = w.packageFiles(c.language)[packageOf(c.language.Imports.PackageLine, content)]
	case conventions.TargetPeers:
		found = w.targetFiles(file)
	case conventions.HeaderPeers:
		stem := strings.TrimSuffix(file, path.Ext(file))
		for _, suffix := range c.language.Headers() {
			if header := stem + suffix; header != file {
				if _, code := w.roles[header]; code {
					found = append(append(found, header), w.to[header]...)
				}
			}
		}
	case conventions.NoPeers:
	}
	return slices.DeleteFunc(slices.Clone(found), func(f string) bool { return f == file })
}

// packageFiles are the code files now of one language by the package their package line declares, made once.
func (w *radiusWalk) packageFiles(language conventions.Language) map[string][]string {
	if w.byPackage == nil {
		w.byPackage = map[string]map[string][]string{}
	}
	if found, made := w.byPackage[language.Name]; made {
		return found
	}
	found := map[string][]string{}
	for _, f := range w.in.now.Files {
		if f.Language != language.Name {
			continue
		}
		if declared := packageOf(language.Imports.PackageLine, w.in.nowContent[f.Path]); declared != "" {
			found[declared] = append(found[declared], f.Path)
		}
	}
	w.byPackage[language.Name] = found
	return found
}

// targetFiles are the files of the module a file belongs to, as the links that import a module's folders reach them:
// a Swift target, wherever its manifest puts it; a file no such link reaches has its folder's files.
func (w *radiusWalk) targetFiles(file string) []string {
	if w.byTarget == nil { // the imported folders that reach each file, and each such folder's files, once
		w.byTarget, w.folderReach = map[string][]string{}, map[string][]string{}
		for _, link := range w.in.now.Links.List {
			if _, seen := w.folderReach[link.To]; !link.Folder || seen {
				continue
			}
			w.folderReach[link.To] = link.Files
			for _, f := range link.Files {
				w.byTarget[f] = append(w.byTarget[f], link.To)
			}
		}
	}
	if folders := w.byTarget[file]; len(folders) > 0 {
		var found []string
		for _, folder := range folders {
			found = append(found, w.folderReach[folder]...)
		}
		return sortedNames(found)
	}
	var found []string
	for _, f := range w.folderFiles(path.Dir(file)) {
		found = append(found, f.Path)
	}
	return found
}

// packageOf is the package or namespace a content declares by a package line, or "".
func packageOf(line *regexp.Regexp, content []byte) string {
	if line == nil || content == nil {
		return ""
	}
	m := line.FindSubmatch(content)
	if m == nil {
		return ""
	}
	return string(m[line.SubexpIndex("package")])
}

// nameSet is a set of the walk's names, by index.
type nameSet []uint64

func (b *nameSet) add(i int) {
	for len(*b) <= i/64 {
		*b = append(*b, 0)
	}
	(*b)[i/64] |= 1 << (i % 64)
}

// and is the names in both b and c.
func (b nameSet) and(c nameSet) nameSet {
	out := make(nameSet, min(len(b), len(c)))
	for i := range out {
		out[i] = b[i] & c[i]
	}
	return out
}

func (b nameSet) empty() bool { return !slices.ContainsFunc(b, func(w uint64) bool { return w != 0 }) }

// each calls do for each name in the set, by index.
func (b nameSet) each(do func(i int)) {
	for word, bitsOf := range b {
		for bitsOf != 0 {
			do(word*64 + bits.TrailingZeros64(bitsOf))
			bitsOf &= bitsOf - 1
		}
	}
}

// walkState is what the walk asked of one file: the names offered to it, those not yet asked, the languages of the
// files counted as a whole whose user it is, and what it holds.
type walkState struct {
	seen, pending   nameSet
	whole, wholeNew []string // languages of the files counted as a whole that reach it; those not yet handed on
	queued          bool
	held, outside   nameSet // the walk's names it holds as identifiers; those it holds outside its test code
	heldRead        bool
}

func (w *radiusWalk) stateOf(file string) *walkState {
	st := w.files[file]
	if st == nil {
		st = &walkState{}
		w.files[file] = st
	}
	return st
}

// pushNames asks files for each of some names that a changed file of a language altered or removed.
func (w *radiusWalk) pushNames(files, names []string, language string) {
	var set nameSet
	for _, name := range names {
		i, known := w.names[name]
		if !known {
			i = len(w.nameFrom)
			w.names[name] = i
			w.nameFrom = append(w.nameFrom, nil)
		}
		w.nameFrom[i] = appendNew(w.nameFrom[i], language)
		set.add(i)
	}
	for _, file := range files {
		w.offer(file, set, "")
	}
}

// pushWhole counts files as users of a file counted as a whole, of a language.
func (w *radiusWalk) pushWhole(files []string, language string) {
	for _, file := range files {
		w.offer(file, nil, language)
	}
}

// offer gives a file the names it has not been asked yet, and, with a language, makes it a user of a file counted as
// a whole of that language; a file with anything new is queued once.
func (w *radiusWalk) offer(file string, names nameSet, wholeLanguage string) {
	st := w.stateOf(file)
	news := false
	if wholeLanguage != "" && !slices.Contains(st.whole, wholeLanguage) {
		st.whole, st.wholeNew, news = append(st.whole, wholeLanguage), append(st.wholeNew, wholeLanguage), true
	}
	for i, word := range names {
		for len(st.seen) <= i {
			st.seen = append(st.seen, 0)
		}
		for len(st.pending) <= i {
			st.pending = append(st.pending, 0)
		}
		if fresh := word &^ st.seen[i]; fresh != 0 {
			st.seen[i] |= fresh
			st.pending[i] |= fresh
			news = true
		}
	}
	if news && !st.queued {
		st.queued = true
		w.queue = append(w.queue, file)
	}
}

// languagesOf are the languages of the changed files that altered or removed the names of a set.
func (w *radiusWalk) languagesOf(names nameSet, more ...string) []string {
	found := slices.Clone(more)
	names.each(func(i int) { found = appendNew(found, w.nameFrom[i]...) })
	return found
}

// run walks the files: a file whose content is missing now is unread; a file that only passes imports on hands what
// it was asked to its importers and is a user of the names it holds; any other file is a user of a file counted as a
// whole that reaches it, and of the names it holds, which it hands to its importers. Each file is asked each name
// once, so the work grows with the files and the names that reach them, and the users are the same in any order.
func (w *radiusWalk) run() {
	fetched := 0 // the files whose identifiers are read ahead
	for len(w.queue) > 0 {
		if fetched == 0 {
			w.readAhead(w.queue)
			fetched = len(w.queue)
		}
		fetched--
		file := w.queue[0]
		w.queue = w.queue[1:]
		w.visit(file)
	}
}

// visit decides one file the walk reached with what it was asked since its last visit.
func (w *radiusWalk) visit(file string) {
	st := w.files[file]
	st.queued = false
	asked, wholeNew := st.pending, st.wholeNew
	st.pending, st.wholeNew = nil, nil
	if missingNow(w.in, file) {
		w.mark(w.unread, file, false, false, w.languagesOf(asked, wholeNew...))
		return
	}
	if _, code := w.roles[file]; !code {
		return
	}
	passes := w.passOn[file] || w.passBase[file] && !w.reaches[file]
	switch {
	case len(wholeNew) == 0:
	case passes:
		for _, importer := range w.to[file] {
			for _, language := range wholeNew {
				w.offer(importer, nil, language)
			}
		}
	default:
		w.mark(w.users, file, false, false, wholeNew)
	}
	if asked.empty() {
		return
	}
	w.readHeld(file, st)
	used := asked.and(st.held)
	if !used.empty() {
		w.mark(w.users, file, true, used.and(st.outside).empty(), w.languagesOf(used))
	}
	forward := used
	if passes {
		forward = asked
	}
	if !forward.empty() {
		for _, importer := range w.to[file] {
			w.offer(importer, forward, "")
		}
	}
}

// readAhead reads, on all processors, which names the queued files that are asked names hold: the walk then asks
// them in its own order from what is read.
func (w *radiusWalk) readAhead(files []string) {
	var wanted []string
	for _, file := range files {
		if st := w.files[file]; !st.heldRead && !st.pending.empty() {
			wanted = append(wanted, file)
		}
	}
	parallel(len(wanted), func(i int) { w.readHeld(wanted[i], w.files[wanted[i]]) })
}

// worth says whether a file holds one of the walk's names as a plain word.
func (w *radiusWalk) worth(file string) func() bool {
	return func() bool {
		hit := false
		eachWord(w.in.nowContent[file], func(word []byte, _ int) {
			if !hit {
				_, hit = w.names[string(word)]
			}
		})
		return hit
	}
}

// readHeld reads, once per file, which of the walk's names a file holds as identifier tokens, and which of them
// outside its test code. The file's identifiers are read once per content, and only after a plain word search finds
// one of the names. It changes the file's own state alone, so files are read at the same time.
func (w *radiusWalk) readHeld(file string, st *walkState) {
	if st.heldRead {
		return
	}
	st.heldRead = true
	language, _ := w.in.rules.LanguageOf(file)
	if language.Tokens == nil {
		return
	}
	inTests := w.roles[file].IsTest()
	identifiers := w.in.readings.identifiersOf(language, language.GrammarOf(file), w.in.nowContent[file], w.in.nowReadings[file].testSpans, w.in.question, w.worth(file))
	held := func(name string, outside bool) {
		if i, wanted := w.names[name]; wanted {
			st.held.add(i)
			if outside && !inTests {
				st.outside.add(i)
			}
		}
	}
	if len(identifiers.names) < len(w.names) { // walk the shorter of the two lists
		for i, name := range identifiers.names {
			held(name, identifiers.outsideTests[i])
		}
		return
	}
	for name := range w.names {
		if found, outside := identifiers.find(name); found {
			held(name, outside)
		}
	}
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
	u.Languages = appendNew(u.Languages, languages...)
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
