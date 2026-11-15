//go:build integration

package gitrepotest

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationTheSameCommitsGiveTheSameNames(t *testing.T) {
	commits := []Commit{
		{Message: "first", Files: map[string]string{"a.ts": "one\n"}},
		{Message: "second", Files: map[string]string{"b.ts": "two\n"}, Remove: []string{"a.ts"}},
	}
	a, b := New(t, commits...), New(t, commits...)
	if first, second := a.Git("rev-parse", "HEAD"), b.Git("rev-parse", "HEAD"); first != second {
		t.Fatalf("New(same commits) heads = %s and %s, want equal", first, second)
	}
}

func TestIntegrationFingerprintSeesContentNotTime(t *testing.T) {
	repo := New(t, Commit{Message: "first", Files: map[string]string{"a.ts": "one\n"}})
	before := Fingerprint(t, repo.Dir)
	later := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(repo.Dir, "a.ts"), later, later); err != nil {
		t.Fatal(err)
	}
	if got := Fingerprint(t, repo.Dir); got != before {
		t.Errorf("Fingerprint after a time change = %s, want %s", got, before)
	}
	head, err := os.OpenFile(filepath.Join(repo.Dir, ".git", "HEAD"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := head.WriteString(" "); err != nil {
		t.Fatal(err)
	}
	if err := head.Close(); err != nil {
		t.Fatal(err)
	}
	changed := Fingerprint(t, repo.Dir)
	if changed == before {
		t.Errorf("Fingerprint after a changed byte in .git = %s, want a different one", changed)
	}
	if err := os.WriteFile(filepath.Join(repo.Dir, ".git", "new"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Fingerprint(t, repo.Dir); got == changed {
		t.Errorf("Fingerprint after a new empty file in .git = %s, want a different one", got)
	}
}
