// Package snapshottest builds a snapshot.Source from sample folders or from maps, for tests.
package snapshottest

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/juzhxng/plimsoll/internal/snapshot"
)

// Load reads dir/now and, when it exists, dir/base; without a base folder the base equals now.
func Load(t testing.TB, dir string) snapshot.Source {
	t.Helper()
	now := readTree(t, dir+"/now")
	base := now
	if _, err := os.Stat(dir + "/base"); err == nil {
		base = readTree(t, dir+"/base")
	}
	return Files(base, now)
}

// Files builds a source from two maps of path to content; a nil base equals an empty one.
// Every path whose content differs gets a Change with all lines of now added and all lines of the base removed.
func Files(base, now map[string]string) snapshot.Source {
	source := snapshot.Source{Before: entries(base), Now: entries(now)}
	paths := make([]string, 0, len(base)+len(now))
	for p := range base {
		paths = append(paths, p)
	}
	for p := range now {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range slices.Compact(paths) {
		before, inBase := base[p]
		after, inNow := now[p]
		if inBase && inNow && before == after {
			continue
		}
		source.Changes = append(source.Changes, snapshot.Change{Path: p, Added: lineCount(after), Removed: lineCount(before)})
	}
	return source
}

// Commit is a commit that adds one line to each path.
func Commit(id string, paths ...string) snapshot.Commit {
	commit := snapshot.Commit{ID: id}
	for _, p := range paths {
		commit.Files = append(commit.Files, snapshot.FileChange{Path: p, Added: 1})
	}
	return commit
}

// Lines returns n lines of text.
func Lines(n int) string {
	return strings.Repeat("line\n", n)
}

func entries(files map[string]string) []snapshot.Entry {
	list := make([]snapshot.Entry, 0, len(files))
	for p, content := range files {
		list = append(list, snapshot.Entry{Path: p, Content: []byte(content)})
	}
	slices.SortFunc(list, func(a, b snapshot.Entry) int { return strings.Compare(a.Path, b.Path) })
	return list
}

func lineCount(content string) int {
	count := strings.Count(content, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func readTree(t testing.TB, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := os.DirFS(dir)
	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(root, name)
		files[name] = string(content)
		return err
	})
	if err != nil {
		t.Fatalf("reading sample %s: %v", dir, err)
	}
	return files
}
