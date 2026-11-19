//go:build integration

package gitrepo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/gitrepo/gitrepotest"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func TestIntegrationReadLeavesTheRepositoryUnchanged(t *testing.T) {
	cases := map[string]func(repo *gitrepotest.Repo){
		"a stale index entry": func(repo *gitrepotest.Repo) {
			later := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			if err := os.Chtimes(filepath.Join(repo.Dir, "a.ts"), later, later); err != nil {
				t.Fatal(err)
			}
		},
		"a file monitor in the configuration": func(repo *gitrepotest.Repo) { repo.Git("config", "core.fsmonitor", "true") },
		"a clean filter that writes": func(repo *gitrepotest.Repo) {
			repo.Commit(gitrepotest.Commit{Message: "filtered", Files: map[string]string{".gitattributes": "*.dat filter=x\n", "a.dat": "one\n"}})
			repo.Git("config", "filter.x.clean", "touch .git/WRITTEN_BY_FILTER; cat")
			repo.Write("a.dat", "two\n")
		},
		"staged and unstaged changes": func(repo *gitrepotest.Repo) {
			repo.Write("a.ts", "changed\n")
			repo.Git("add", "a.ts")
			repo.Write("b.ts", "unstaged\n")
		},
	}
	for name, prepare := range cases {
		t.Run(name, func(t *testing.T) {
			repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n", "b.ts": "b\n", ".plimsoll.toml": "[defaults]\nbase = \"main\"\n"}})
			prepare(repo)
			before := gitrepotest.Fingerprint(t, repo.Dir)
			for _, base := range []string{"HEAD", ""} {
				if _, err := read(t, repo.Dir, base); err != nil {
					t.Fatalf("Read(%q) = %v", base, err)
				}
			}
			if got := gitrepotest.Fingerprint(t, repo.Dir); got != before {
				t.Errorf("Read changed the repository folder")
			}
			if _, err := os.Stat(filepath.Join(repo.Dir, ".git", "fsmonitor--daemon")); err == nil {
				t.Errorf("Read started a file monitor daemon")
			}
		})
	}
}

func TestIntegrationReadParsesTheHistory(t *testing.T) {
	repo := gitrepotest.New(t)
	base := repo.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{"docs/a b.md": "a\n"}})
	repo.Commit(gitrepotest.Commit{Message: "umlaut", Files: map[string]string{"src/grüße.ts": "a\nb\n"}})
	repo.Commit(gitrepotest.Commit{Message: "newline", Files: map[string]string{"src/line\nbreak.ts": "a\n"}})
	repo.Git("mv", "src/grüße.ts", "src/moved.ts")
	repo.Commit(gitrepotest.Commit{Message: "rename"})
	repo.Commit(gitrepotest.Commit{Message: "binary", Files: map[string]string{"logo.bin": "BIN\x00"}})
	repo.Git("switch", "-q", "-c", "side")
	repo.Commit(gitrepotest.Commit{Message: "side", Files: map[string]string{"side.ts": "s\n"}})
	repo.Git("switch", "-q", "main")
	repo.Git("merge", "-q", "--no-ff", "-m", "merge", "side")

	src, err := read(t, repo.Dir, base)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	var got [][]snapshot.FileChange
	for _, commit := range src.Commits {
		got = append(got, commit.Files)
	}
	want := [][]snapshot.FileChange{
		{{Path: "side.ts", Added: 1}},
		{{Path: "logo.bin"}},
		{{Path: "src/moved.ts"}},
		{{Path: "src/line\nbreak.ts", Added: 1}},
		{{Path: "src/grüße.ts", Added: 2}},
		{{Path: "docs/a b.md", Added: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Read.Commits files = %+v, want %+v", got, want)
	}
	if len(src.Since) != 6 || src.Since[len(src.Since)-1] == base {
		t.Errorf("Read.Since = %v, want the six commits after the base, newest first", src.Since)
	}
	if src.Commits[len(src.Commits)-1].ID != base {
		t.Errorf("Read.Commits ends with %s, want the base commit %s", src.Commits[len(src.Commits)-1].ID, base)
	}
}

func TestIntegrationReadIsTheSameEverywhere(t *testing.T) {
	repo := gitrepotest.New(t,
		gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}},
		gitrepotest.Commit{Message: "second", Files: map[string]string{"b.ts": "b\n"}},
	)
	first, err := read(t, repo.Dir, "HEAD~1")
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if second, _ := read(t, repo.Dir, "HEAD~1"); !reflect.DeepEqual(first, second) {
		t.Errorf("two reads differ: %+v and %+v", first, second)
	}
	clone := filepath.Join(t.TempDir(), "clone")
	repo.Git("clone", "-q", repo.Dir, clone)
	if fromClone, _ := read(t, clone, "HEAD~1"); !reflect.DeepEqual(first, fromClone) {
		t.Errorf("a clone reads %+v, want %+v", fromClone, first)
	}
	other := gitrepotest.New(t, gitrepotest.Commit{Message: "other", Files: map[string]string{"x.ts": "x\n"}})
	redirected := NewReader(append(os.Environ(), "GIT_DIR="+filepath.Join(other.Dir, ".git")))
	got, err := redirected.Read(context.Background(), repo.Dir, "HEAD~1", snapshot.WantsOf(conventions.Default()))
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Errorf("a reader with GIT_DIR set reads %+v, %v; want the named repository", got, err)
	}
}

