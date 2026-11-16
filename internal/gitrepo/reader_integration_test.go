//go:build integration

package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/julianzh0/plimsoll/internal/conventions"
	"github.com/julianzh0/plimsoll/internal/gitrepo/gitrepotest"
	"github.com/julianzh0/plimsoll/internal/snapshot"
)

func read(t *testing.T, repository, base string) (snapshot.Source, error) {
	t.Helper()
	return NewReader(os.Environ()).Read(context.Background(), repository, base, snapshot.WantsOf(conventions.Default()))
}

func paths(entries []snapshot.Entry) []string {
	var list []string
	for _, entry := range entries {
		list = append(list, entry.Path)
	}
	return list
}

func content(entries []snapshot.Entry, path string) string {
	for _, entry := range entries {
		if entry.Path == path {
			return string(entry.Content)
		}
	}
	return "<absent>"
}

func TestIntegrationReadSeesTheTrackedFilesOfBothSides(t *testing.T) {
	repo := gitrepotest.New(t)
	base := repo.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "one\ntwo\nthree\n", "b.ts": "x\n"}})
	head := repo.Commit(gitrepotest.Commit{Message: "second", Files: map[string]string{"a.ts": "one\ntwo\nthree\nfour\n", "c.ts": "y\n"}, Remove: []string{"b.ts"}})
	repo.Write("a.ts", "one\ntwo\nthree\nfour\nfive\n")
	repo.Write("d.ts", "untracked\n")

	src, err := read(t, repo.Dir, base)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if got := paths(src.Now); !reflect.DeepEqual(got, []string{"a.ts", "c.ts"}) {
		t.Errorf("Read.Now paths = %v, want [a.ts c.ts]", got)
	}
	if got := content(src.Now, "a.ts"); got != "one\ntwo\nthree\nfour\nfive\n" {
		t.Errorf("Read.Now a.ts = %q, want the working tree's content", got)
	}
	if got := paths(src.Before); !reflect.DeepEqual(got, []string{"a.ts", "b.ts"}) {
		t.Errorf("Read.Before paths = %v, want [a.ts b.ts]", got)
	}
	if got := content(src.Before, "a.ts"); got != "one\ntwo\nthree\n" {
		t.Errorf("Read.Before a.ts = %q, want the base's content", got)
	}
	want := []snapshot.Change{{Path: "a.ts", Added: 2}, {Path: "b.ts", Removed: 1}, {Path: "c.ts", Added: 1}}
	if !reflect.DeepEqual(src.Changes, want) {
		t.Errorf("Read.Changes = %+v, want %+v", src.Changes, want)
	}
	if !src.Modified || src.Untracked != 1 || src.Base != base || src.Head != head || src.AskedBase != base || src.MergeBase {
		t.Errorf("Read = modified %v, untracked %d, base %s, head %s, asked %s, merge base %v; want true, 1, %s, %s, %s, false",
			src.Modified, src.Untracked, src.Base, src.Head, src.AskedBase, src.MergeBase, base, head, base)
	}
	if got := repo.Git("rev-parse", "HEAD"); got != head {
		t.Errorf("HEAD after Read = %s, want %s", got, head)
	}

	if err := os.Remove(filepath.Join(repo.Dir, "c.ts")); err != nil {
		t.Fatal(err)
	}
	src, err = read(t, repo.Dir, base)
	if err != nil || content(src.Now, "c.ts") != "<absent>" {
		t.Errorf("Read after deleting c.ts = %v, %v; want no entry for c.ts", paths(src.Now), err)
	}
}

func TestIntegrationReadFollowsARename(t *testing.T) {
	repo := gitrepotest.New(t)
	base := repo.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{"old.ts": strings.Repeat("line\n", 40)}})
	repo.Git("mv", "old.ts", "new.ts")
	src, err := read(t, repo.Dir, base)
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if want := []snapshot.Change{{Path: "new.ts", From: "old.ts"}}; !reflect.DeepEqual(src.Changes, want) {
		t.Errorf("Read.Changes = %+v, want %+v", src.Changes, want)
	}
	if paths(src.Before)[0] != "old.ts" || paths(src.Now)[0] != "new.ts" || content(src.Before, "old.ts") == "" {
		t.Errorf("Read = before %v, now %v; want old.ts with its content before and new.ts now", paths(src.Before), paths(src.Now))
	}
}

func TestIntegrationReadLeavesUnwantedAndBinaryContentUnread(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"README.md": "# x\n", "logo.bin": "BIN\x00data", "empty.ts": "",
		"tsconfig.json": "{}\n", "web/tsconfig.json": "{}\n",
		"pyproject.toml": "[project]\n", "web/pyproject.toml": "[project]\n", "pytest.ini": "[pytest]\n"}})
	src, err := read(t, repo.Dir, "HEAD")
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	for _, entry := range src.Now {
		switch entry.Path {
		case "README.md":
			if entry.Content != nil {
				t.Errorf("Read.Now README.md has content %q, want none", entry.Content)
			}
		case "logo.bin":
			if !entry.Binary || entry.Content != nil {
				t.Errorf("Read.Now logo.bin = binary %v, content %q; want binary and no content", entry.Binary, entry.Content)
			}
		case "empty.ts":
			if entry.Content == nil {
				t.Errorf("Read.Now empty.ts has nil content, want an empty read")
			}
		case "tsconfig.json":
			if string(entry.Content) != "{}\n" {
				t.Errorf("Read.Now tsconfig.json has content %q, want the root config read", entry.Content)
			}
		case "pyproject.toml":
			if string(entry.Content) != "[project]\n" {
				t.Errorf("Read.Now pyproject.toml has content %q, want the root manifest read", entry.Content)
			}
		case "web/pyproject.toml":
			if entry.Content != nil {
				t.Errorf("Read.Now web/pyproject.toml has content %q, want none: only the root manifest is read", entry.Content)
			}
		case "pytest.ini":
			if entry.Content != nil {
				t.Errorf("Read.Now pytest.ini has content %q, want none: a file that shadows a declaration is never read", entry.Content)
			}
		case "web/tsconfig.json":
			if entry.Content != nil {
				t.Errorf("Read.Now web/tsconfig.json has content %q, want none: only the root config is read", entry.Content)
			}
		}
	}
}

