//go:build integration

package gitrepo

import (
	"fmt"
	"slices"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/gitrepo/gitrepotest"
	"github.com/julianzh0/plimsoll/internal/snapshot"
	"github.com/julianzh0/plimsoll/internal/snapshot/snapshottest"
)

// repeatsRead reads a repository through the reader and lists the function-repeat rule's pairs.
func repeatsRead(t *testing.T, dir, base string) []string {
	t.Helper()
	source, err := read(t, dir, base)
	if err != nil {
		t.Fatalf("Read(%s) = %v", base, err)
	}
	var pairs []string
	for _, r := range snapshot.Build(source, conventions.Default(), snapshottest.Readings(t)).Repeats.Listed {
		pairs = append(pairs, fmt.Sprintf("%s %s <- %s %s", r.Path, r.Name, r.Counterpart.Path, r.Counterpart.Name))
	}
	return pairs
}

// TestIntegrationRepeatsFollowTheReadersOwnInputs checks the function-repeat rule on what the reader itself decides:
// the working tree, the repository's own file, the merge base and its rename detection.
func TestIntegrationRepeatsFollowTheReadersOwnInputs(t *testing.T) {
	col, fil, file := snapshottest.GoColumns, snapshottest.GoFiles, snapshottest.GoFile
	copied := []string{"pkg/b.go Copied <- pkg/a.go Original"}

	t.Run("an uncommitted change of a tracked file", func(t *testing.T) {
		repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"pkg/a.go": file(col("Original")), "pkg/b.go": file()}})
		repo.Write("pkg/b.go", file(col("Copied")))
		if got := repeatsRead(t, repo.Dir, "HEAD"); !slices.Equal(got, copied) {
			t.Errorf("repeats = %q, want %q", got, copied)
		}
	})

	t.Run("a counterpart in a folder the repository declares as tests", func(t *testing.T) {
		repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{
			"helpers/a.go": file(col("Original")), ".plimsoll.toml": "[layout]\ntest_folders = [\"helpers\"]\n"}})
		repo.Commit(gitrepotest.Commit{Message: "copy", Files: map[string]string{"pkg/b.go": file(col("Copied"))}})
		if got := repeatsRead(t, repo.Dir, "HEAD~1"); len(got) != 0 {
			t.Errorf("repeats = %q, want none: the original is test code", got)
		}
	})

	t.Run("the merge base, not the asked base", func(t *testing.T) {
		repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"pkg/a.go": file(col("Original"))}})
		repo.Git("checkout", "-q", "-b", "feature")
		repo.Commit(gitrepotest.Commit{Message: "copy", Files: map[string]string{"pkg/b.go": file(col("Copied"))}})
		repo.Git("checkout", "-q", "main")
		repo.Commit(gitrepotest.Commit{Message: "the same copy on main", Files: map[string]string{"pkg/b.go": file(col("Copied"))}})
		repo.Git("checkout", "-q", "feature")
		// against main's tip pkg/b.go would be unchanged; against the merge base the feature added it
		if got := repeatsRead(t, repo.Dir, "main"); !slices.Equal(got, copied) {
			t.Errorf("repeats = %q, want %q", got, copied)
		}
	})

	t.Run("a function kept in a renamed file", func(t *testing.T) {
		kept := file(fil("Helper"), fil("Other"), fil("Third"))
		repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{
			"pkg/a.go": file(col("Original")), "pkg/old.go": kept, "README.md": "x\n", "docs/old.md": "y\n"}})
		repo.Git("mv", "pkg/old.go", "pkg/new.go")
		repo.Git("mv", "docs/old.md", "docs/new.md")
		// the renamed file now also holds a function that repeats Original; Helper, Other and Third were there before
		repo.Write("pkg/new.go", kept+"\n"+col("Added"))
		got := repeatsRead(t, repo.Dir, "HEAD")
		if want := []string{"pkg/new.go Added <- pkg/a.go Original"}; !slices.Equal(got, want) {
			t.Errorf("repeats = %q, want %q: only the added function is new", got, want)
		}
	})
}
