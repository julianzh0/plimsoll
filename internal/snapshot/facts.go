package snapshot

import (
	"bytes"
	"cmp"
	"encoding/json"
	"maps"
	"math"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/juzhxng/plimsoll/internal/conventions"
)

// SettingsFile is the one settings template that is read, at the repository root.
const SettingsFile = ".env.example"

var settingLine = regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]+)=`)

// WantsOf says which paths the reader reads and when a file is binary, from the rules.
func WantsOf(rules conventions.Rules) Wants {
	return Wants{
		Path: func(path string) bool {
			if path == SettingsFile {
				return true
			}
			if _, known := manifestOf(rules, path); known || slices.ContainsFunc(rules.Manifests, func(m conventions.Manifest) bool { return slices.Contains(m.Lockfiles, path) }) {
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
		Binary:     conventions.IsBinary,
		SniffBytes: conventions.BinarySniffBytes,
	}
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
	var locks [][]byte
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
		for _, lockfile := range known.Lockfiles {
			if i := slices.IndexFunc(entries, func(e Entry) bool { return e.Path == lockfile && e.Content != nil }); i >= 0 {
				m.Lockfiles = append(m.Lockfiles, lockfile)
				locks = append(locks, entries[i].Content)
			}
		}
		if len(m.Lockfiles) > 0 {
			m.Unlocked = []string{}
			for _, name := range names {
				if !slices.ContainsFunc(locks, func(lock []byte) bool { return holdsName(lock, name) }) {
					m.Unlocked = append(m.Unlocked, name)
				}
			}
		}
		locks = locks[:0]
		found = append(found, m)
	}
	slices.SortFunc(found, func(a, b Manifest) int { return strings.Compare(a.Path, b.Path) })
	return found, unreadable
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

// pyprojectKey is a key of the [project] table that the dependency reader needs, at the start of a line.
var pyprojectKey = regexp.MustCompile(`(?m)^[ \t]*(dependencies|dynamic)[ \t]*=[ \t]*`)

// pyprojectDependencies reads the names of [project] dependencies by text. manifest is false when the file has
// no [project] table; readable is false when the dependencies cannot be read as an array of strings.
// NOTE: a quoted key and a dotted key outside the table are not read, and a line inside a multi-line string that
// starts like one of the two keys is taken for it. Upgrade: a TOML reader.
func pyprojectDependencies(content []byte) (names []string, manifest, readable bool) {
	table, found := tomlTable(strings.TrimPrefix(string(content), byteOrderMark), "project")
	if !found {
		return nil, false, true
	}
	names = []string{}
	for _, m := range pyprojectKey.FindAllStringSubmatchIndex(table, -1) {
		key, value := table[m[2]:m[3]], table[m[1]:]
		if !strings.HasPrefix(value, "[") {
			if key == "dependencies" {
				return nil, true, false
			}
			continue
		}
		items, closed := stringArray(value[1:])
		switch {
		case !closed:
			return nil, true, false
		case key == "dynamic" && slices.Contains(items, "dependencies"):
			return nil, true, false
		case key == "dependencies":
			for _, item := range items {
				if name := requirementName(item); name != "" {
					names = append(names, name)
				}
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

// Forms are the ways code writes a setting's name, in this order: as written, in lower case, in camel case.
func Forms(name string) []string {
	parts := strings.Split(strings.ToLower(name), "_")
	var camel strings.Builder
	camel.WriteString(parts[0])
	for _, part := range parts[1:] {
		if part != "" {
			camel.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	forms := []string{name}
	for _, form := range []string{strings.ToLower(name), camel.String()} {
		if !slices.Contains(forms, form) {
			forms = append(forms, form)
		}
	}
	return forms
}

// settingUses finds, for every setting name, the product and test files that hold one of its forms as a word, and the
// product files that declare a prefix of the name and hold the rest of it in lower case.
// NOTE: a prefix is matched per file, not per class; a field not in lower case, a prefix built at run time, an alias
// in another case and env_nested_delimiter are not seen. Upgrade: read class bodies.
func settingUses(rules conventions.Rules, settings *Settings, entries []Entry, files []File) []SettingUse {
	if settings == nil {
		return nil
	}
	names := slices.Compact(slices.Sorted(slices.Values(settings.Names)))
	roles := rolesOf(files)
	type hit struct{ product, test, rest, prefixed map[string]bool }
	hits := make([]hit, len(names))
	byForm := map[string][]int{}     // a form of a name -> the names it belongs to
	byRestForm := map[string][]int{} // a form of a name's remainder -> the names it belongs to
	for i, name := range names {
		hits[i] = hit{map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}}
		for _, form := range Forms(name) {
			byForm[form] = append(byForm[form], i)
		}
		if _, rest, found := strings.Cut(name, "_"); found && rest != "" {
			for _, form := range Forms(rest) {
				byRestForm[form] = append(byRestForm[form], i)
			}
		}
	}
	for _, entry := range entries {
		role, isCode := roles[entry.Path]
		if !isCode || role == Other || entry.Content == nil {
			continue
		}
		var prefixes []string
		if role == Product {
			prefixes = declaredPrefixes(rules, entry)
		}
		words := map[string]bool{} // filled only for a file that declares a prefix
		eachWord(entry.Content, func(w []byte) {
			if len(prefixes) > 0 {
				words[string(w)] = true
			}
			for _, i := range byForm[string(w)] {
				if role == Product {
					hits[i].product[entry.Path] = true
				} else {
					hits[i].test[entry.Path] = true
				}
			}
			if role == Product {
				for _, i := range byRestForm[string(w)] {
					hits[i].rest[entry.Path] = true
				}
			}
		})
		for _, prefix := range prefixes {
			for i, name := range names {
				rest, starts := strings.CutPrefix(name, strings.ToUpper(prefix))
				if starts && rest != "" && words[strings.ToLower(rest)] {
					hits[i].prefixed[entry.Path] = true
				}
			}
		}
	}
	found := make([]SettingUse, 0, len(names))
	for i, name := range names {
		use := SettingUse{Name: name, Product: sortedKeys(hits[i].product), Test: sortedKeys(hits[i].test), Prefixed: sortedKeys(hits[i].prefixed)}
		if len(use.Product)+len(use.Prefixed) == 0 && len(hits[i].rest) > 0 {
			_, use.Rest, _ = strings.Cut(name, "_")
		}
		found = append(found, use)
	}
	return found
}

// declaredPrefixes are the distinct settings prefixes a code file declares, by its language's pattern.
func declaredPrefixes(rules conventions.Rules, entry Entry) []string {
	language, known := rules.LanguageOf(entry.Path)
	if !known || language.SettingsPrefix == nil {
		return nil
	}
	group := language.SettingsPrefix.SubexpIndex("prefix")
	var prefixes []string
	for _, m := range language.SettingsPrefix.FindAllSubmatch(entry.Content, -1) {
		if prefix := string(m[group]); !slices.Contains(prefixes, prefix) {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}

// dynamicReads counts, per product file, the settings reads whose name is computed at run time.
func dynamicReads(rules conventions.Rules, entries []Entry, files []File) []DynamicRead {
	roles := rolesOf(files)
	var found []DynamicRead
	for _, entry := range entries {
		language, known := rules.LanguageOf(entry.Path)
		if !known || language.DynamicRead == nil || roles[entry.Path] != Product {
			continue
		}
		if n := len(language.DynamicRead.FindAllIndex(entry.Content, -1)); n > 0 {
			found = append(found, DynamicRead{Path: entry.Path, Count: n})
		}
	}
	slices.SortFunc(found, func(a, b DynamicRead) int { return strings.Compare(a.Path, b.Path) })
	return found
}

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

// eachWord calls found with every run of ASCII letters, digits and "_" in content: the words a setting's name is
// looked for among. A loop over the bytes, since a pattern match over every code file was the largest cost of a build.
func eachWord(content []byte, found func(word []byte)) {
	start := -1
	for i, c := range content {
		inWord := c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
		switch {
		case inWord && start < 0:
			start = i
		case !inWord && start >= 0:
			found(content[start:i])
			start = -1
		}
	}
	if start >= 0 {
		found(content[start:])
	}
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
