package snapshot

import (
	"crypto/sha256"
	"errors"
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
	dropped   uint64 // the latest question that dropped older readings: at most once per question
}

// readingKey names a reading: a reading depends only on the content, the entry, the grammar and the binary.
type readingKey struct {
	entry, grammar string
	declarations   bool // the reading holds the file's declarations too
	functions      bool // the reading holds the file's measured functions too
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

// dropOlder drops the readings that no question from this one on has used. It runs with mu held.
func (rs *Readings) dropOlder(question uint64) {
	for key, kept := range rs.kept {
		if kept.question < question {
			delete(rs.kept, key)
		}
	}
}

// wanted is what a reading holds beside the entry's query: the file's declarations, its functions, and the rules that
// find a function's repeated blocks. Those rules are the binary's own (no repository sets them), so they are not part of
// a reading's key.
type wanted struct {
	declarations, functions bool
	functionRules           *conventions.FunctionRules
}

// of returns the reading of one file's content for its language entry and grammar, made once per content.
func (rs *Readings) of(language conventions.Language, grammar string, content []byte, question uint64, w wanted) reading {
	key := readingKey{entry: language.Name, grammar: grammar, declarations: w.declarations, functions: w.functions, content: sha256.Sum256(content)}
	rs.mu.Lock()
	if kept, found := rs.kept[key]; found {
		kept.question = max(kept.question, question)
		rs.mu.Unlock()
		<-kept.read
		return kept.r
	}
	if len(rs.kept) >= rs.bound && rs.dropped < question {
		rs.dropOlder(question)
		rs.dropped = question
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

// syntaxErrorPattern is added to every entry's query: a node the grammar could not fit into the language, as an
// unfinished edit leaves. The grammar recovers after it, but a statement next to it may be lost, and nothing else
// says so.
const syntaxErrorPattern = "\n(ERROR) @error\n"

// read makes the reading of one file: by the entry's grammar when it names one, and every fact for which the entry
// has a pattern by that pattern (no entry reads one fact both ways), over the file with its comments and strings
// blanked when it is parsed. A file the parser cannot read is unread; its patterns still read it. For an entry that
// reads broken files line by line, a file the grammar cannot read, or reads only with an error, is read by the
// entry's text patterns alone.
//
// lasting is false for a failure of the moment (parse.ErrUnavailable): such a reading answers this question and is not
// kept, so a later question reads the file again.
func (rs *Readings) read(language conventions.Language, grammar string, content []byte, w wanted) (r reading, lasting bool) {
	if grammar == "" {
		return readByText(language, content), true
	}
	query := language.Query
	if w.declarations {
		query += language.Declarations
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
	if r.syntaxError && language.LineReadWhenBroken {
		// the imports by the text patterns; the functions still from the tree, each flagged where it holds the error
		r = readByText(language, content)
	}
	if w.functions {
		tree, err := rs.parser.Nodes(grammar, content)
		if err != nil {
			r.functionsUnread = true
			return r, !errors.Is(err, parse.ErrUnavailable)
		}
		functions := functionsOf(*language.Functions, language.Tokens, *w.functionRules, tree, content)
		functions = rs.readBlanked(language, grammar, content, *w.functionRules, functions)
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

// readBlanked reads, for a language with attribute macros and a file with functions it could not read as written, a
// copy with those macros blanked, and takes its functions where the written file gave none it could read. A copy the
// parser cannot read leaves the functions as they were.
func (rs *Readings) readBlanked(language conventions.Language, grammar string, content []byte, rules conventions.FunctionRules,
	functions []function) []function {
	if language.AttributeMacros == nil || !slices.ContainsFunc(functions, func(f function) bool { return f.broken }) {
		return functions
	}
	blanked, changed := blankMacros(language.AttributeMacros, content)
	if !changed {
		return functions
	}
	tree, err := rs.parser.Nodes(grammar, blanked)
	if err != nil {
		return functions
	}
	return withBlanked(functions, functionsOf(*language.Functions, language.Tokens, rules, tree, blanked))
}
