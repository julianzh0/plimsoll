// Package parsetest gives the tests of one test binary one parser. Compiling a grammar takes up to 1.5 s and 720 MB
// under the race detector, so tests share the parser instead of each making their own; it lives until the binary ends.
package parsetest

import (
	"sync"
	"testing"

	"github.com/julianzh0/plimsoll/internal/parse"
)

var shared = sync.OnceValues(parse.New)

// Parser returns the one parser of this test binary, failing the test when it cannot be made.
func Parser(t testing.TB) *parse.Parser {
	t.Helper()
	parser, err := shared()
	if err != nil {
		t.Fatalf("parse.New() error = %v", err)
	}
	return parser
}
