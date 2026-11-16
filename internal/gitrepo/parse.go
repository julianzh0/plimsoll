package gitrepo

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/julianzh0/plimsoll/internal/snapshot"
)

// Modes of a tracked entry: regular files are measured, a submodule is named, anything else is left out.
const (
	modeFile       = "100644"
	modeExecutable = "100755"
	modeSubmodule  = "160000"
)

// regular says whether a mode is a regular file, the only kind of entry that is measured.
func regular(mode string) bool {
	return mode == modeFile || mode == modeExecutable
}

// fields splits NUL-terminated output.
func fields(output []byte) []string {
	trimmed := strings.TrimSuffix(string(output), "\x00")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\x00")
}

// trackedEntry is one line of ls-files -s or ls-tree -r: its mode, object name and path.
type trackedEntry struct {
	mode, object, path string
}

// parseTracked reads `ls-files -s -z` (mode object stage TAB path) or `ls-tree -r -z` (mode type object TAB path).
// A path listed once per stage of an unfinished merge is kept once.
func parseTracked(output []byte, objectField int) []trackedEntry {
	var entries []trackedEntry
	seen := map[string]bool{}
	for _, record := range fields(output) {
		meta, path, found := strings.Cut(record, "\t")
		parts := strings.Fields(meta)
		if !found || len(parts) <= objectField || seen[path] {
			continue
		}
		seen[path] = true
		entries = append(entries, trackedEntry{mode: parts[0], object: parts[objectField], path: path})
	}
	return entries
}

// parseStatus reads `status --porcelain=v2 -z`: whether a tracked file differs from the head, and how many
// files are untracked.
func parseStatus(output []byte) (modified bool, untracked int) {
	records := fields(output)
	for i := 0; i < len(records); i++ {
		switch {
		case strings.HasPrefix(records[i], "1 "), strings.HasPrefix(records[i], "u "):
			modified = true
		case strings.HasPrefix(records[i], "2 "):
			modified = true
			i++ // a rename is followed by its old path
		case strings.HasPrefix(records[i], "? "):
			untracked++
		}
	}
	return modified, untracked
}

// parseNumstat reads `--numstat -z` records from position i: `added TAB removed TAB path`, or for a rename
// `added TAB removed TAB` followed by the old and the new path. A binary file's `-` counts as 0.
// It returns the change and the position after it.
func parseNumstat(records []string, i int) (snapshot.Change, int, error) {
	parts := strings.SplitN(strings.TrimPrefix(records[i], "\n"), "\t", 3)
	if len(parts) != 3 {
		return snapshot.Change{}, 0, fmt.Errorf("unexpected numstat record %q", records[i])
	}
	change := snapshot.Change{Added: count(parts[0]), Removed: count(parts[1]), Path: parts[2]}
	if change.Path != "" {
		return change, i + 1, nil
	}
	if i+2 >= len(records) {
		return snapshot.Change{}, 0, fmt.Errorf("a rename without its paths at %q", records[i])
	}
	change.From, change.Path = records[i+1], records[i+2]
	return change, i + 3, nil
}

func count(field string) int {
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0 // "-" for a binary file
	}
	return n
}

// parseBatch reads `cat-file --batch` output: per object a header `name type size`, the content, a newline.
func parseBatch(output []byte) (map[string][]byte, error) {
	contents := map[string][]byte{}
	for len(output) > 0 {
		header, rest, found := bytes.Cut(output, []byte{'\n'})
		parts := strings.Fields(string(header))
		if found && len(parts) == 2 && parts[1] == "missing" { // a partial clone lacks it: no content
			output = rest
			continue
		}
		if !found || len(parts) != 3 {
			return nil, fmt.Errorf("unexpected cat-file header %q", header)
		}
		size, err := strconv.Atoi(parts[2])
		if err != nil || size > len(rest) {
			return nil, fmt.Errorf("unexpected cat-file size in %q", header)
		}
		contents[parts[0]] = rest[:size:size]
		output = bytes.TrimPrefix(rest[size:], []byte{'\n'})
	}
	return contents, nil
}

// pathsOfObjects are the paths, sorted, at which `log --raw --no-abbrev -z --format=%x01%H` shows one of the objects
// on either side of a change. A record is ":<modes> <old> <new> <status>" and then its path; the first record of a
// commit follows its header after a newline.
func pathsOfObjects(raw []byte, objects map[string]bool) []string {
	records := fields(raw)
	var found []string
	for i := 0; i < len(records); i++ {
		record := records[i][strings.LastIndexByte(records[i], '\n')+1:]
		parts := strings.Fields(record)
		if !strings.HasPrefix(record, ":") || len(parts) < 4 || i+1 >= len(records) {
			continue
		}
		if objects[parts[2]] || objects[parts[3]] {
			found = append(found, records[i+1])
		}
		i++
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// parseLog reads `log --numstat -z --format=%x01%H`: a field that starts with 0x01 opens a commit and holds its
// name; its numstat records follow, the first one after a newline. Paths are never split on newlines.
func parseLog(output []byte) ([]snapshot.Commit, error) {
	records := fields(output)
	var commits []snapshot.Commit
	for i := 0; i < len(records); {
		if !strings.HasPrefix(records[i], "\x01") {
			if len(commits) == 0 {
				return nil, fmt.Errorf("unexpected log record %q", records[i])
			}
			change, next, err := parseNumstat(records, i)
			if err != nil {
				return nil, err
			}
			last := &commits[len(commits)-1]
			last.Files = append(last.Files, snapshot.FileChange{Path: change.Path, Added: change.Added, Removed: change.Removed})
			i = next
			continue
		}
		commits = append(commits, snapshot.Commit{ID: strings.TrimPrefix(records[i], "\x01")})
		i++
	}
	return commits, nil
}
