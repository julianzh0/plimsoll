package snapshot

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/julianzh0/plimsoll/internal/conventions"
)

// decodeTOML decodes a TOML file into plain maps. An error means it is not valid TOML: a reader then names the file
// as unreadable, never the error's message, which quotes the file (notTOML).
func decodeTOML(content []byte) (document map[string]any, err error) {
	err = toml.Unmarshal(content, &document)
	return document, err
}

// notTOML says why a file is no valid TOML: the line where decoding stopped, never the decoder's message.
func notTOML(err error) string {
	if decodeError, ok := errors.AsType[*toml.DecodeError](err); ok {
		if row, _ := decodeError.Position(); row > 0 {
			return fmt.Sprintf("it is not valid TOML at line %d", row)
		}
	}
	return "it is not valid TOML"
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

// readsConfig says whether a tool reads a configuration of the given content: a file it reads whenever tracked, or
// one that holds the configuration's section.
func readsConfig(content map[string][]byte, c conventions.Config) bool {
	file, tracked := content[c.File]
	switch {
	case !tracked:
		return false
	case c.Always:
		return true
	case c.INI:
		return iniHolds(file, c.Table)
	}
	document, _ := decodeTOML(file) // a file that is no TOML holds no section
	if c.Present {
		return tomlTableAt(document, strings.Split(c.Table, ".")...) != nil
	}
	return tomlHolds(document, c.Table)
}

// tomlHolds says whether a document holds a section at a dotted path: a table with a key of its own, one that is no
// table. An empty table, or one that only holds other tables (tool.pytest above tool.pytest.ini_options), holds none,
// as pytest counts its sections.
func tomlHolds(document map[string]any, dotted string) bool {
	for _, value := range tomlTableAt(document, strings.Split(dotted, ".")...) {
		if _, isTable := value.(map[string]any); !isTable {
			return true
		}
	}
	return false
}

// cargoWorkspace is the [workspace.dependencies] table of the nearest tracked Cargo.toml at or above a folder that has
// a [workspace] table, from which an entry with workspace = true takes its package; nil when there is none.
func cargoWorkspace(content map[string][]byte, folder string) map[string]any {
	for {
		if file := content[path.Join(folder, "Cargo.toml")]; file != nil {
			if document, err := decodeTOML(file); err == nil && document["workspace"] != nil {
				return tomlTableAt(document, "workspace", "dependencies")
			}
		}
		if folder == "." {
			return nil
		}
		folder = path.Dir(folder)
	}
}

// cargoDependencies reads the runtime dependencies of a Cargo.toml: the entries of [dependencies] and of every target
// table's dependencies ([target.<...>.dependencies]), in any TOML spelling. An entry renamed with package = "..." is
// named by its package, also one that inherits its entry from the workspace (workspace = true). The dev, build and
// workspace tables add nothing. manifest is false when the file has neither
// a [package] table nor a dependency table: a workspace root that only lists members. readable is false when the file
// is no valid TOML, or a dependency table or a key in one cannot be read.
func cargoDependencies(content []byte, workspace map[string]any) (names []string, manifest, readable bool) {
	document, err := decodeTOML(content)
	if err != nil {
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
			if inherits, _ := tomlAt(table, key, "workspace"); inherits == true {
				if renamed := tomlText(workspace, key, "package"); renamed != "" {
					name = renamed
				}
			}
			if renamed := tomlText(table, key, "package"); renamed != "" {
				name = renamed
			}
			if name == "" || strings.ContainsAny(name, " \t") {
				return nil, true, false
			}
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names), true, true
}

// pyprojectDependencies reads the names of [project] dependencies. manifest is false when the file has no [project]
// table; readable is false when the file is no valid TOML, or its dependencies are no list of text or are dynamic.
func pyprojectDependencies(content []byte) (names []string, manifest, readable bool) {
	document, err := decodeTOML(content)
	if err != nil {
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

// readDeclared reads one declared key from a file's content: its values, and what the file says about it. In TOML the
// value is a list of text, or one text that holds values separated by white space; in INI it holds values separated
// as a shell separates arguments. A TOML file that holds the declaration's rival table beside its own cannot be read.
func readDeclared(content []byte, d conventions.Declaration) ([]string, KeyState) {
	if content == nil {
		return nil, 0
	}
	if d.INI {
		value, found := iniValue(content, d.Table, d.Key)
		if !found {
			return nil, 0
		}
		values, ok := shellWords(value)
		if !ok {
			return nil, KeyUnreadable
		}
		return values, KeyRead
	}
	document, err := decodeTOML(content)
	switch {
	case err != nil && d.Always: // the tool's own file: nothing else names it
		return nil, KeyUnreadable
	case err != nil:
		return nil, 0 // the manifest reader names a manifest that is no valid TOML
	}
	value, found := tomlAt(document, append(strings.Split(d.Table, "."), d.Key)...)
	switch {
	case !found:
		return nil, 0
	case d.Rival != "" && tomlHolds(document, d.Rival):
		return nil, KeyUnreadable
	}
	if text, isText := value.(string); isText {
		return strings.Fields(text), KeyRead
	}
	if texts, isList := tomlTexts(value); isList {
		return texts, KeyRead
	}
	return nil, KeyUnreadable
}
