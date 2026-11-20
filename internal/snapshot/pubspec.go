package snapshot

import (
	"path"
	"regexp"
	"strings"
)

// pubspecNameLine is the top-level name of a pubspec.yaml, unquoted or quoted.
var pubspecNameLine = regexp.MustCompile(`(?m)^name:[ \t]*["']?([A-Za-z_][A-Za-z0-9_]*)["']?[ \t]*(?:#.*)?$`)

// pubspecPackages reads the package name of the root pubspec.yaml and of the workspace members it lists: an import
// "package:<name>/x" is the file x under the lib/ folder of the package with that name.
// NOTE: members are read as listed; a glob in the workspace list is not expanded. Upgrade: match the globs against
// the tracked paths, if a field check finds them in use.
func pubspecPackages(root []byte, content map[string][]byte) prefix {
	name := pubspecNameOf(root)
	if name == "" {
		return prefix{state: PrefixUnreadable}
	}
	packages := map[string]string{name: "lib"}
	for _, member := range pubspecBlock(root, "workspace") {
		member = strings.Trim(strings.TrimPrefix(member, "- "), `"' `)
		if memberName := pubspecNameOf(content[path.Join(member, "pubspec.yaml")]); memberName != "" && !strings.Contains(member, "*") {
			packages[memberName] = path.Join(member, "lib")
		}
	}
	return prefix{state: PrefixRead, own: func(_, spec string, _ []string) ([]string, bool) {
		packageName, rest, found := strings.Cut(strings.TrimPrefix(spec, "package:"), "/")
		base, own := packages[packageName]
		if !strings.HasPrefix(spec, "package:") || !found || !own || rest == "" || leaves(rest) {
			return nil, false
		}
		return []string{path.Join(base, rest)}, false
	}}
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
