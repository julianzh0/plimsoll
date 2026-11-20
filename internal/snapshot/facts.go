package snapshot

import (
	"bytes"
	"cmp"
	"encoding/json"
	"maps"
	"math"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// SettingsFile is the one settings template that is read, at the repository root.
const SettingsFile = ".env.example"

var settingLine = regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]+)=`)

// WantsOf says which paths the reader reads and when a file is binary, from the rules.
func WantsOf(rules conventions.Rules) Wants {
	return Wants{
		Path: func(path string) bool {
			if path == SettingsFile || path == rules.OwnFile {
				return true
			}
			// a root licence file names the repository's own copyright holders: a file added whole that another
			// holder signed came from another project
			if !strings.Contains(path, "/") && rules.FunctionRules.RootLicence.MatchString(path) {
				return true
			}
			if _, known := manifestOf(rules, path); known || isLockfile(rules, path) {
				return true
			}
			if onPaths(rules, path, rules.ComposeFiles) {
				return true
			}
			if slices.ContainsFunc(rules.Languages, func(l conventions.Language) bool {
				prefix := l.Imports.Prefix
				member := prefix.Members && prefix.File != "" && strings.HasSuffix(path, "/"+prefix.File) && !inFolders(path, rules.SkippedFolders)
				return prefix.File == path || member || slices.ContainsFunc(l.Layout, func(d conventions.Declaration) bool { return d.File == path })
			}) {
				return true
			}
			return !excluded(rules, path)
		},
		// NOTE: documentation and configuration files are read with every question, also when no added name needs
		// them (6 to 9 ms on the largest field repository), and a large tracked data file of such a suffix is read
		// whole. Upgrade: read them in a second step, only when an added name has no use in the code files.
		Words:      func(path string) bool { return wordsFile(rules, path) },
		Binary:     conventions.IsBinary,
		SniffBytes: conventions.BinarySniffBytes,
		BaseFile:   rules.OwnFile,
		Base:       func(content []byte) (string, string) { return declaredBaseIn(rules, content) },
	}
}

// declaredBaseIn reads the default base from the own file's content: its name, or the problem that makes it
// unusable; both are "" when the file declares none.
func declaredBaseIn(rules conventions.Rules, content []byte) (name, problem string) {
	o := readOwnFile(rules, map[string][]byte{rules.OwnFile: bytes.TrimPrefix(content, []byte(byteOrderMark))}, nil)
	at := slices.IndexFunc(rules.OwnKeys, func(k conventions.OwnKey) bool { return k.Holds == conventions.BaseName })
	if at < 0 {
		return "", ""
	}
	key := rules.OwnKeys[at].Key
	switch {
	case o.unreadable:
		return "", o.problems[0]
	case o.single[key] != "":
		return "", o.single[key]
	case len(o.values[key]) > 0:
		return o.values[key][0], ""
	}
	return "", ""
}

// settings reads the names of the settings template; it keeps no value.
func settings(entries []Entry) *Settings {
	for _, entry := range entries {
		if entry.Path != SettingsFile || entry.Content == nil {
			continue
		}
		names, active := []string{}, []string{}
		for _, match := range settingLine.FindAllSubmatchIndex(entry.Content, -1) {
			name := string(entry.Content[match[2]:match[3]])
			names = append(names, name)
			if entry.Content[match[0]] != '#' {
				active = append(active, name)
			}
		}
		return &Settings{Path: entry.Path, Names: names, Active: active}
	}
	return nil
}

// manifests reads the runtime dependencies of the known manifests: at the root, and a nested format's wherever it is
// tracked. A manifest that cannot be read as its format is returned as unreadable.
// NOTE: only a nested format's manifests are read below the repository root; a monorepo's other nested manifests are
// not seen.
// Upgrade: read every tracked manifest.
func manifests(rules conventions.Rules, entries []Entry) ([]Manifest, []string) {
	var found []Manifest
	var unreadable []string
	content := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		content[entry.Path] = entry.Content
	}
	type lockRead struct {
		names map[string]bool
		ok    bool
	}
	read := map[string]lockRead{} // each lockfile is read once, however many manifests share it
	for _, entry := range entries {
		known, isManifest := manifestOf(rules, entry.Path)
		if !isManifest || entry.Content == nil {
			continue
		}
		names, manifest, readable := dependencies(known.Format, entry.Content)
		switch {
		case !manifest:
			continue
		case !readable:
			unreadable = append(unreadable, entry.Path)
			continue
		}
		slices.Sort(names)
		m := Manifest{Path: entry.Path, Dependencies: names}
		var locks [][]byte
		m.Lockfiles, locks = nearestLockfiles(content, path.Dir(entry.Path), known.Lockfiles)
		holds := make([]func(string) bool, len(m.Lockfiles))
		for i, lockfile := range m.Lockfiles {
			r, seen := read[lockfile]
			if !seen {
				r.names, r.ok = lockNames(lockfile, locks[i])
				read[lockfile] = r
			}
			if !r.ok {
				m.ByText = append(m.ByText, lockfile)
				holds[i] = func(name string) bool { return holdsName(locks[i], name) }
				continue
			}
			holds[i] = func(name string) bool { return lockHolds(r.names, lockfile, name) }
		}
		if len(m.Lockfiles) > 0 {
			m.Unlocked = []string{}
			for _, name := range names {
				if !slices.ContainsFunc(holds, func(held func(string) bool) bool { return held(name) }) {
					m.Unlocked = append(m.Unlocked, name)
				}
			}
		}
		found = append(found, m)
	}
	slices.SortFunc(found, func(a, b Manifest) int { return strings.Compare(a.Path, b.Path) })
	return found, unreadable
}

// nearestLockfiles are the lockfiles of the given names in the nearest folder, at or above a manifest's, that holds
// one: a workspace's members share the lockfile at its root. Only lockfiles that were read count.
func nearestLockfiles(content map[string][]byte, folder string, names []string) (paths []string, locks [][]byte) {
	for {
		for _, name := range names {
			if lock := content[path.Join(folder, name)]; lock != nil {
				paths, locks = append(paths, path.Join(folder, name)), append(locks, lock)
			}
		}
		if len(paths) > 0 || folder == "." {
			return paths, locks
		}
		folder = path.Dir(folder)
	}
}

// dependencies reads a manifest's runtime dependencies. manifest is false when the file declares no project at all;
// readable is false when it does but its dependencies cannot be read.
func dependencies(format conventions.ManifestFormat, content []byte) (names []string, manifest, readable bool) {
	names = []string{}
	switch format {
	case conventions.PyprojectDependencies:
		return pyprojectDependencies(content)
	case conventions.PubspecDependencies:
		return pubspecDependencies(content)
	case conventions.PomDependencies:
		return pomDependencies(content)
	case conventions.GradleDependencies:
		return gradleDependencies(content)
	case conventions.CsprojDependencies:
		return csprojDependencies(content)
	case conventions.SwiftDependencies:
		return swiftDependencies(content)
	case conventions.VcpkgDependencies:
		return vcpkgDependencies(content)
	case conventions.CargoDependencies:
		return cargoDependencies(content)
	case conventions.JSONDependencies:
		var parsed struct {
			Dependencies map[string]json.RawMessage `json:"dependencies"`
		}
		if err := json.Unmarshal(content, &parsed); err != nil {
			return nil, true, false
		}
		names = slices.AppendSeq(names, maps.Keys(parsed.Dependencies))
	case conventions.GoModRequire:
		inBlock := false
		for line := range strings.Lines(string(content)) {
			fields := strings.Fields(line)
			switch {
			case len(fields) == 0 || strings.HasPrefix(fields[0], "//"):
			case inBlock && fields[0] == ")":
				inBlock = false
			case !inBlock && len(fields) >= 2 && fields[0] == "require" && fields[1] == "(":
				inBlock = true
			case !inBlock && fields[0] == "require" && len(fields) >= 3 && !strings.Contains(line, "// indirect"):
				names = append(names, fields[1])
			case inBlock && len(fields) >= 2 && !strings.Contains(line, "// indirect"):
				names = append(names, fields[0])
			}
		}
	}
	return names, true, true
}

// requirementName is the leading name of a requirement string, normalised as Python packaging compares names: lower
// case, every run of ".", "_" and "-" one "-".
func requirementName(requirement string) string {
	var name strings.Builder
	separator := false
	for _, r := range strings.TrimSpace(requirement) {
		switch {
		case r == '.' || r == '_' || r == '-':
			separator = name.Len() > 0
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			if separator {
				name.WriteByte('-')
				separator = false
			}
			name.WriteRune(unicode.ToLower(r))
		default:
			return name.String()
		}
	}
	return name.String()
}

// rolesOf maps each path to its role; a later list wins over an earlier one.
func rolesOf(lists ...[]File) map[string]Role {
	roles := map[string]Role{}
	for _, files := range lists {
		for _, file := range files {
			roles[file.Path] = file.Role
		}
	}
	return roles
}

// footprint sums the changes since the base by area: the role of the file now, else at the base; documentation
// by path; anything else counts only as a changed file. In a product file whose test code grew, as many of the added
// lines as it grew by are test lines; in one whose test code shrank, as many of the removed lines.
// NOTE: a changed line of test code counts as a product line added and removed. Upgrade: read the changed line
// ranges from git and count them against the blocks of both sides.
func footprint(rules conventions.Rules, changes []Change, since int, now, before Tree) Footprint {
	result := Footprint{Commits: since, Files: len(changes)}
	roles := rolesOf(before.Files, now.Files)
	testNow, testBefore := testLinesOf(now.Files), testLinesOf(before.Files)
	for _, change := range changes {
		fileRole, found := roles[change.Path]
		if !found {
			fileRole, found = roles[change.From]
		}
		var lines *Lines
		switch {
		case found && fileRole == Product:
			lines = &result.Product
			basePath := cmp.Or(change.From, change.Path)
			added, removed := change.Added, change.Removed
			if testNow[change.Path] > 0 || testBefore[basePath] > 0 {
				result.HoldsTestCode++
			}
			if grew := testNow[change.Path] - testBefore[basePath]; grew > 0 {
				moved := min(grew, added)
				result.Test.Added, added = result.Test.Added+moved, added-moved
			} else if grew < 0 {
				moved := min(-grew, removed)
				result.Test.Removed, removed = result.Test.Removed+moved, removed-moved
			}
			change.Added, change.Removed = added, removed
		case found && fileRole.IsTest():
			lines = &result.Test
		case area(rules, change.Path) == AreaDocs:
			lines = &result.Docs
		default:
			continue
		}
		lines.Added += change.Added
		lines.Removed += change.Removed
	}
	if result.Product.Added > 0 {
		ratio := math.Round(10*float64(result.Test.Added)/float64(result.Product.Added)) / 10
		result.TestPerProduct = &ratio
	}
	return result
}

// testLinesOf maps each file that holds test code inside its product code to its number of such lines.
func testLinesOf(files []File) map[string]int {
	found := map[string]int{}
	for _, file := range files {
		if file.TestLines > 0 {
			found[file.Path] = file.TestLines
		}
	}
	return found
}

// history flags each commit as since the base and as usable, and gives each path its area.
func history(rules conventions.Rules, commits []Commit, since []string) History {
	result := History{Commits: make([]Commit, 0, len(commits)), Read: len(commits)}
	inSince := make(map[string]bool, len(since))
	for _, id := range since {
		inSince[id] = true
	}
	for _, commit := range commits {
		commit.Since = inSince[commit.ID]
		commit.Usable = len(commit.Files) >= 1 && len(commit.Files) <= rules.CommitPaths
		files := make([]FileChange, len(commit.Files))
		for i, file := range commit.Files {
			file.Area = area(rules, file.Path)
			files[i] = file
		}
		commit.Files = files
		if commit.Usable {
			result.Usable++
		}
		result.Commits = append(result.Commits, commit)
	}
	return result
}

var (
	composeKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// composeFiles reads the environment keys of every compose file, line by line. Only keys are kept.
func composeFiles(rules conventions.Rules, entries []Entry) []ComposeFile {
	var found []ComposeFile
	for _, entry := range entries {
		if entry.Content == nil || !onPaths(rules, entry.Path, rules.ComposeFiles) {
			continue
		}
		keys, unread := composeKeys(string(entry.Content))
		found = append(found, ComposeFile{Path: entry.Path, Keys: keys, Unread: unread})
	}
	slices.SortFunc(found, func(a, b ComposeFile) int { return strings.Compare(a.Path, b.Path) })
	return found
}

// composeKeys reads the keys of `environment:` blocks in both forms, `- KEY=value` and `KEY: value`, and counts
// the environment lines it cannot read: anchors and merges, flow style, env_file. A key counts only when its
// value comes from the host: no value, which passes the host's on, or one that interpolates with `$`. A fixed
// value is configuration of the container that nobody supplies, so it is no setting.
func composeKeys(content string) ([]string, int) {
	keys := map[string]bool{}
	unread, inBlock, blockIndent, entryIndent := 0, false, 0, -1
	for raw := range strings.Lines(content) {
		line := strings.TrimRight(raw, "\r\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if inBlock && indent > blockIndent {
			if entryIndent < 0 {
				entryIndent = indent
			}
			if indent > entryIndent {
				continue // a value that goes on over several lines
			}
			if key, fromHost, ok := environmentKey(trimmed); !ok {
				unread++
			} else if fromHost {
				keys[key] = true
			}
			continue
		}
		inBlock = false
		switch {
		case trimmed == "environment:":
			inBlock, blockIndent, entryIndent = true, indent, -1
		case strings.HasPrefix(trimmed, "environment:"), strings.HasPrefix(trimmed, "env_file:"):
			unread++
		}
	}
	return sortedKeys(keys), unread
}

// environmentKey reads one entry of an environment block: its key, whether its value comes from the host, and
// whether the entry could be read at all.
func environmentKey(entry string) (key string, fromHost, ok bool) {
	var value string
	if item, isList := strings.CutPrefix(entry, "- "); isList {
		item = strings.Trim(strings.TrimSpace(item), `"'`)
		key, value, _ = strings.Cut(item, "=")
	} else {
		var found bool
		if key, value, found = strings.Cut(entry, ":"); !found {
			return "", false, false
		}
	}
	value = strings.TrimSpace(value)
	return key, value == "" || strings.Contains(value, "$"), composeKey.MatchString(key)
}

// holdsName says whether a lockfile's text holds a dependency name as a whole name: not as part of a longer name
// (letters, digits, "_", "." and "-" around it).
// NOTE: the name is matched by text, not by the lockfile's format; a name that only appears inside another
// package's metadata counts as held. Upgrade: read each lockfile format.
func holdsName(lock []byte, name string) bool {
	part := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '.' || b == '-'
	}
	for start := 0; ; {
		i := bytes.Index(lock[start:], []byte(name))
		if i < 0 {
			return false
		}
		i += start
		end := i + len(name)
		if (i == 0 || !part(lock[i-1])) && (end == len(lock) || !part(lock[end])) {
			return true
		}
		start = i + 1
	}
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}
