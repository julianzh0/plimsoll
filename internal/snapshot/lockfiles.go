package snapshot

import (
	"encoding/json"
	"path"
	"slices"
	"strings"
)

// lockNames reads the package names a lockfile lists, by the layout its tool writes. ok is false for a lockfile in a
// layout it does not know, or one that does not decode: its names are then matched by text, as before any reader of
// its format existed, and the answer says so.
func lockNames(lockPath string, content []byte) (names map[string]bool, ok bool) {
	var list []string
	switch path.Base(lockPath) {
	case "Cargo.lock":
		list, ok = tomlLockNames(content, func(document map[string]any) bool {
			version, has := document["version"]
			return !has || version == int64(3) || version == int64(4)
		})
	case "uv.lock":
		list, ok = tomlLockNames(content, func(document map[string]any) bool { return document["version"] == int64(1) })
	case "poetry.lock":
		list, ok = tomlLockNames(content, func(document map[string]any) bool {
			version := tomlText(document, "metadata", "lock-version")
			return strings.HasPrefix(version, "1.") || strings.HasPrefix(version, "2.")
		})
	case "package-lock.json":
		list, ok = npmLockNames(content)
	case "pnpm-lock.yaml":
		list, ok = pnpmLockNames(content)
	case "yarn.lock":
		list, ok = yarnLockNames(content)
	case "pubspec.lock":
		list, ok = yamlBlockKeys(content, "packages")
	case "go.sum":
		list, ok = goSumNames(content)
	case "Package.resolved":
		list, ok = swiftResolvedNames(content)
	}
	if !ok {
		return nil, false
	}
	names = make(map[string]bool, len(list))
	for _, name := range list {
		names[name] = true
	}
	return names, true
}

// tomlLockNames are the names of a TOML lockfile's [[package]] array, when its version marker is one the reader knows.
func tomlLockNames(content []byte, known func(map[string]any) bool) ([]string, bool) {
	document, err := decodeTOML(content)
	if err != nil || !known(document) {
		return nil, false
	}
	packages, isArray := document["package"].([]any)
	if !isArray {
		return nil, false
	}
	var names []string
	for _, entry := range packages {
		if table, isTable := entry.(map[string]any); isTable {
			if name, isText := table["name"].(string); isText {
				names = append(names, name)
			}
		}
	}
	return names, true
}

// npmLockNames are the names of a package-lock.json: version 1 lists them as the keys of "dependencies" at every
// depth, versions 2 and 3 as the keys of "packages" after their last "node_modules/". The root "" and the paths of
// workspace members name no installed package.
func npmLockNames(content []byte) ([]string, bool) {
	var lock struct {
		Version      int                        `json:"lockfileVersion"`
		Dependencies map[string]json.RawMessage `json:"dependencies"`
		Packages     map[string]json.RawMessage `json:"packages"`
	}
	if json.Unmarshal(content, &lock) != nil {
		return nil, false
	}
	var names []string
	switch {
	case lock.Version == 1 && lock.Dependencies != nil:
		var walk func(map[string]json.RawMessage)
		walk = func(dependencies map[string]json.RawMessage) {
			for name, raw := range dependencies {
				names = append(names, name)
				var entry struct {
					Dependencies map[string]json.RawMessage `json:"dependencies"`
				}
				if json.Unmarshal(raw, &entry) == nil {
					walk(entry.Dependencies)
				}
			}
		}
		walk(lock.Dependencies)
	case (lock.Version == 2 || lock.Version == 3) && lock.Packages != nil:
		for key := range lock.Packages {
			if at := strings.LastIndex(key, "node_modules/"); at >= 0 {
				names = append(names, key[at+len("node_modules/"):])
			}
		}
	default:
		return nil, false
	}
	return names, true
}

// pnpmLockNames are the names of a pnpm-lock.yaml's packages: "/name/version" in version 5, "/name@version" in 6,
// "name@version" in 9, a scoped name keeping its "@" and a peer suffix in parentheses dropped.
func pnpmLockNames(content []byte) ([]string, bool) {
	version := yamlTopValue(content, "lockfileVersion")
	keys, hasPackages := yamlBlockKeys(content, "packages")
	if !hasPackages {
		return nil, false
	}
	var names []string
	for _, key := range keys {
		key, _, _ = strings.Cut(key, "(")
		switch {
		case strings.HasPrefix(version, "5."):
			key = strings.TrimPrefix(key, "/")
			if at := strings.LastIndex(key, "/"); at > 0 {
				names = append(names, key[:at])
			}
		case strings.HasPrefix(version, "6."), strings.HasPrefix(version, "9."):
			key = strings.TrimPrefix(key, "/")
			if at := strings.LastIndex(key, "@"); at > 0 {
				names = append(names, key[:at])
			}
		default:
			return nil, false
		}
	}
	return names, true
}