func TestIntegrationReadCountsAnUnmergedFileOnce(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "one\n"}})
	repo.Git("switch", "-q", "-c", "side")
	repo.Commit(gitrepotest.Commit{Message: "side", Files: map[string]string{"a.ts": "side\n"}})
	repo.Git("switch", "-q", "main")
	repo.Commit(gitrepotest.Commit{Message: "main", Files: map[string]string{"a.ts": "main\n"}})
	if err := repo.TryGit("merge", "-q", "side"); err == nil {
		t.Fatal("the merge succeeded, want a conflict")
	}
	src, err := read(t, repo.Dir, "HEAD")
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	if got := paths(src.Now); !reflect.DeepEqual(got, []string{"a.ts"}) || !src.Modified {
		t.Errorf("Read mid-merge = now %v, modified %v; want [a.ts], true", got, src.Modified)
	}
}

func TestIntegrationReadAcceptsAFolderInsideTheRepository(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"src/a.ts": "a\n"}})
	want, err := read(t, repo.Dir, "HEAD")
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(repo.Dir, link); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{repo.Dir + "/src", repo.Dir + "/", link} {
		got, err := read(t, folder, "HEAD")
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Read(%s) = %+v, %v; want the source of the top folder", folder, got, err)
		}
	}
}

func TestIntegrationReadRefuses(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	empty := gitrepotest.New(t)
	orphan := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	orphan.Git("switch", "-q", "--orphan", "other")
	orphan.Commit(gitrepotest.Commit{Message: "other", Files: map[string]string{"b.ts": "b\n"}})
	before := gitrepotest.Fingerprint(t, repo.Dir)
	written := filepath.Join(t.TempDir(), "written")
	cases := []struct {
		name, repository, base string
		want                   snapshot.RefusalKind
	}{
		{"not a repository", t.TempDir(), "HEAD", snapshot.NotARepository},
		{"no commit", empty.Dir, "HEAD", snapshot.NoCommit},
		{"an unknown base", repo.Dir, "nope", snapshot.UnknownBase},
		{"a base that looks like an option", repo.Dir, "--output=" + written, snapshot.UnknownBase},
		{"a short option", repo.Dir, "-h", snapshot.UnknownBase},
		{"no common history", orphan.Dir, "main", snapshot.NoCommonHistory},
	}
	for _, tc := range cases {
		src, err := read(t, tc.repository, tc.base)
		refusal, ok := errors.AsType[*snapshot.Refusal](err)
		if !ok || refusal.Kind != tc.want || !reflect.DeepEqual(src, snapshot.Source{}) {
			t.Errorf("Read(%s) = %+v, %v; want refusal %d and no source", tc.name, src, err, tc.want)
		}
		if tc.want == snapshot.UnknownBase && refusal != nil && refusal.Detail != tc.base {
			t.Errorf("Read(%s) detail = %q, want %q", tc.name, refusal.Detail, tc.base)
		}
	}
	if got := gitrepotest.Fingerprint(t, repo.Dir); got != before {
		t.Errorf("the repository changed during the refused reads")
	}
	if _, err := os.Stat(written); err == nil {
		t.Errorf("a base that looks like an option wrote a file")
	}
}

func TestIntegrationReadUsesTheMergeBaseOfABranchThatMovedOn(t *testing.T) {
	repo := gitrepotest.New(t)
	first := repo.Commit(gitrepotest.Commit{Message: "A", Files: map[string]string{"a.ts": "a\n"}})
	repo.Git("switch", "-q", "-c", "feature")
	repo.Git("switch", "-q", "main")
	repo.Commit(gitrepotest.Commit{Message: "B", Files: map[string]string{"b.ts": "b\n"}})
	repo.Git("switch", "-q", "feature")
	repo.Commit(gitrepotest.Commit{Message: "C", Files: map[string]string{"c.ts": "c\n"}})
	src, err := read(t, repo.Dir, "main")
	if err != nil || src.Base != first || !src.MergeBase {
		t.Errorf("Read(base main) = base %s, merge base %v, %v; want %s, true", src.Base, src.MergeBase, err, first)
	}
	src, err = read(t, repo.Dir, first)
	if err != nil || src.Base != first || src.MergeBase {
		t.Errorf("Read(base A) = base %s, merge base %v, %v; want %s, false", src.Base, src.MergeBase, err, first)
	}
}
