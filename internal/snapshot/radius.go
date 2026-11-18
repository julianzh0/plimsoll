package snapshot

import (
	"path"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

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
	readings                    *Readings // where each reached file's identifiers are kept by content
	question                    uint64
}

// blastRadius decides the state of each changed code file and walks from their seeds to the users.
func blastRadius(in radiusInput) Radius {
	var r Radius
	w := newWalk(in)
	for _, p := range in.missing {
		r.MissingNow = r.MissingNow || w.missingNow(p)
	}
	w.readPrints()
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
	queue    []string              // files with names or a whole file to be asked, in the order they were offered
	files    map[string]*walkState // per file: what it was asked and what it holds
	names    map[string]int        // every name the walk asks for, by its index in the name sets
	nameFrom [][]string            // per name index: the languages of the changed files that altered or removed it
	users    map[string]*User
	unread   map[string]*User
	// nowPrints and basePrints are the fingerprints of the changed files on each side: pure imports left out, and not.
	nowPrints, basePrints map[string][2]prints
}

// readPrints makes the fingerprints of every changed file on both sides that was read for its units, one goroutine per
// processor: tokenizing a large changed file is the slow part of comparing it.
func (w *radiusWalk) readPrints() {
	type job struct {
		into     map[string][2]prints
		path     string
		content  []byte
		reading  reading
		language conventions.Language
	}
	var jobs []job
	w.nowPrints, w.basePrints = map[string][2]prints{}, map[string][2]prints{}
	add := func(into map[string][2]prints, p string, content []byte, r reading) {
		if language, known := w.in.rules.LanguageOf(p); known && content != nil && language.Tokens != nil {
			jobs = append(jobs, job{into, p, content, r, language})
		}
	}
	for _, ch := range w.in.changes {
		add(w.nowPrints, ch.Path, w.in.nowContent[ch.Path], w.in.nowReadings[ch.Path])
		for _, p := range []string{ch.Path, ch.From} {
			if p != "" {
				add(w.basePrints, p, w.in.baseContent[p], w.in.beforeReadings[p])
			}
		}
	}
	results := make([][2]prints, len(jobs))
	work := make(chan int)
	var group sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(jobs)) {
		group.Go(func() {
			for i := range work {
				pure, all := w.in.readings.printsOfReading(jobs[i].language, jobs[i].content, jobs[i].reading, w.in.question)
				results[i] = [2]prints{pure, all}
			}
		})
	}
	for i := range jobs {
		work <- i
	}
	close(work)
	group.Wait()
	for i, j := range jobs {
		j.into[j.path] = results[i]
	}
}

