package snapshot

import (
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/parse"
)

// KeptReadings is how many file readings are kept between questions: more than 25 times the code files of the largest
// field repository (json, 634). A reading is a few hundred bytes.
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
	kept.r = rs.read(language, grammar, content)
	close(kept.read)
	return kept.r
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

// read makes the reading of one file: by the entry's grammar when it names one, then every fact for which the entry
// still has a pattern by that pattern. A file the parser cannot read is unread; its patterns still read it.
func (rs *Readings) read(language conventions.Language, grammar string, content []byte) reading {
	if grammar == "" {
		return readByText(language, content)
	}
	var r reading
	if result, err := rs.parser.Read(grammar, language.Query, content); err != nil {
		r.unread = true
	} else {
		r = decode(result, content)
	}
	byText := readByText(language, content)
	if language.TestBlock != nil {
		r.testLines = byText.testLines
	}
	if language.DynamicRead != nil {
		r.dynamic = byText.dynamic
	}
	if language.SettingsPrefix != nil {
		r.prefixes = byText.prefixes
	}
	return r
}

// readByText is the reading of one file by its entry's text patterns.
func readByText(language conventions.Language, content []byte) reading {
	var r reading
	if len(language.Imports.Statements) > 0 {
		r = scan(language.Imports, masked(language.Literals, content))
	}
	r.testLines = testLines(language, content)
	if language.DynamicRead != nil {
		r.dynamic = len(language.DynamicRead.FindAllIndex(content, -1))
	}
	if language.SettingsPrefix != nil {
		group := language.SettingsPrefix.SubexpIndex("prefix")
		for _, m := range language.SettingsPrefix.FindAllSubmatch(content, -1) {
			if prefix := string(m[group]); !slices.Contains(r.prefixes, prefix) {
				r.prefixes = append(r.prefixes, prefix)
			}
		}
	}
	return r
}