func TestIntegrationKeptHistoryEqualsAFreshRead(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	reader := NewReader(os.Environ())
	wants := snapshot.WantsOf(conventions.Default())
	first, err := reader.Read(context.Background(), repo.Dir, "HEAD", wants)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	kept, _ := reader.Read(context.Background(), repo.Dir, "HEAD", wants)
	fresh, _ := NewReader(os.Environ()).Read(context.Background(), repo.Dir, "HEAD", wants)
	if !reflect.DeepEqual(kept, first) || !reflect.DeepEqual(kept, fresh) {
		t.Errorf("a kept history reads %+v, want %+v", kept, fresh)
	}
	next := repo.Commit(gitrepotest.Commit{Message: "next", Files: map[string]string{"b.ts": "b\n"}})
	after, _ := reader.Read(context.Background(), repo.Dir, "HEAD", wants)
	if len(after.Commits) != 2 || after.Commits[0].ID != next {
		t.Errorf("after a new commit the reader keeps %d commits, newest %v; want 2, %s", len(after.Commits), after.Commits, next)
	}
}

// Questions may be answered at the same time; they share one reader and its kept histories.
func TestIntegrationReadsAtTheSameTimeShareOneReader(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	reader := NewReader(os.Environ())
	wants := snapshot.WantsOf(conventions.Default())
	fresh, err := NewReader(os.Environ()).Read(context.Background(), repo.Dir, "HEAD", wants)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	var wg sync.WaitGroup
	sources := make([]snapshot.Source, 4)
	for i := range sources {
		wg.Go(func() { sources[i], _ = reader.Read(context.Background(), repo.Dir, "HEAD", wants) })
	}
	wg.Wait()
	for i, source := range sources {
		if !reflect.DeepEqual(source, fresh) {
			t.Errorf("read %d at the same time = %+v, want %+v", i, source, fresh)
		}
	}
}

func TestIntegrationReadIgnoresCommitSignatures(t *testing.T) {
	repo := gitrepotest.New(t)
	key := filepath.Join(t.TempDir(), "key")
	if output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key).CombinedOutput(); err != nil {
		t.Fatalf("ssh-keygen: %v\n%s", err, output)
	}
	repo.Git("config", "gpg.format", "ssh")
	repo.Git("config", "user.signingkey", key)
	repo.Git("config", "commit.gpgsign", "true")
	repo.Git("config", "log.showSignature", "true")
	repo.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	repo.Commit(gitrepotest.Commit{Message: "signed", Files: map[string]string{"b.ts": "b\n"}})
	src, err := read(t, repo.Dir, "HEAD~1")
	if err != nil || len(src.Commits) != 2 {
		t.Fatalf("Read of signed commits = %d commits, %v; want 2 and no refusal", len(src.Commits), err)
	}
}