func newWalk(in radiusInput) *radiusWalk {
	w := &radiusWalk{in: in, touched: map[string]bool{}, nowPath: map[string]string{}, to: map[string][]string{},
		toBase: map[string][]string{}, reaches: map[string]bool{}, passOn: map[string]bool{}, passBase: map[string]bool{},
		roles: map[string]Role{}, files: map[string]*walkState{}, names: map[string]int{}, users: map[string]*User{},
		unread: map[string]*User{}}
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
			if w.touched[file] { // only a changed file's base importers are asked
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
	variant := 0
	if slices.Contains(language.SurfaceFiles, path.Base(either(nowPath, basePath))) {
		variant = 1 // what the package offers: no import is pure
	}
	nowPrints, basePrints := w.nowPrints[nowPath][variant], w.basePrints[basePath][variant]
	switch {
	case basePath == "":
		return NothingAltered, "", nil // a new file
	case nowPath != "" && sameCode(nowPrints, basePrints, nowReading, baseReading):
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
	return compareUnits(nowPrints, basePrints, nowReading, baseReading)
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
func compareUnits(nowPrint, basePrint prints, now, base reading) (FileState, string, []string) {
	if nowPrint.remainder != basePrint.remainder {
		return AsAWhole, ReasonRemainder, nil
	}
	nowPrints := map[uint64]int{}
	for _, print := range nowPrint.units {
		nowPrints[print]++
	}
	var names []string
	for i, u := range base.units {
		if print := basePrint.units[i]; nowPrints[print] > 0 {
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

var identifier = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_]*$`)

func sortedNames(names []string) []string {
	slices.Sort(names)
	return slices.Compact(names)
}

// sameCode says whether two readings hold the same code tokens: all of them, or, when both were read for their units,
// those outside pure imports.
func sameCode(nowPrint, basePrint prints, now, base reading) bool {
	return nowPrint.whole == basePrint.whole || now.unitsRead && base.unitsRead && nowPrint.code == basePrint.code
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

// nameSet is a set of the walk's names, by index.
type nameSet []uint64

func (b nameSet) has(i int) bool { return i/64 < len(b) && b[i/64]&(1<<(i%64)) != 0 }

func (b *nameSet) add(i int) {
	for len(*b) <= i/64 {
		*b = append(*b, 0)
	}
	(*b)[i/64] |= 1 << (i % 64)
}

// minus is the names of b that are not in c.
func (b nameSet) minus(c nameSet) nameSet {
	out := slices.Clone(b)
	for i := range min(len(out), len(c)) {
		out[i] &^= c[i]
	}
	return out
}

// and is the names in both b and c.
func (b nameSet) and(c nameSet) nameSet {
	out := make(nameSet, min(len(b), len(c)))
	for i := range out {
		out[i] = b[i] & c[i]
	}
	return out
}

func (b *nameSet) or(c nameSet) {
	for len(*b) < len(c) {
		*b = append(*b, 0)
	}
	for i := range c {
		(*b)[i] |= c[i]
	}
}

func (b nameSet) empty() bool { return !slices.ContainsFunc(b, func(w uint64) bool { return w != 0 }) }

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
		if !slices.Contains(w.nameFrom[i], language) {
			w.nameFrom[i] = append(w.nameFrom[i], language)
		}
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
	if fresh := names.minus(st.seen); !fresh.empty() {
		st.seen.or(fresh)
		st.pending.or(fresh)
		news = true
	}
	if news && !st.queued {
		st.queued = true
		w.queue = append(w.queue, file)
	}
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
		st := w.files[file]
		st.queued = false
		asked, wholeNew := st.pending, st.wholeNew
		st.pending, st.wholeNew = nil, nil
		languages := slices.Clone(wholeNew)
		for i := range len(w.nameFrom) {
			if asked.has(i) {
				languages = appendNew(languages, w.nameFrom[i]...)
			}
		}
		if w.missingNow(file) {
			w.mark(w.unread, file, false, false, languages)
			continue
		}
		if _, code := w.roles[file]; !code {
			continue
		}
		passes := w.passOn[file] || w.passBase[file] && !w.reaches[file]
		if len(wholeNew) > 0 {
			if passes {
				for _, importer := range w.to[file] {
					for _, language := range wholeNew {
						w.offer(importer, nil, language)
					}
				}
			} else {
				w.mark(w.users, file, false, false, wholeNew)
			}
		}
		if asked.empty() {
			continue
		}
		w.readHeld(file, st)
		used := asked.and(st.held)
		forward := used
		if !used.empty() {
			var usedFrom []string
			for i := range len(w.nameFrom) {
				if used.has(i) {
					usedFrom = appendNew(usedFrom, w.nameFrom[i]...)
				}
			}
			w.mark(w.users, file, true, used.and(st.outside).empty(), usedFrom)
		}
		if passes {
			forward = asked
		}
		if !forward.empty() {
			for _, importer := range w.to[file] {
				w.offer(importer, forward, "")
			}
		}
	}
}

// readAhead reads the identifiers of the queued files that are asked names, one goroutine per processor: the walk
// then asks them in its own order from what is kept.
func (w *radiusWalk) readAhead(files []string) {
	var wanted []string
	for _, file := range files {
		if st := w.files[file]; !st.heldRead && !st.pending.empty() {
			wanted = append(wanted, file)
		}
	}
	work := make(chan string)
	var group sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), len(wanted)) {
		group.Go(func() {
			for file := range work {
				if language, _ := w.in.rules.LanguageOf(file); language.Tokens != nil {
					_ = w.in.readings.identifiersOf(language, w.in.nowContent[file], w.in.nowReadings[file].testSpans, w.in.question, w.worth(file))
				}
			}
		})
	}
	for _, file := range wanted {
		work <- file
	}
	close(work)
	group.Wait()
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

// readHeld reads which of the walk's names a file holds as identifier tokens, and which of them outside its test
// code. The file's identifiers are read once per content, and only after a plain word search finds one of the names.
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
	identifiers := w.in.readings.identifiersOf(language, w.in.nowContent[file], w.in.nowReadings[file].testSpans, w.in.question, w.worth(file))
	held := func(name string, outside bool) {
		if i, wanted := w.names[name]; wanted {
			st.held.add(i)
			if outside && !inTests {
				st.outside.add(i)
			}
		}
	}
	if len(identifiers.names) < len(w.names) {
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
