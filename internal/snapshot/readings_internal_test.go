package snapshot

import (
	"testing"

	"github.com/juzhxng/plimsoll/internal/conventions"
	"github.com/juzhxng/plimsoll/internal/parse"
)

func TestAReadingTheParserCouldNotStartIsNotKept(t *testing.T) {
	parser, err := parse.New()
	if err != nil {
		t.Fatal(err)
	}
	parser.Close() // a closed parser fails for the moment, not for the file
	readings := NewReadings(parser)
	language, _ := conventions.Default().LanguageOf("a.go")
	if r := readings.of(language, "go", []byte("package a\n")); !r.unread {
		t.Fatalf("of(a closed parser) = %+v, want unread", r)
	}
	if len(readings.kept) != 0 {
		t.Errorf("readings kept %d entries after a failure of the moment, want none", len(readings.kept))
	}
}
