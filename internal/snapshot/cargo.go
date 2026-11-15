package snapshot

import (
	"slices"
	"strings"
)

// cargoDependencies reads the runtime dependencies of a Cargo.toml by text: the keys of [dependencies] and of the
// target tables ([target.<...>.dependencies]), and the name of a sub-table ([dependencies.<name>],
// [target.<...>.dependencies.<name>]). The dev, build and workspace tables add nothing. manifest is false when the
// file has neither a [package] table nor a dependency table: a workspace root that only lists members. readable is
// false when a line of a table of keys is no key.
// NOTE: the dependencies of workspace members are not counted, and a dependency renamed with package = "..." is
// listed by its key. Upgrade: read every tracked Cargo.toml.
func cargoDependencies(content []byte) (names []string, manifest, readable bool) {
	text := strings.TrimPrefix(string(content), byteOrderMark)
	names = []string{}
	add := func(name string) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	inKeys, depth := false, 0
	for line := range strings.Lines(text) {
		if depth > 0 {
			depth += openedBrackets(line)
			continue
		}
		if header, isHeader := tomlHeader(line); isHeader {
			table, name := cargoTable(header)
			inKeys = table && name == ""
			manifest = manifest || table || header == "package"
			if name != "" {
				add(name)
			}
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		depth += openedBrackets(line)
		if !inKeys {
			continue
		}
		key, ok := cargoKey(trimmed)
		if !ok {
			return nil, true, false
		}
		add(key)
	}
	if !manifest {
		return nil, false, true
	}
	return names, true, true
}

// cargoTable says whether a header opens a runtime dependency table, and the dependency it names when it is a
// sub-table of one.
func cargoTable(header string) (table bool, name string) {
	parts := strings.Split(header, ".")
	last := len(parts) - 1
	isTarget := len(parts) >= 3 && parts[0] == "target"
	switch {
	case header == "dependencies" || isTarget && parts[last] == "dependencies":
		return true, ""
	case len(parts) == 2 && parts[0] == "dependencies", isTarget && len(parts) >= 4 && parts[last-1] == "dependencies":
		return true, parts[last]
	}
	return false, ""
}

// cargoKey reads the key a line assigns, bare or quoted, up to "=" or to its first "." (log.workspace = true is log).
func cargoKey(line string) (string, bool) {
	name, _, assigns := strings.Cut(line, "=")
	key, _, _ := strings.Cut(dottedName(name), ".")
	if !assigns || key == "" || strings.ContainsAny(key, " \t") {
		return "", false
	}
	return key, true
}

// openedBrackets counts the brackets and braces a line opens minus those it closes, outside strings and before a
// comment, so a value that goes on over lines is followed to its end.
func openedBrackets(line string) int {
	depth := 0
	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case '#':
			return depth
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case '"', '\'':
			for i++; i < len(line) && line[i] != c; i++ {
				if c == '"' && line[i] == '\\' {
					i++
				}
			}
		}
	}
	return depth
}