// yarnLockNames are the names of a yarn.lock, version 1 (its comment header) or Berry (its __metadata with a version
// from 4 to 8): each header at column 0 lists specifiers, each a name before its last "@" range.
func yarnLockNames(content []byte) ([]string, bool) {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	berryVersion := yamlNestedValue(text, "__metadata", "version")
	switch {
	case strings.Contains(text, "# yarn lockfile v1"):
	case len(berryVersion) == 1 && berryVersion >= "4" && berryVersion <= "8":
	default:
		return nil, false
	}
	var names []string
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\n")
		if line == "" || line[0] == ' ' || line[0] == '#' || strings.HasPrefix(line, "__metadata") || !strings.HasSuffix(line, ":") {
			continue
		}
		for spec := range strings.SplitSeq(strings.TrimSuffix(line, ":"), ",") {
			spec = strings.Trim(strings.TrimSpace(spec), `"`)
			if at := strings.LastIndex(spec, "@"); at > 0 {
				names = append(names, spec[:at])
			}
		}
	}
	return names, true
}

// goSumNames are the module paths of a go.sum: every line holds three fields.
func goSumNames(content []byte) ([]string, bool) {
	var names []string
	for line := range strings.Lines(string(content)) {
		fields := strings.Fields(line)
		switch len(fields) {
		case 0:
		case 3:
			names = append(names, fields[0])
		default:
			return nil, false
		}
	}
	return names, true
}

// swiftResolvedNames are the identities of a Package.resolved: in version 1 the last part of each pin's repository
// URL, lower case, without ".git"; in versions 2 and 3 each pin's identity.
func swiftResolvedNames(content []byte) ([]string, bool) {
	type pin struct {
		Identity      string `json:"identity"`
		RepositoryURL string `json:"repositoryURL"`
	}
	var resolved struct {
		Version int   `json:"version"`
		Pins    []pin `json:"pins"`
		Object  struct {
			Pins []pin `json:"pins"`
		} `json:"object"`
	}
	if json.Unmarshal(content, &resolved) != nil {
		return nil, false
	}
	var names []string
	switch resolved.Version {
	case 1:
		for _, p := range resolved.Object.Pins {
			names = append(names, strings.ToLower(strings.TrimSuffix(path.Base(p.RepositoryURL), ".git")))
		}
	case 2, 3:
		for _, p := range resolved.Pins {
			names = append(names, strings.ToLower(p.Identity))
		}
	default:
		return nil, false
	}
	return names, true
}

// yamlBlockKeys are the keys one level below a top-level key of a YAML file their tools write in block style: the
// lines indented once below it, up to the next top-level line, quotes dropped. found is false without the key.
func yamlBlockKeys(content []byte, key string) (keys []string, found bool) {
	indent := ""
	for line := range strings.Lines(strings.ReplaceAll(string(content), "\r\n", "\n")) {
		line = strings.TrimRight(line, "\n")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			continue
		case line[0] != ' ':
			if found {
				return keys, true
			}
			found = line == key+":"
			continue
		case !found:
			continue
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " "))]
		if indent == "" {
			indent = lead
		}
		if lead == indent && strings.HasSuffix(trimmed, ":") {
			keys = append(keys, strings.Trim(strings.TrimSuffix(trimmed, ":"), `'"`))
		}
	}
	return keys, found
}

// yamlTopValue is the value of a top-level "key: value" line, quotes dropped.
func yamlTopValue(content []byte, key string) string {
	for line := range strings.Lines(string(content)) {
		if value, found := strings.CutPrefix(strings.TrimRight(line, "\r\n"), key+":"); found {
			return strings.Trim(strings.TrimSpace(value), `'"`)
		}
	}
	return ""
}

// yamlNestedValue is the value of "child: value" one level below a top-level key.
func yamlNestedValue(text, parent, child string) string {
	in := false
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\n")
		switch {
		case line == parent+":":
			in = true
		case line != "" && line[0] != ' ':
			in = false
		case in:
			if value, found := strings.CutPrefix(strings.TrimSpace(line), child+":"); found {
				return strings.Trim(strings.TrimSpace(value), `'"`)
			}
		}
	}
	return ""
}

// lockHolds says whether a lockfile's names hold a dependency, compared as its ecosystem compares names: Python
// names by PEP 503, every other name exactly (SwiftPM names are lower case on both sides).
func lockHolds(names map[string]bool, lockPath, name string) bool {
	if slices.Contains([]string{"uv.lock", "poetry.lock"}, path.Base(lockPath)) {
		for listed := range names {
			if requirementName(listed) == name {
				return true
			}
		}
		return false
	}
	return names[name]
}
