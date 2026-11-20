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

// pubspecDependencies reads the keys of the top-level dependencies block by text: the lines indented one level below
// "dependencies:", up to the next top-level key. manifest is false when the file names no package, or is a workspace
// root without dependencies of its own; readable is false when the block is written in flow style.
// NOTE: only the block form is read; a flow-style map ("dependencies: {a: ^1}") is named unreadable, and the
// dependencies of workspace members are not counted. Upgrade: a YAML reader, and the members' manifests, if a field
// check finds either in use.
func pubspecDependencies(content []byte) (names []string, manifest, readable bool) {
	text := content
	if pubspecNameOf(text) == "" {
		return nil, false, true
	}
	lines, flow := pubspecBlockLines(text, "dependencies")
	switch {
	case flow:
		return nil, true, false
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
// without the deeper lines of an entry. flow is true when the key's value stands on its own line.
func pubspecBlockLines(content []byte, key string) (lines []string, flow bool) {
	inBlock, indent := false, ""
	for line := range strings.Lines(string(content)) {
		trimmed, _, _ := strings.Cut(strings.TrimSpace(line), " #")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			found, value, _ := strings.Cut(trimmed, ":")
			if inBlock = found == key; inBlock {
				if strings.TrimSpace(value) != "" {
					return nil, true
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
	return lines, false
}
