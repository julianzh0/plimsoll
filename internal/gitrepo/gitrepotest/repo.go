// Package gitrepotest builds temporary git repositories with fixed names and fingerprints folders, for tests.
package gitrepotest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Commit is one commit to build: files written, then files removed, then everything committed.
type Commit struct {
	Message string
	Files   map[string]string // path -> content
	Remove  []string
}

// Repo is a temporary repository in a test's temporary folder.
type Repo struct {
	Dir     string
	t       testing.TB
	commits int
}

// New initializes a repository on branch main and makes each commit with a fixed author and date.
func New(t testing.TB, commits ...Commit) *Repo {
	t.Helper()
	repo := &Repo{Dir: t.TempDir(), t: t}
	repo.Git("init", "-q", "-b", "main")
	for _, commit := range commits {
		repo.Commit(commit)
	}
	return repo
}

// Commit writes and removes the commit's files, commits everything and returns the full commit name.
func (r *Repo) Commit(c Commit) string {
	r.t.Helper()
	for path, content := range c.Files {
		r.Write(path, content)
	}
	for _, path := range c.Remove {
		if err := os.RemoveAll(filepath.Join(r.Dir, path)); err != nil {
			r.t.Fatalf("removing %s: %v", path, err)
		}
	}
	r.commits++
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", c.Message)
	return r.Git("rev-parse", "HEAD")
}

// Write writes a file into the working tree only.
func (r *Repo) Write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.Dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		r.t.Fatalf("creating the folder of %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		r.t.Fatalf("writing %s: %v", path, err)
	}
}

// Git runs git in the repository with no user or system configuration and returns its trimmed output.
// A failure fails the test.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	output, err := r.run(args)
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// TryGit runs git like Git and returns its error, for a command that is meant to fail, such as a conflicting merge.
func (r *Repo) TryGit(args ...string) error {
	r.t.Helper()
	_, err := r.run(args)
	return err
}

func (r *Repo) run(args []string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", r.Dir}, args...)...)
	date := time.Date(2026, 1, 1, 0, 0, r.commits, 0, time.UTC).Format(time.RFC3339)
	command.Env = append(command.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Sample", "GIT_AUTHOR_EMAIL=sample@example.test", "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME=Sample", "GIT_COMMITTER_EMAIL=sample@example.test", "GIT_COMMITTER_DATE="+date,
	)
	return command.CombinedOutput()
}

// Fingerprint hashes every entry of a folder, .git included: its path, kind, size and content. Times are left out.
func Fingerprint(t testing.TB, dir string) string {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("opening %s: %v", dir, err)
	}
	defer func() { _ = root.Close() }()
	hash := sha256.New()
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content := ""
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			content, err = root.Readlink(path)
		case entry.Type().IsRegular():
			var data []byte
			data, err = fs.ReadFile(root.FS(), path)
			sum := sha256.Sum256(data)
			content = hex.EncodeToString(sum[:])
		}
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(hash, "%s\x00%v\x00%d\x00%s\n", path, entry.Type(), info.Size(), content)
		return err
	})
	if err != nil {
		t.Fatalf("fingerprinting %s: %v", dir, err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
