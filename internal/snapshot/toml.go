package snapshot

import (
	"maps"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// decodeTOML decodes a TOML file into plain maps. ok is false when it is not valid TOML: a reader then names the file
// as unreadable, never the decoder's message, which quotes the file.
func decodeTOML(content []byte) (document map[string]any, ok bool) {
	if err := toml.Unmarshal(content, &document); err != nil {
		return nil, false
	}
	return document, true
}

// tomlAt is the value at a path of keys, each a table but the last; found is false when a step is missing or no
// table.
func tomlAt(document map[string]any, keys ...string) (value any, found bool) {
	value = document
	for _, key := range keys {
		table, isTable := value.(map[string]any)
		if !isTable {
			return nil, false
		}
		if value, found = table[key]; !found {
			return nil, false
		}
	}
	return value, true
}

// tomlTableAt is the table at a path of keys, or nil.
func tomlTableAt(document map[string]any, keys ...string) map[string]any {
	value, _ := tomlAt(document, keys...)
	table, _ := value.(map[string]any)
	return table
}

// tomlText is the text at a path of keys, or "".
func tomlText(document map[string]any, keys ...string) string {
	value, _ := tomlAt(document, keys...)
	text, _ := value.(string)
	return text
}

// tomlTexts is a list of text: ok is false when the value is no list or holds anything but text.
func tomlTexts(value any) (texts []string, ok bool) {
	items, isList := value.([]any)
	if !isList {
		return nil, false
	}
	texts = make([]string, 0, len(items))
	for _, item := range items {
		text, isText := item.(string)
		if !isText {
			return nil, false
		}
		texts = append(texts, text)
	}
	return texts, true
}

// cargoDependencies reads the runtime dependencies of a Cargo.toml: the entries of [dependencies] and of every target
// table's dependencies ([target.<...>.dependencies]), in any TOML spelling. An entry renamed with package = "..." is
// named by its package. The dev, build and workspace tables add nothing. manifest is false when the file has neither
// a [package] table nor a dependency table: a workspace root that only lists members. readable is false when the file
// is no valid TOML, or a dependency table or a key in one cannot be read.
// NOTE: the dependencies of workspace members are not counted. Upgrade: read every tracked Cargo.toml.
func cargoDependencies(content []byte) (names []string, manifest, readable bool) {
	document, ok := decodeTOML(content)
	if !ok {
		return nil, true, false
	}
	tables := []any{}
	if table, found := document["dependencies"]; found {
		tables = append(tables, table)
	}
	for _, target := range tomlTableAt(document, "target") {
		if platform, isTable := target.(map[string]any); isTable && platform["dependencies"] != nil {
			tables = append(tables, platform["dependencies"])
		}
	}
	_, hasPackage := document["package"]
	if !hasPackage && len(tables) == 0 {
		return nil, false, true
	}
	names = []string{}
	for _, value := range tables {
		table, isTable := value.(map[string]any)
		if !isTable {
			return nil, true, false
		}
		for _, key := range slices.Sorted(maps.Keys(table)) {
			name := key
			if renamed := tomlText(table, key, "package"); renamed != "" {
				name = renamed
			}
			if name == "" || strings.ContainsAny(name, " \t") {
				return nil, true, false
			}
			names = appendNew(names, name)
		}
	}
	slices.Sort(names)
	return names, true, true
}

// pyprojectDependencies reads the names of [project] dependencies. manifest is false when the file has no [project]
// table; readable is false when the file is no valid TOML, or its dependencies are no list of text or are dynamic.
func pyprojectDependencies(content []byte) (names []string, manifest, readable bool) {
	document, ok := decodeTOML(content)
	if !ok {
		return nil, true, false
	}
	project, isTable := document["project"].(map[string]any)
	if !isTable {
		return nil, false, true
	}
	if dynamic, found := project["dynamic"]; found {
		listed, isList := tomlTexts(dynamic)
		if !isList || slices.Contains(listed, "dependencies") {
			return nil, true, false
		}
	}
	names = []string{}
	value, found := project["dependencies"]
	if !found {
		return names, true, true
	}
	requirements, isList := tomlTexts(value)
	if !isList {
		return nil, true, false
	}
	for _, requirement := range requirements {
		if name := requirementName(requirement); name != "" {
			names = append(names, name)
		}
	}
	return names, true, true
}

// readDeclared reads one declared key from a file's content: its values, and what the file says about it. The value
// is a list of text, or one text that holds values separated by white space.
func readDeclared(content []byte, d conventions.Declaration) ([]string, KeyState) {
	if content == nil {
		return nil, 0
	}
	document, ok := decodeTOML(content)
	if !ok {
		return nil, 0 // the manifest reader names a file that is no valid TOML
	}
	value, found := tomlAt(document, append(strings.Split(d.Table, "."), d.Key)...)
	if !found {
		return nil, 0
	}
	if text, isText := value.(string); isText {
		return strings.Fields(text), KeyRead
	}
	if texts, isList := tomlTexts(value); isList {
		return texts, KeyRead
	}
	return nil, KeyUnreadable
}
