package snapshot

import (
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

// pubspecNameLine is the top-level name of a pubspec.yaml, unquoted or quoted.
var pubspecNameLine = regexp.MustCompile(`(?m)^name:[ \t]*["']?([A-Za-z_][A-Za-z0-9_]*)["']?[ \t]*(?:#[^\r\n]*)?\r?$`)

// pubspecPath is a path key in a dependency's block or flow map, with its value: never a key that ends in "path".
var pubspecPath = regexp.MustCompile(`(?:^|[\s{,])path:[ \t]*["']?([^"',}\s]+)`)

// pubspecPackages reads every tracked pubspec.yaml with a name as a package, and every one with a workspace list as a
// workspace whose entries, globs with * per segment included, name member folders. An import "package:<name>/x" is the
// file x under lib/ of the package of that name when it is the importing package itself, a member of a workspace the
// importing package belongs to, or a path dependency of the importing package; otherwise it is outside.
func pubspecPackages(content map[string][]byte, tracked map[string]bool) prefix {
	packages := map[string]string{} // a name to its package's folder
	names := map[string]string{}    // a package's folder to its name
	var manifests []string
	for file := range tracked {
		if path.Base(file) == "pubspec.yaml" {
			manifests = append(manifests, file)
		}
	}
	slices.Sort(manifests)
	for _, file := range manifests {
		if name := pubspecNameOf(content[file]); name != "" {
			if _, taken := packages[name]; !taken {
				packages[name] = path.Dir(file)
			}
			names[path.Dir(file)] = name
		}
	}
	if len(packages) == 0 {
		if len(manifests) > 0 {
			return prefix{state: PrefixUnreadable}
		}
		return prefix{state: PrefixAbsent}
	}
	workspaceOf := map[string][]string{} // a package's folder to the workspace roots that list it, and the root itself
	for _, file := range manifests {
		root := path.Dir(file)
		for _, member := range pubspecBlock(content[file], "workspace") {
			pattern := path.Join(root, strings.Trim(strings.TrimPrefix(member, "- "), `"' `))
			for folder := range names {
				if matched, _ := path.Match(pattern, folder); matched {
					workspaceOf[folder] = append(workspaceOf[folder], root)
				}
			}
			workspaceOf[root] = appendNew(workspaceOf[root], root)
		}
	}
	pathDependencies := map[string][]string{} // a package's folder to the folders it depends on by path
	for folder := range names {
		pathDependencies[folder] = pubspecPathDependencies(content[path.Join(folder, "pubspec.yaml")], folder)
	}
	return prefix{state: PrefixRead, own: func(from, spec string, _ []string) ([]string, bool) {
		packageName, rest, found := strings.Cut(strings.TrimPrefix(spec, "package:"), "/")
		if !strings.HasPrefix(spec, "package:") || !found || rest == "" || leaves(rest) {
			return nil, false
		}
		var importer string
		for folder := range foldersUp(path.Dir(from)) {
			if _, isPackage := names[folder]; isPackage {
				importer = folder
				break
			}
		}
		if importer == "" {
			return nil, false
		}
		// the importing package itself, then a package it depends on by path, then a member of its workspace
		candidates := append([]string{importer}, pathDependencies[importer]...)
		for _, folder := range slices.Sorted(maps.Keys(names)) { // of members of one name, the first by folder
			if slices.ContainsFunc(workspaceOf[importer], func(root string) bool { return slices.Contains(workspaceOf[folder], root) }) {
				candidates = append(candidates, folder)
			}
		}
		for _, folder := range candidates {
			if names[folder] == packageName {
				return []string{path.Join(folder, "lib", rest)}, false
			}
		}
		return nil, false
	}}
}

// pubspecPathDependencies are the folders a pubspec's dependencies name by a local path: an entry of dependencies,
// dev_dependencies or dependency_overrides with a path key, in a block or a flow map (also on the key's own line),
// resolved against the pubspec's folder.
func pubspecPathDependencies(content []byte, folder string) []string {
	var found []string
	inBlock := false
	for line := range strings.Lines(string(content)) {
		code, _, _ := strings.Cut(line, " #")
		if trimmed := strings.TrimSpace(code); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			key, value, _ := strings.Cut(code, ":")
			inBlock = key == "dependencies" || key == "dev_dependencies" || key == "dependency_overrides"
			code = value
		}
		for _, m := range pubspecPath.FindAllStringSubmatch(code, -1) {
			if inBlock {
				found = append(found, path.Join(folder, m[1]))
			}
		}
	}
	return found
}