// A partial clone lacks the content of old commits and would fetch it from its remote on demand; a read must not.
func TestIntegrationReadFetchesNothingFromARemote(t *testing.T) {
	origin := gitrepotest.New(t,
		gitrepotest.Commit{Message: "base", Files: map[string]string{"src/a.ts": "old\n"}},
		gitrepotest.Commit{Message: "change", Files: map[string]string{"src/a.ts": "new\n"}},
	)
	origin.Git("config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", "--filter=blob:none", "file://"+origin.Dir, clone).CombinedOutput(); err != nil {
		t.Fatalf("partial clone: %v: %s", err, out)
	}
	objects := func() string {
		out, err := exec.Command("git", "-C", clone, "count-objects", "-v").Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	before := objects()
	_, _ = read(t, clone, "HEAD~1") // the old content is missing; whether the read refuses or reads around it, it fetches nothing
	if after := objects(); after != before {
		t.Errorf("Read fetched objects from the remote:\nbefore %s\nafter %s", before, after)
	}
}

// TestIntegrationContentMissingOnlyInBetweenIsAvailable reads a partial clone in which two changed paths lack content
// only in a commit between the base and the head: one the change added, one whose base content is there.
func TestIntegrationContentMissingOnlyInBetweenIsAvailable(t *testing.T) {
	big := strings.Repeat("x\n", 20000) // 40 kB, over the clone's limit
	origin := gitrepotest.New(t,
		gitrepotest.Commit{Message: "base", Files: map[string]string{"src/kept.ts": "a\n"}},
		gitrepotest.Commit{Message: "big for a while", Files: map[string]string{"src/kept.ts": big, "src/added.ts": big}},
		gitrepotest.Commit{Message: "small again", Files: map[string]string{"src/kept.ts": "b\n", "src/added.ts": "c\n"}},
	)
	origin.Git("config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", "--filter=blob:limit=10k", "file://"+origin.Dir, clone).CombinedOutput(); err != nil {
		t.Fatalf("partial clone: %v: %s", err, out)
	}
	got, err := read(t, clone, "HEAD~2")
	if err != nil || len(got.Missing) != 2 || len(got.Unavailable) != 0 {
		t.Errorf("Read(partial clone, HEAD~2) = %v, Missing %q, Unavailable %q; want both missing from history, none unavailable", err, got.Missing, got.Unavailable)
	}
}

func TestIntegrationAPartialCloneIsReadAroundItsMissingContent(t *testing.T) {
	big := func(fill string) string { return strings.Repeat(fill+"\n", 20000) } // 40 kB, over the clone's limit
	origin := gitrepotest.New(t,
		gitrepotest.Commit{Message: "base", Files: map[string]string{"src/a.ts": "a\n", "data/big.txt": big("1"), "package.json": `{"dependencies": {"x": "1"}}`}},
		gitrepotest.Commit{Message: "grow", Files: map[string]string{"src/a.ts": "a\nb\n", "data/big.txt": big("2")}},
		gitrepotest.Commit{Message: "only the big file", Files: map[string]string{"data/big.txt": big("4")}},
		gitrepotest.Commit{Message: "change", Files: map[string]string{"src/a.ts": "a\nb\nc\n", "src/b.ts": "b\n", "data/big.txt": big("3")}},
	)
	origin.Git("config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", "--filter=blob:limit=10k", "file://"+origin.Dir, clone).CombinedOutput(); err != nil {
		t.Fatalf("partial clone: %v: %s", err, out)
	}
	partial, err := read(t, clone, "HEAD~3")
	if err != nil {
		t.Fatalf("Read(partial clone) = %v, want an answer", err)
	}
	if want := []string{"data/big.txt"}; !slices.Equal(partial.Missing, want) {
		t.Errorf("Read(partial clone).Missing = %q, want %q", partial.Missing, want)
	}
	// it changed since the base and its content there is missing: unavailable to a rule that compares the two states
	if want := []string{"data/big.txt"}; !slices.Equal(partial.Unavailable, want) {
		t.Errorf("Read(partial clone, HEAD~3).Unavailable = %q, want %q", partial.Unavailable, want)
	}
	// against the head itself nothing changed: content missing only from older commits is no gap
	if same, err := read(t, clone, "HEAD"); err != nil || len(same.Unavailable) != 0 {
		t.Errorf("Read(partial clone, HEAD) = %v, Unavailable %q; want none", err, same.Unavailable)
	}
	full, err := read(t, origin.Dir, "HEAD~3")
	if err != nil {
		t.Fatal(err)
	}
	without := func(source snapshot.Source) ([]snapshot.Commit, []snapshot.Change) {
		var commits []snapshot.Commit
		for _, c := range source.Commits {
			c.Files = slices.DeleteFunc(slices.Clone(c.Files), func(f snapshot.FileChange) bool { return f.Path == "data/big.txt" })
			if len(c.Files) == 0 {
				c.Files = nil // a commit that only changed the missing file stays, with no files
			}
			commits = append(commits, c)
		}
		return commits, slices.DeleteFunc(slices.Clone(source.Changes), func(c snapshot.Change) bool { return c.Path == "data/big.txt" })
	}
	fullCommits, fullChanges := without(full)
	// the missing file changed: it is listed, with no lines counted
	fullChanges = append(fullChanges, snapshot.Change{Path: "data/big.txt"})
	slices.SortFunc(fullChanges, func(a, b snapshot.Change) int { return strings.Compare(a.Path, b.Path) })
	if !reflect.DeepEqual(partial.Commits, fullCommits) || !reflect.DeepEqual(partial.Changes, fullChanges) {
		t.Errorf("partial clone: commits %+v, changes %+v; want the full clone's without data/big.txt: %+v, %+v", partial.Commits, partial.Changes, fullCommits, fullChanges)
	}
	if !reflect.DeepEqual(partial.Now, full.Now) {
		t.Errorf("partial clone: Now = %v, want the full clone's %v", paths(partial.Now), paths(full.Now))
	}
}

// A partial clone can lack the old content of more files than one command line can name; the read goes on.
func TestIntegrationAPartialCloneMissingManyFilesIsRead(t *testing.T) {
	folder := "data/" + strings.Repeat("a-long-folder-name/", 4)
	old, now := map[string]string{"src/a.ts": "a\n"}, map[string]string{"src/a.ts": "a\nb\n"}
	for i := range 12000 { // 12,000 paths of about 100 bytes: over the 1 MiB a command line may hold
		path := fmt.Sprintf("%sfile-%05d-with-a-long-name.txt", folder, i)
		old[path], now[path] = "1\n", "2\n"
	}
	origin := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: old}, gitrepotest.Commit{Message: "change", Files: now})
	origin.Git("config", "uploadpack.allowFilter", "true")
	clone := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", "--filter=blob:none", "file://"+origin.Dir, clone).CombinedOutput(); err != nil {
		t.Fatalf("partial clone: %v: %s", err, out)
	}
	source, err := read(t, clone, "HEAD~1")
	if err != nil {
		t.Fatalf("Read(partial clone missing 12,000 old contents) = %v, want an answer", err)
	}
	if len(source.Missing) < 12000 {
		t.Errorf("Read(partial clone missing 12,000 old contents) = %d missing, want the 12,000 named", len(source.Missing))
	}
}
