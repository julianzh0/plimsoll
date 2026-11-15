package snapshot

import (
	"regexp"
	"strings"
)

// pubspecNameLine is the top-level name of a pubspec.yaml, unquoted or quoted.
var pubspecNameLine = regexp.MustCompile(`(?m)^name:[ \t]*["']?([A-Za-z_][A-Za-z0-9_]*)["']?[ \t]*(?:#.*)?$`)

// pubspecName reads the package name: an import "package:<name>/x" is the file x under lib/.
func pubspecName(content []byte) prefix {
	m := pubspecNameLine.FindSubmatch([]byte(strings.TrimPrefix(string(content), byteOrderMark)))
	if m == nil {
		return prefix{state: PrefixUnreadable}
	}
	own := "package:" + string(m[1]) + "/"
	return prefix{state: PrefixRead, own: func(_, spec string) ([]string, bool) {
		rest, isOwn := strings.CutPrefix(spec, own)
		if !isOwn || rest == "" || leaves(rest) {
			return nil, false
		}
		return []string{"lib/" + rest}, false
	}}
}

// pubspecDependencies reads the keys of the top-level dependencies block by text: the lines indented one level below
// "dependencies:", up to the next top-level key. manifest is false when the file names no package; readable is false
// when the block is written in flow style.
// NOTE: only the block form is read; a flow-style map ("dependencies: {a: ^1}") is named unreadable. Upgrade: a YAML
// reader, if a field check finds flow style in use.
func pubspecDependencies(content []byte) (names []string, manifest, readable bool) {
	text := strings.TrimPrefix(string(content), byteOrderMark)
	if !pubspecNameLine.MatchString(text) {
		return nil, false, true
	}
	names = []string{}
	inBlock, indent := false, ""
	for line := range strings.Lines(text) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			key, value, _ := strings.Cut(trimmed, ":")
			if inBlock = key == "dependencies"; inBlock {
				value, _, _ = strings.Cut(value, "#")
				if strings.TrimSpace(value) != "" {
					return nil, true, false
				}
				indent = ""
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
		if lead != indent {
			continue // a nested line of an entry, such as "sdk: flutter"
		}
		if key, _, found := strings.Cut(trimmed, ":"); found {
			names = append(names, strings.Trim(key, `"'`))
		}
	}
	return names, true, true
}