// pubspecNameOf is a pubspec.yaml's top-level name, or "" when it has none.
func pubspecNameOf(content []byte) string {
	if m := pubspecNameLine.FindSubmatch(content); m != nil {
		return string(m[1])
	}
	return ""
}

// pubspecBlock is the entries one level below a top-level key of a pubspec.yaml, without comments; nil when the key
// is absent or its value is not a block.
func pubspecBlock(content []byte, key string) []string {
	lines, _ := pubspecBlockLines(content, key)
	return lines
}

// pubspecDependencies reads the keys of the top-level dependencies by text: the lines indented one level below
// "dependencies:", up to the next top-level key, or the keys of a flow map ("dependencies: {a: ^1, b: {path: ../b}}")
// on one line or several. manifest is false when the file names no package, or is a workspace root without
// dependencies of its own; readable is false when a flow value is no map whose keys it can read.
func pubspecDependencies(text []byte) (names []string, manifest, readable bool) {
	if pubspecNameOf(text) == "" {
		return nil, false, true
	}
	lines, flow := pubspecBlockLines(text, "dependencies")
	switch {
	case flow != "":
		names, readable = flowMapKeys(flow)
		return names, true, readable
	case lines == nil && pubspecBlock(text, "workspace") != nil:
		return nil, false, true
	}
	names = []string{}
	for _, line := range lines {
		if key, _, found := strings.Cut(line, ":"); found {
			names = append(names, strings.Trim(key, `"' `))
		}
	}
	return names, true, true
}

// pubspecBlockLines finds a top-level key's block: its entries one level below it, trimmed, without comments and
// without the deeper lines of an entry. flow is the key's value when it stands on the key's own line, with the
// following lines up to the end of an open flow collection.
func pubspecBlockLines(content []byte, key string) (lines []string, flow string) {
	inBlock, indent := false, ""
	for line := range strings.Lines(string(content)) {
		if flow != "" && flowDepth(flow) > 0 {
			flow += " " + strings.TrimSpace(line)
			continue
		}
		trimmed, _, _ := strings.Cut(strings.TrimSpace(line), " #")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			found, value, _ := strings.Cut(trimmed, ":")
			if inBlock = found == key; inBlock {
				if value = strings.TrimSpace(value); value != "" {
					flow, inBlock = value, false
					continue
				}
				lines = []string{}
			}
			continue
		}
		if !inBlock {
			continue
		}
		lead := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if indent == "" {
			indent = lead
		}
		if lead == indent {
			lines = append(lines, trimmed)
		}
	}
	return lines, flow
}

// flowDepth is how many flow collections ({ or [) a text leaves open, quotes respected.
func flowDepth(text string) int {
	depth, quote := 0, byte(0)
	for i := 0; i < len(text); i++ {
		switch c := text[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{' || c == '[':
			depth++
		case c == '}' || c == ']':
			depth--
		}
	}
	return depth
}

// flowMapKeys are the keys of a flow map's top level, quotes dropped: "{a: ^1, 'b': {path: ../b}}" gives a and b. ok is
// false for a value that is no flow map or does not close.
func flowMapKeys(text string) (keys []string, ok bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "{") || !strings.HasSuffix(text, "}") || flowDepth(text) != 0 {
		return nil, false
	}
	keys = []string{}
	depth, quote, start := 0, byte(0), 1
	for i := 1; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			continue
		case c == '"' || c == '\'':
			quote = c
			continue
		case c == '{' || c == '[':
			depth++
			continue
		case c == '}' && depth == 0, c == ',' && depth == 0:
		case c == '}' || c == ']':
			if depth--; depth < 0 {
				return nil, false
			}
			continue
		default:
			continue
		}
		if entry := strings.TrimSpace(text[start:i]); entry != "" {
			key, _, found := strings.Cut(entry, ":")
			if !found {
				return nil, false
			}
			keys = append(keys, strings.Trim(strings.TrimSpace(key), `"'`))
		}
		start = i + 1
	}
	return keys, true
}
