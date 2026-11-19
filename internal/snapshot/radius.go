package snapshot

import (
	"cmp"
	"path"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// FileState is what a change did to one changed code file, for the blast radius.
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
	ReasonIgnored      = "left out by the repository's own file"
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
// that use what it altered or removed. It holds no name, token or text of any file (trust).
type Radius struct {
	Changed []ChangedFile // sorted by path now, then path at the base
	Users   []User        // untouched users, sorted by path
	Touched []User        // users the change itself touched, sorted by path
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
	missing, unreadable         []string  // content not in a partial clone; files that could not be read now
	readings                    *Readings // where each reached file's identifiers are kept by content
	question                    uint64
}

// blastRadius decides the state of each changed code file and walks from their seeds to the users.
func blastRadius(in radiusInput) Radius {
	var r Radius
	for _, p := range slices.Concat(in.missing, in.unreadable) {
		r.MissingNow = r.MissingNow || missingNow(in, p)
	}
	if len(in.changes) == 0 {
		return r
	}
	w := newWalk(in)
	w.readPrints()
	var all []changed
	for _, ch := range in.changes {
		all = append(all, w.changedFiles(ch)...)
	}
	for _, c := range all {
		r.Changed = append(r.Changed, c.file)
		w.seed(c)
	}
	slices.SortFunc(r.Changed, func(a, b ChangedFile) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), strings.Compare(a.BasePath, b.BasePath))
	})
	w.run()
	r.Users, r.Touched = w.results()
	return r
}

// missingNow says whether a path is tracked now, of a language plimsoll reads and outside what it leaves out, with its
// content missing.
func missingNow(in radiusInput, p string) bool {
	_, known := in.rules.LanguageOf(p)
	content, tracked := in.nowContent[p]
	return known && tracked && content == nil && !excluded(in.rules, p)
}

// changed is a changed code file with what the walk needs of it.
type changed struct {
	file     ChangedFile
	language conventions.Language
	names    []string // altered or removed
}

// compared says whether a change's two sides are both code that the comparison of units reads: tracked on both
// sides, read, in no skipped folder and not generated. The others need no fingerprints.
func compared(in radiusInput, nowPath, basePath string) bool {
	for _, side := range []struct {
		p       string
		content []byte
	}{{nowPath, in.nowContent[nowPath]}, {basePath, in.baseContent[basePath]}} {
		if side.p == "" || side.content == nil || excluded(in.rules, side.p) ||
			onPaths(in.rules, side.p, in.rules.GeneratedNames) || generatedOrBinary(in.rules, side.content) {
			return false
		}
	}
	return true
}

// readPrints makes the fingerprints of every change whose two sides are compared, on all processors: tokenizing a
// large changed file is the slow part of comparing it.
func (w *radiusWalk) readPrints() {
	type job struct {
		now      bool
		path     string
		content  []byte
		reading  reading
		language conventions.Language
	}
	var jobs []job
	for _, ch := range w.in.changes {
		basePath := cmp.Or(ch.From, ch.Path)
		language, known := w.in.rules.LanguageOf(ch.Path)
		if !known || language.Tokens == nil || !compared(w.in, ch.Path, basePath) {
			continue
		}
		jobs = append(jobs, job{true, ch.Path, w.in.nowContent[ch.Path], w.in.nowReadings[ch.Path], language},
			job{false, basePath, w.in.baseContent[basePath], w.in.beforeReadings[basePath], language})
	}
	results := make([]prints, len(jobs))
	parallel(len(jobs), func(i int) {
		results[i] = w.in.readings.printsOfReading(jobs[i].language, jobs[i].language.GrammarOf(jobs[i].path), jobs[i].content, jobs[i].reading, w.in.question)
	})
	for i, j := range jobs {
		if j.now {
			w.nowPrints[j.path] = results[i]
		} else {
			w.basePrints[j.path] = results[i]
		}
	}
}

// parallel calls do for 0 to n-1 on one goroutine per processor, and returns when every call has.
func parallel(n int, do func(i int)) {
	work := make(chan int)
	var group sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), n) {
		group.Go(func() {
			for i := range work {
				do(i)
			}
		})
	}
	for i := range n {
		work <- i
	}
	close(work)
	group.Wait()
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
	nowLanguage, nowKnown := w.in.rules.LanguageOf(nowPath)
	baseLanguage, baseKnown := w.in.rules.LanguageOf(basePath)
	nowKnown, baseKnown = nowKnown && nowPath != "", baseKnown && basePath != ""
	switch {
	case !nowKnown && !baseKnown:
		return nil
	case !nowKnown && nowPath != "": // code renamed to a suffix no entry reads: its old path is gone
		return w.changedFiles(Change{Path: basePath})
	case !baseKnown:
		basePath = "" // new, or renamed from a suffix no entry reads
	}
	language := nowLanguage
	if !nowKnown {
		language = baseLanguage
	}
	if ch.From != "" && nowPath != "" && basePath != "" && language.Imports.Target == conventions.FolderTarget && path.Dir(ch.From) != path.Dir(nowPath) {
		gone := w.changedFiles(Change{Path: ch.From})
		moved := changed{file: ChangedFile{Path: nowPath, Language: language.Name}, language: language}
		moved.file.State, moved.file.Reason, moved.names = w.state(language, nowPath, "")
		return append(gone, moved)
	}
	c := changed{file: ChangedFile{Path: nowPath, BasePath: basePath, Language: language.Name}, language: language}
	c.file.State, c.file.Reason, c.names = w.state(language, nowPath, basePath)
	return []changed{c}
}

