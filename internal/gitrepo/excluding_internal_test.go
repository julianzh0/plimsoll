package gitrepo

import (
	"slices"
	"testing"
)

func TestExcludingNamesAFolderWithoutContentOnce(t *testing.T) {
	missing := []string{"data/x/a.bin", "data/x/b.bin", "src/c.ts", "gone/old.ts"}
	tracked := []string{"data/x/a.bin", "data/x/b.bin", "src/c.ts", "src/d.ts"}
	want := []string{"--", ".", ":(exclude)data", ":(exclude)gone", ":(exclude)src/c.ts"}
	if got := excluding(missing, tracked); !slices.Equal(got, want) {
		t.Errorf("excluding(%q, %q) = %q, want %q", missing, tracked, got, want)
	}
	if got := excluding(nil, tracked); got != nil {
		t.Errorf("excluding(nothing missing) = %q, want nil", got)
	}
}
