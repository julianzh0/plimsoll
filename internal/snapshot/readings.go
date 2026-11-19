package snapshot

import (
	"crypto/sha256"
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

// KeptReadings is how many file readings may be kept between questions before readings no later question used are
// dropped: more than 25 times the code files of the largest field repository (json, 634). A kept reading with its key
// takes about 1.4 KB of heap (4,078 readings of the sixteen field repositories took 5.8 MB), so the bound holds about
// 23 MB. Only changed files are read with their declarations (2.4 KB a reading) or with their functions too (5.6 KB;
// the 217 code files with function tables the field repositories changed in their last 20 commits): were every kept
// reading one with functions, the bound would hold about 92 MB. A question never loses the readings it made itself: one larger than the bound keeps all of them, so the next
// question about the same repository reads only what changed (swc, 43,434 code files: about 60 MB).
const KeptReadings = 16384

// Readings keeps what each file's content says, by content, between questions: a second question about unchanged
// files does not read them again. It may be used from any number of goroutines; questions that need the same content
// at the same time share one reading.
type Readings struct {
	parser    *parse.Parser
	bound     int
	mu        sync.Mutex
	kept      map[readingKey]*keptReading
	questions uint64 // questions begun
	// identifiers are what the blast radius walk asks of the files it reaches, and prints the fingerprints of the
	// changed files it compares; each depends on a content alone and is kept and dropped like the readings.
	identifiers byContent[identifiers]
	prints      byContent[prints]
}

// printsOfReading gives the prints of a content read with its units by a grammar, made once per content and grammar
// (one entry's grammars split the same content into different units).
func (rs *Readings) printsOfReading(language conventions.Language, grammar string, content []byte, r reading, question uint64) prints {
	return keptOrMade(rs, &rs.prints, contentKey("prints\x00"+language.Name+"\x00"+grammar, content), question, func() (prints, bool) {
		return printsOf(language, content, r.units, r.pureImports), !r.unread // a failure of the moment is not kept
	})
}

// identifiers are the distinct identifier tokens of one content, sorted, each with whether it stands outside its test
// code: a sorted list, not a map, since the walk may keep those of tens of thousands of files.
type identifiers struct {
	names        []string
	outsideTests []bool
}

// find says whether the content holds a name, and whether outside its test code.
func (ids identifiers) find(name string) (held, outside bool) {
	i, found := slices.BinarySearch(ids.names, name)
	return found, found && ids.outsideTests[i]
}

// identifiersOf gives the identifier tokens of a content: each with whether it stands outside the test spans its
// grammar read, made once per content and grammar. When they are not kept yet and worth is false for the content, it
// gives none, reads nothing and keeps nothing; those of a reading the parser could not finish are not kept either.
func (rs *Readings) identifiersOf(language conventions.Language, grammar string, content []byte, r reading, question uint64, worth func() bool) identifiers {
	testSpans := r.testSpans
	return keptOrMade(rs, &rs.identifiers, contentKey("identifiers\x00"+language.Name+"\x00"+grammar, content), question, func() (identifiers, bool) {
		if !worth() {
			return identifiers{}, false
		}
		found := map[string]bool{}
		for t := range eachToken(language.Tokens, content, 1) {
			if !t.identifier {
				continue
			}
			outside := !inSpans(testSpans, t.start)
			if was, seen := found[string(content[t.start:t.end])]; !seen || outside && !was {
				found[string(content[t.start:t.end])] = outside
			}
		}
		ids := identifiers{names: slices.Sorted(maps.Keys(found))}
		for _, name := range ids.names {
			ids.outsideTests = append(ids.outsideTests, found[name])
		}
		return ids, !r.unread
	})
}

// readingKey names a reading: a reading depends only on the content, the entry, the grammar and the binary.
type readingKey struct {
	entry, grammar string
	declarations   bool // the reading holds the file's declarations too
	functions      bool // the reading holds the file's measured functions too
	places         bool // the reading holds only its functions' places and tokens
	content        [sha256.Size]byte
}

// keptReading is a reading, or the promise of one: read is closed once r is made. question is the latest question
// that used it.
type keptReading struct {
	read     chan struct{}
	r        reading
	question uint64
}

// NewReadings returns empty readings that read with the parser.
func NewReadings(parser *parse.Parser) *Readings {
	return &Readings{parser: parser, bound: KeptReadings, kept: map[readingKey]*keptReading{}}
}

// begin numbers a new question; the readings it uses are kept until a later question needs the room.
func (rs *Readings) begin() uint64 {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.questions++
	return rs.questions
}

// end closes a question: when more readings are kept than the bound, it drops those that no question from this one
// on has used. Dropping only once a question is done keeps every older reading the question goes on to use after
// its first new content (one edit used to drop them all: swc's next question took 31.8 s instead of 5 s).
func (rs *Readings) end(question uint64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if len(rs.kept) > rs.bound {
		maps.DeleteFunc(rs.kept, func(_ readingKey, kept *keptReading) bool { return kept.question < question })
	}
	rs.identifiers.drop(question, rs.bound)
	rs.prints.drop(question, rs.bound)
}

// wanted is what a reading holds beside the entry's query: the file's declarations, its functions, and the rules that
// find a function's repeated blocks. Those rules are the binary's own (no repository sets them), so they are not part of
// a reading's key.
type wanted struct {
	declarations, functions bool
	places                  bool // only the functions' places, names and leaf tokens (readPlaces)
	functionRules           *conventions.FunctionRules
}

// of returns the reading of one file's content for its language entry and grammar, made once per content.
func (rs *Readings) of(language conventions.Language, grammar string, content []byte, question uint64, w wanted) reading {
	key := readingKey{entry: language.Name, grammar: grammar, declarations: w.declarations, functions: w.functions, places: w.places, content: sha256.Sum256(content)}
	rs.mu.Lock()
	if kept, found := rs.kept[key]; found {
		kept.question = max(kept.question, question)
		rs.mu.Unlock()
		<-kept.read
		return kept.r
	}
	kept := &keptReading{read: make(chan struct{}), question: question}
	rs.kept[key] = kept
	rs.mu.Unlock()
	r, lasting := rs.read(language, grammar, content, w)
	kept.r = r
	close(kept.read)
	if !lasting {
		rs.mu.Lock()
		if rs.kept[key] == kept {
			delete(rs.kept, key)
		}
		rs.mu.Unlock()
	}
	return r
}

// side reads, for one question, the code files of one side that have a language entry, from content with its byte
// order mark dropped. A file declares names is read with its entry's declarations query too: the files a change names;
// a file in measures is read with its functions.
func (rs *Readings) side(rules conventions.Rules, content map[string][]byte, files []File, question uint64, declares, measures map[string]bool) map[string]reading {
	found := map[string]reading{}
	for _, file := range files {
		language, known := rules.LanguageOf(file.Path)
		if !known || content[file.Path] == nil {
			continue
		}
		w := wanted{declarations: declares[file.Path], functions: measures[file.Path], functionRules: &rules.FunctionRules}
		found[file.Path] = rs.of(language, language.GrammarOf(file.Path), content[file.Path], question, w)
	}
	return found
}

// syntaxErrorPattern is added to every entry's query: a node the grammar could not fit into the language, or a token
// it had to assume (a missing parenthesis), as an unfinished edit leaves. The grammar recovers after it, but a statement next to it may be lost, and nothing else
// says so.
const syntaxErrorPattern = "\n(ERROR) @error\n(MISSING) @error\n"

// read makes the reading of one file: by the entry's grammar when it names one, and every fact for which the entry
// has a pattern by that pattern (no entry reads one fact both ways), over the file with its comments and strings
// blanked when it is parsed. A file the parser cannot read is unread; its patterns still read it. For an entry that
// reads broken files line by line, the facts of a file the grammar cannot read, or reads only with an error, come from
// the entry's text patterns alone; its functions still come from the tree (none when the parser read nothing).
//
// lasting is false for a failure of the moment (parse.ErrUnavailable): such a reading answers this question and is not
// kept, so a later question reads the file again.
func (rs *Readings) read(language conventions.Language, grammar string, content []byte, w wanted) (r reading, lasting bool) {
	if grammar == "" {
		return readByText(language, content), true
	}
	if w.places {
		return rs.readPlaces(language, grammar, content, *w.functionRules)
	}
	query := language.Query
	if w.declarations {
		query += language.Declarations + language.Units
	}
	result, err := rs.parser.Read(grammar, query+syntaxErrorPattern, content)
	if err != nil && language.LineReadWhenBroken {
		r = readByText(language, content)
		r.functionsUnread = w.functions // the grammar read nothing, so no function either
		return r, !errors.Is(err, parse.ErrUnavailable)
	}
	if err != nil {
		r = reading{unread: true}
		return r, !errors.Is(err, parse.ErrUnavailable)
	}
	r = decode(language, result, content)
	r.unitsRead = w.declarations // the units query runs with the declarations query
	if r.syntaxError && language.LineReadWhenBroken {
		// the imports by the text patterns; the functions and the units still from the tree, each flagged where it holds
		// the error
		tree := r
		r = readByText(language, content)
		r.units, r.pureImports, r.unitsRead, r.errors = tree.units, tree.pureImports, tree.unitsRead, tree.errors
	}
	if w.functions {
		tree, err := rs.parser.Nodes(grammar, content)
		if err != nil {
			r.functionsUnread = true
			return r, !errors.Is(err, parse.ErrUnavailable)
		}
		pass := functionPass{leaf: leafKindsFor(grammar, tree)}
		functions := functionsOf(language.Functions, language.Tokens, *w.functionRules, tree, content, nil, pass)
		functions, err = rs.readRewritten(language, grammar, content, *w.functionRules, functions, pass)
		if err != nil { // a failure of the moment on the rewritten copy: answer now, read the file again later
			r.functionsUnread = true
			return r, false
		}
		for _, f := range functions {
			if !inSpans(r.testSpans, f.start) {
				r.functions = append(r.functions, f)
			}
		}
	}
	return r, true
}

// readByText is the reading of one file by its entry's text patterns.
func readByText(language conventions.Language, content []byte) reading {
	var r reading
	if len(language.Imports.Statements) > 0 {
		r = scan(language.Imports, masked(language.Literals, content))
	}
	return r
}

// readRewritten reads, for a language with macro rules and a file with functions it could not read as written, a
// copy with those macros rewritten, and takes its functions where the written file gave none it could read. A copy
// the parser cannot read leaves the functions as they were.
//
// The error is a failure of the moment (parse.ErrUnavailable) only; any other failure of the copy is no error.
func (rs *Readings) readRewritten(language conventions.Language, grammar string, content []byte, rules conventions.FunctionRules,
	functions []function, pass functionPass) ([]function, error) {
	if len(language.Macros) == 0 || !slices.ContainsFunc(functions, func(f function) bool { return f.broken }) {
		return functions, nil
	}
	rewritten := rewriteMacros(language.Macros, content)
	if rewritten == nil {
		return functions, nil
	}
	tree, err := rs.parser.Nodes(grammar, rewritten)
	if errors.Is(err, parse.ErrUnavailable) {
		return functions, err
	}
	if err != nil {
		return functions, nil
	}
	var clean []function
	for _, f := range functions {
		if !f.broken {
			clean = append(clean, f)
		}
	}
	// a function read cleanly as written is not measured again on the copy
	within := func(start, end int) bool { return !overlapsAny(clean, function{start: start, end: end}) }
	if pass.leaf != nil {
		pass.leaf = leafKindsFor(grammar, tree)
	}
	return withRewritten(functions, functionsOf(language.Functions, language.Tokens, rules, tree, rewritten, within, pass)), nil
}

// readPlaces reads one file for its functions' places, names and leaf tokens only: no query, no measure. The rule that
// asks for it takes the file's test spans from the question's own reading.
func (rs *Readings) readPlaces(language conventions.Language, grammar string, content []byte, rules conventions.FunctionRules) (reading, bool) {
	tree, err := rs.parser.Nodes(grammar, content)
	if err != nil {
		return reading{functionsUnread: true}, !errors.Is(err, parse.ErrUnavailable)
	}
	pass := functionPass{leaf: leafKindsFor(grammar, tree), light: true}
	functions, err := rs.readRewritten(language, grammar, content, rules, functionsOf(language.Functions, language.Tokens, rules, tree, content, nil, pass), pass)
	if err != nil {
		return reading{functionsUnread: true}, false
	}
	return reading{functions: functions}, true
}

// leafKindsFor resolves a grammar's token kinds against a tree; nil for a grammar without them.
func leafKindsFor(grammar string, tree parse.Tree) *leafKinds {
	kinds, known := conventions.TokenKindsOf(grammar)
	if !known {
		return nil
	}
	resolved := leafKindsOf(tree, kinds)
	return &resolved
}