// state decides a changed code file's state, with the names it altered or removed. The reasons not to count it are
// checked in order over both sides: a skipped folder, then missing content, then generated code.
func (w *radiusWalk) state(language conventions.Language, nowPath, basePath string) (FileState, string, []string) {
	rules := w.in.rules
	nowContent, baseContent := w.in.nowContent[nowPath], w.in.baseContent[basePath]
	sides := []struct {
		p       string
		content []byte
	}{{nowPath, nowContent}, {basePath, baseContent}}
	for _, side := range sides {
		switch {
		case side.p == "":
		case ignored(rules, side.p):
			return NotCounted, ReasonIgnored, nil
		case excluded(rules, side.p):
			return NotCounted, ReasonSkipped, nil
		}
	}
	for _, side := range sides {
		if side.p != "" && side.content == nil && slices.Contains(w.in.missing, side.p) {
			return NotCounted, ReasonMissing, nil
		}
	}
	for _, side := range sides { // content that could not be read for another reason is binary
		if side.p != "" && (side.content == nil || onPaths(rules, side.p, rules.GeneratedNames) || generatedOrBinary(rules, side.content)) {
			return NotCounted, ReasonGenerated, nil
		}
	}
	nowReading, baseReading := w.in.nowReadings[nowPath], w.in.beforeReadings[basePath]
	nowPrints, basePrints := w.nowPrints[nowPath], w.basePrints[basePath]
	switch {
	case basePath == "":
		return NothingAltered, "", nil // a new file
	case nowPath != "" && sameCode(nowPrints, basePrints, nowReading, baseReading):
		return NothingAltered, "", nil
	case nowPath == "":
		if reason := unreadUnits(baseReading); reason != "" {
			return Deleted, reason, nil
		}
		if len(baseReading.errors) > 0 { // a broken declaration's names may be wrong: its importers count as a whole
			return Deleted, ReasonSyntaxError, nil
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
	holdsError := func(r reading, u unit) bool {
		return slices.ContainsFunc(r.errors, func(e span) bool { return e.start < u.end && u.start <= e.end })
	}
	for _, i := range unmatched(nowPrint.units, basePrint.units) {
		if holdsError(now, now.units[i]) {
			return AsAWhole, ReasonSyntaxError, nil
		}
	}
	var names []string
	for _, i := range unmatched(basePrint.units, nowPrint.units) {
		u := base.units[i]
		switch {
		case holdsError(base, u):
			return AsAWhole, ReasonSyntaxError, nil
		case u.inner != nil:
			return AsAWhole, ReasonRemainder, nil // a container's head, such as a namespace's name or an #if condition, changed
		case len(u.names) == 0:
			return AsAWhole, ReasonUnnamed, nil
		case slices.ContainsFunc(u.names, func(n string) bool { return !identifier.MatchString(n) }):
			return AsAWhole, ReasonNoIdentifier, nil
		}
		names = append(names, u.names...)
	}
	if len(names) == 0 {
		return NothingAltered, "", nil // units only added or reordered
	}
	return ByName, "", sortedNames(names)
}

// unmatched are the indexes of a side's unit prints the other side does not hold, each print counted as often as it
// occurs.
func unmatched(side, other []uint64) []int {
	held := map[uint64]int{}
	for _, p := range other {
		held[p]++
	}
	var found []int
	for i, p := range side {
		if held[p] > 0 {
			held[p]--
			continue
		}
		found = append(found, i)
	}
	return found
}

var identifier = regexp.MustCompile(`^[\p{L}_$][\p{L}\p{N}_$]*$`)

func sortedNames(names []string) []string {
	slices.Sort(names)
	return slices.Compact(names)
}

// sameCode says whether two readings hold the same code tokens: all of them, or, when both were read for their units,
// those outside pure imports.
func sameCode(nowPrint, basePrint prints, now, base reading) bool {
	return nowPrint.whole == basePrint.whole || now.unitsRead && base.unitsRead && nowPrint.code == basePrint.code
}
