package snapshottest

import (
	"testing"

	"github.com/juzhxng/plimsoll/internal/parse/parsetest"
	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Readings returns empty readings on the parser of this test binary: a test keeps nothing another test made.
func Readings(t testing.TB) *snapshot.Readings {
	t.Helper()
	return snapshot.NewReadings(parsetest.Parser(t))
}
