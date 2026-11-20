package snapshot

import (
	"strings"
	"testing"
)

// A pattern of many ** against a deep path that it does not match tries each pattern and path position once: the
// matcher would otherwise retry the same positions exponentially often.
func TestGlobMatchesTriesEachPositionOnce(t *testing.T) {
	pattern := "src/" + strings.Repeat("**/", 12) + "x.ts"
	file := "src/" + strings.Repeat("d/", 40) + "y.ts"
	if globMatches(pattern, file) {
		t.Errorf("globMatches(%q, %q) = true, want false", pattern, file)
	}
	if !globMatches(pattern, "src/"+strings.Repeat("d/", 40)+"x.ts") {
		t.Errorf("globMatches(%q, a deep x.ts) = false, want true", pattern)
	}
}
