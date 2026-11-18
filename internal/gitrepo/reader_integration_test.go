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

func TestIntegrationReadLeavesOutTheFilesOfADeletedFolder(t *testing.T) {
	repo := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{
		"a.ts": "x\n", "lib/b.ts": "x\n", "lib/notes.md": "# notes\n", "lib/deep/c.ts": "x\n"}})
	if err := os.RemoveAll(filepath.Join(repo.Dir, "lib")); err != nil {
		t.Fatal(err)
	}
	src, err := read(t, repo.Dir, "HEAD")
	if err != nil || !reflect.DeepEqual(paths(src.Now), []string{"a.ts"}) || len(src.Unreadable) != 0 {
		t.Errorf("Read after deleting lib/ = now %v, unreadable %v, %v; want only a.ts and nothing unreadable", paths(src.Now), src.Unreadable, err)
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
		"tsconfig.json": "{}\n", "web/tsconfig.json": "{}\n", "logo.svg": "<svg/>\n", ".npmrc": "x=1\n",
		"pyproject.toml": "[project]\n", "web/pyproject.toml": "[project]\n", "pytest.ini": "[pytest]\n",
		"LICENSE": "Copyright x\n", "web/LICENSE": "Copyright y\n"}})
	src, err := read(t, repo.Dir, "HEAD")
	if err != nil {
		t.Fatalf("Read = %v", err)
	}
	// Documentation and configuration are read for their words alone, to see whether they name code a change added.
	read := map[string]string{"README.md": "# x\n", "tsconfig.json": "{}\n", "web/tsconfig.json": "{}\n", "pyproject.toml": "[project]\n",
		"web/pyproject.toml": "[project]\n", "pytest.ini": "[pytest]\n", "empty.ts": "", "LICENSE": "Copyright x\n"}
	for _, entry := range src.Now {
		want, wanted := read[entry.Path]
		switch {
		case entry.Path == "logo.bin":
			if !entry.Binary || entry.Content != nil {
				t.Errorf("Read.Now logo.bin = binary %v, content %q; want binary and no content", entry.Binary, entry.Content)
			}
		case wanted && (entry.Content == nil || string(entry.Content) != want):
			t.Errorf("Read.Now %s has content %q, want %q", entry.Path, entry.Content, want)
		case !wanted && entry.Content != nil:
			t.Errorf("Read.Now %s has content %q, want none", entry.Path, entry.Content)
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

	declaring := func(content string) string {
		r := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n", ".plimsoll.toml": content}})
		return r.Dir
	}
	untracked := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	untracked.Write(".plimsoll.toml", "[defaults]\nbase = \"main\"\n")
	hashed := gitrepotest.New(t)
	hash := hashed.Commit(gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	hashed.Commit(gitrepotest.Commit{Message: "declare", Files: map[string]string{".plimsoll.toml": "[defaults]\nbase = \"" + hash + "\"\n"}})
	lonely := gitrepotest.New(t, gitrepotest.Commit{Message: "base", Files: map[string]string{"a.ts": "a\n"}})
	lonely.Git("switch", "-q", "--orphan", "other")
	lonely.Commit(gitrepotest.Commit{Message: "other", Files: map[string]string{"b.ts": "b\n", ".plimsoll.toml": "[defaults]\nbase = \"main\"\n"}})
	notName := "`base` is not the name of a branch, a remote branch or a tag"
	withoutBase := []struct {
		name, repository string
		want             snapshot.RefusalKind
		detail, file     string
	}{
		{"no file", repo.Dir, snapshot.NoBase, "", ".plimsoll.toml"},
		{"limits only", declaring("[limits]\nfile_lines = 1500\n"), snapshot.NoBase, "", ".plimsoll.toml"},
		{"an untracked file", untracked.Dir, snapshot.NoBase, "", ".plimsoll.toml"},
		{"a base that is no text", declaring("[defaults]\nbase = 3\n"), snapshot.NoBase, "`base` is not text", ".plimsoll.toml"},
		{"a base without its form", declaring("[defaults]\nbase = \"HEAD~1\"\n"), snapshot.NoBase, notName, ".plimsoll.toml"},
		{"HEAD", declaring("[defaults]\nbase = \"HEAD\"\n"), snapshot.UnknownDeclaredBase, "HEAD", ".plimsoll.toml"},
		{"a commit hash", hashed.Dir, snapshot.UnknownDeclaredBase, hash, ".plimsoll.toml"},
		{"no such branch", declaring("[defaults]\nbase = \"nope\"\n"), snapshot.UnknownDeclaredBase, "nope", ".plimsoll.toml"},
		{"no common history", lonely.Dir, snapshot.NoCommonHistory, "main", ""},
	}
	for _, tc := range withoutBase {
		src, err := read(t, tc.repository, "")
		refusal, ok := errors.AsType[*snapshot.Refusal](err)
		if !ok || refusal.Kind != tc.want || refusal.Detail != tc.detail || refusal.File != tc.file || !reflect.DeepEqual(src, snapshot.Source{}) {
			t.Errorf("Read(%s, no base) = %+v, %v; want refusal %d, detail %q, file %q", tc.name, src, err, tc.want, tc.detail, tc.file)
		}
	}
	if _, err := os.Stat(written); err == nil {
		t.Errorf("a base that looks like an option wrote a file")
	}
}

func TestIntegrationReadWithoutABaseUsesTheDeclaredOne(t *testing.T) {
	declare := func(base string) string { return "[defaults]\nbase = \"" + base + "\"\n" }
	// setup gives a repository on branch feature, cut from main, whose main commit tracks the file declaring main.
	setup := func() (*gitrepotest.Repo, string, string) {
		repo := gitrepotest.New(t)
		mainCommit := repo.Commit(gitrepotest.Commit{Message: "main", Files: map[string]string{"a.ts": "a\n", ".plimsoll.toml": declare("main")}})
		repo.Git("switch", "-q", "-c", "feature")
		feature := repo.Commit(gitrepotest.Commit{Message: "feature", Files: map[string]string{"c.ts": "c\n"}})
		return repo, mainCommit, feature
	}
	cases := []struct {
		name    string
		prepare func(repo *gitrepotest.Repo, mainCommit, feature string)
		asked   string
	}{
		{"a branch", func(*gitrepotest.Repo, string, string) {}, "main"},
		{"a remote branch", func(repo *gitrepotest.Repo, mainCommit, _ string) {
			repo.Git("update-ref", "refs/remotes/origin/main", mainCommit)
			repo.Write(".plimsoll.toml", declare("origin/main"))
		}, "origin/main"},
		{"a tag", func(repo *gitrepotest.Repo, mainCommit, _ string) {
			repo.Git("tag", "v1", mainCommit)
			repo.Write(".plimsoll.toml", declare("v1"))
		}, "v1"},
		{"a branch before a tag of its name", func(repo *gitrepotest.Repo, mainCommit, feature string) {
			repo.Git("tag", "x", feature)
			repo.Git("branch", "x", mainCommit)
			repo.Write(".plimsoll.toml", declare("x"))
		}, "x"},
		{"an edit not yet committed", func(repo *gitrepotest.Repo, mainCommit, _ string) {
			repo.Git("tag", "v1", mainCommit)
			repo.Commit(gitrepotest.Commit{Message: "nope", Files: map[string]string{".plimsoll.toml": declare("nope")}})
			repo.Write(".plimsoll.toml", declare("v1"))
		}, "v1"},
	}
	for _, tc := range cases {
		repo, mainCommit, feature := setup()
		tc.prepare(repo, mainCommit, feature)
		src, err := read(t, repo.Dir, "")
		if err != nil || src.AskedBase != tc.asked || !src.BaseDeclared || src.Base != mainCommit || src.MergeBase {
			t.Errorf("%s: Read without a base = asked %q, declared %v, base %s, merge base %v, %v; want %q, true, %s, false",
				tc.name, src.AskedBase, src.BaseDeclared, src.Base, src.MergeBase, err, tc.asked, mainCommit)
		}
	}
	repo, _, _ := setup()
	if src, err := read(t, repo.Dir, "HEAD"); err != nil || src.BaseDeclared || src.AskedBase != "HEAD" {
		t.Errorf("Read(HEAD) = asked %q, declared %v, %v; want HEAD as asked, not declared", src.AskedBase, src.BaseDeclared, err)
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
