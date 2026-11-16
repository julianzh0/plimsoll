package snapshot

import (
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/parse"
)

// KeptReadings is how many file readings are kept between questions: more than 25 times the code files of the largest
// field repository (json, 634). A kept reading with its key takes about 1.4 KB of heap (4,078 readings of the sixteen
// field repositories took 5.8 MB), so the bound holds about 23 MB.
//
// NOTE: when a new reading would exceed the bound, every kept reading is dropped first, so a repository with more code
// files than the bound is read again on every question. Upgrade: drop the least recently used.
const KeptReadings = 16384

// Readings keeps what each file's content says, by content, between questions: a second question about unchanged
// files does not read them again. It may be used from any number of goroutines; questions that need the same content
// at the same time share one reading.
type Readings struct {
	parser *parse.Parser
	mu     sync.Mutex
	kept   map[readingKey]*keptReading
}

// readingKey names a reading: a reading depends only on the content, the entry, the grammar and the binary.
type readingKey struct {
	entry, grammar string
	content        [sha256.Size]byte
}

// keptReading is a reading, or the promise of one: read is closed once r is made.
type keptReading struct {
	read chan struct{}
	r    reading
}

// NewReadings returns empty readings that read with the parser.
func NewReadings(parser *parse.Parser) *Readings {
	return &Readings{parser: parser, kept: map[readingKey]*keptReading{}}
}

// of returns the reading of one file's content for its language entry and grammar, made once per content.
func (rs *Readings) of(language conventions.Language, grammar string, content []byte) reading {
	key := readingKey{entry: language.Name, grammar: grammar, content: sha256.Sum256(content)}
	rs.mu.Lock()
	if kept, found := rs.kept[key]; found {
		rs.mu.Unlock()
		<-kept.read
		return kept.r
	}
	if len(rs.kept) >= KeptReadings {
		rs.kept = map[readingKey]*keptReading{}
	}
	kept := &keptReading{read: make(chan struct{})}
	rs.kept[key] = kept
	rs.mu.Unlock()
	r, lasting := rs.read(language, grammar, content)
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

// side reads the code files of one side that have a language entry, from content with its byte order mark dropped.
func (rs *Readings) side(rules conventions.Rules, content map[string][]byte, files []File) map[string]reading {
	found := map[string]reading{}
	for _, file := range files {
		language, known := rules.LanguageOf(file.Path)
		if !known || content[file.Path] == nil {
			continue
		}
		found[file.Path] = rs.of(language, language.GrammarOf(file.Path), content[file.Path])
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
func (rs *Readings) read(language conventions.Language, grammar string, content []byte) (r reading, lasting bool) {
	if grammar == "" {
		return readByText(language, content), true
	}
	result, err := rs.parser.Read(grammar, language.Query+syntaxErrorPattern, content)
	if err != nil && language.LineReadWhenBroken {
		return readByText(language, content), !errors.Is(err, parse.ErrUnavailable)
	}
	if err != nil {
		r = reading{unread: true}
		readPatterns(language, content, &r)
		return r, !errors.Is(err, parse.ErrUnavailable)
	}
	r = decode(language, result, content)
	if r.syntaxError && language.LineReadWhenBroken {
		return readByText(language, content), true
	}
	return r, true
}

// readByText is the reading of one file by its entry's text patterns.
func readByText(language conventions.Language, content []byte) reading {
	var r reading
	if len(language.Imports.Statements) > 0 {
		r = scan(language.Imports, masked(language.Literals, content))
	}
	readPatterns(language, content, &r)
	return r
}

// readPatterns reads the computed settings reads when the entry has a pattern for them.
func readPatterns(language conventions.Language, content []byte, r *reading) {
	if language.DynamicRead != nil {
		r.dynamic = len(language.DynamicRead.FindAllIndex(content, -1))
	}
}
