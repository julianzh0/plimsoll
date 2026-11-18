package snapshot

import (
	"crypto/sha256"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/parse"
)

func TestAReadingTheParserCouldNotStartIsNotKept(t *testing.T) {
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	parser.Close() // a closed parser fails for the moment, not for the file
	readings := NewReadings(parser)
	language, _ := conventions.Default().LanguageOf("a.go")
	if r := readings.of(language, "go", []byte("package a\n"), readings.begin(), wanted{}); !r.unread {
		t.Fatalf("of(a closed parser) = %+v, want unread", r)
	}
	if len(readings.kept) != 0 {
		t.Errorf("readings kept %d entries after a failure of the moment, want none", len(readings.kept))
	}
}

func TestAFullCacheDropsOnlyReadingsNoLaterQuestionUsed(t *testing.T) {
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	readings := NewReadings(parser)
	readings.bound = 2
	language, _ := conventions.Default().LanguageOf("a.go")
	a, b, c := []byte("package a\n"), []byte("package b\n"), []byte("package c\n")
	first := readings.begin()
	readings.of(language, "go", a, first, wanted{})
	readings.of(language, "go", b, first, wanted{})
	readings.end(first)
	second := readings.begin()
	readings.of(language, "go", a, second, wanted{})
	readings.of(language, "go", c, second, wanted{})
	readings.end(second)
	kept := func(content []byte) bool {
		_, found := readings.kept[readingKey{entry: language.Name, grammar: "go", content: sha256.Sum256(content)}]
		return found
	}
	if !kept(a) || kept(b) || !kept(c) {
		t.Errorf("kept a, b, c = %v, %v, %v after a second question read a and c over a bound of 2, want true, false, true",
			kept(a), kept(b), kept(c))
	}
}

func TestOneQuestionKeepsEveryReadingItMadeOverTheBound(t *testing.T) {
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	readings := NewReadings(parser)
	readings.bound = 1
	language, _ := conventions.Default().LanguageOf("a.go")
	question := readings.begin()
	for _, content := range []string{"package a\n", "package b\n", "package c\n"} {
		readings.of(language, "go", []byte(content), question, wanted{})
	}
	readings.end(question)
	if len(readings.kept) != 3 {
		t.Errorf("readings kept %d entries after one question read 3 files over a bound of 1, want 3", len(readings.kept))
	}
}

// An edit gives one file new content: the next question reads it first and must still find every older reading it
// goes on to use (swc after one edit: 31.8 s instead of 5 s when they were dropped at the first new content).
func TestAQuestionOverTheBoundKeepsTheOlderReadingsItUsesAfterNewContent(t *testing.T) {
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(parser.Close)
	readings := NewReadings(parser)
	readings.bound = 2
	language, _ := conventions.Default().LanguageOf("a.go")
	a, b, edited := []byte("package a\n"), []byte("package b\n"), []byte("package a // edited\n")
	first := readings.begin()
	readings.of(language, "go", a, first, wanted{})
	readings.of(language, "go", b, first, wanted{})
	readings.end(first)
	keyOf := func(content []byte) readingKey {
		return readingKey{entry: language.Name, grammar: "go", content: sha256.Sum256(content)}
	}
	before := readings.kept[keyOf(b)]
	second := readings.begin()
	readings.of(language, "go", edited, second, wanted{})
	readings.of(language, "go", b, second, wanted{})
	if after := readings.kept[keyOf(b)]; after != before {
		t.Errorf("of(b) in a second question after new content = a new reading, want the one the first question made")
	}
}
