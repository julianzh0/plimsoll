package conventions

import "regexp"

// The Python entry: what the shared rules cannot know for Python files.

// pythonLanguage is the entry of Python.
func pythonLanguage() Language {
	return Language{
		Name: "python", Extensions: []string{".py", ".pyi"},
		DynamicRead: pythonDynamicRead, SettingsPrefix: pythonSettingsPrefix, Imports: pythonImports(), Layout: pytestLayout(),
	}
}

var (
	// A settings read whose name is computed at run time: not a quoted name after the opening.
	pythonDynamicRead = regexp.MustCompile(`os\.environ\[[ \t]*[^"' \t\]]|os\.(?:getenv|environ\.get)\([ \t]*[^"' \t)]`)
	// A settings class that declares the prefix of its names: pydantic-settings' env_prefix.
	pythonSettingsPrefix = regexp.MustCompile(`env_prefix[ \t]*=[ \t]*["'](?P<prefix>[A-Za-z0-9_]+)["']`)
)

// pyNames are the names after "import" in a from statement: a parenthesised list over lines, with comments, or the rest
// of the line up to a ";", with backslash continuations.
const pyNames = `\((?:[^()#\n]|#[^\n]*|\n)*\)|(?:[^\n\\;]|\\\n)*`

var (
	pyFromImport = regexp.MustCompile(`(?m)^[ \t]*(?:lazy[ \t]+)?from[ \t]+(?P<spec>\.*[A-Za-z_][\w.]*|\.+)[ \t]+import[ \t]*(?P<names>` + pyNames + `)`)
	// NOTE: a second module on an import line ("import a, b") is not read. Upgrade: split the list.
	pyImport   = regexp.MustCompile(`(?m)^[ \t]*(?:lazy[ \t]+)?import[ \t]+(?P<spec>[A-Za-z_][\w.]*)(?:[ \t]+as[ \t]+\w+)?[ \t\r]*(?:[,;#][^\n]*)?$`)
	pyPassesOn = regexp.MustCompile(`\A(?:[ \t\r]*(?:#[^\n]*)?\n` +
		`|[ \t]*(?:"""(?:[^"]|"[^"]|""[^"])*"""|'''(?:[^']|'[^']|''[^'])*''')[ \t\r]*\n` +
		`|from[ \t]+[.\w]+[ \t]+import[ \t]*(?:` + pyNames + `)\n` +
		`|__all__[ \t]*(?::[^=\n]*)?=[ \t]*[\[(](?:[^\])#]|#[^\n]*)*[\])][ \t\r]*\n)*\z`)
	pyCall     = regexp.MustCompile(`(?:\bimport_module|\b__import__)\([ \t\r\n]*["'](?P<spec>[A-Za-z_][\w.]*)["'][ \t\r\n]*[,)]`)
	pyComputed = regexp.MustCompile(`(?:\bimport_module|\b__import__)\([ \t\r\n]*(?:[^"'\s)]|["']\.|["'][^"'\n]*["'][ \t\r\n]*[^\s,)])`)
	// A block under "if TYPE_CHECKING:" at the top level runs only for the type checker: its blank and indented lines.
	pyTypeOnlyBlock = regexp.MustCompile(`(?m)^if[ \t]+(?:\w+\.)?TYPE_CHECKING[ \t]*:[^\n]*\n(?:[ \t\r]*\n|[ \t]+[^\n]*(?:\n|\z))*`)
)

// pyCallWords are the words a call import needs; a file without them is not searched for one.
func pyCallWords() []string { return []string{"import_module(", "__import__("} }

func pythonImports() Imports {
	return Imports{
		Statements:    []ImportForm{{Pattern: pyFromImport}, {Pattern: pyImport}, {Pattern: pyCall, Words: pyCallWords()}},
		Computed:      ImportForm{Pattern: pyComputed, Words: pyCallWords()},
		TypeOnlyBlock: ImportForm{Pattern: pyTypeOnlyBlock, Words: []string{"TYPE_CHECKING"}},
		PassesOn:      pyPassesOn,
		Relative:      LeadingDots,
		Prefix:        Prefix{Roots: []string{".", "src"}, Format: OwnPackages, Required: true},
		Target:        FileTarget,
		Candidates:    Candidates{Add: []string{".py"}, InFolder: []string{"__init__.py"}},
		Implicit:      []string{"__init__.py", "__main__.py"},
	}
}

// pytestLayout is what pytest's table in pyproject.toml declares: where tests are collected and which folders join
// the import path. pytest reads the first of its own files it finds instead, so a tracked one shadows the table.
func pytestLayout() []Declaration {
	shadowed := []string{"pytest.toml", ".pytest.toml", "pytest.ini", ".pytest.ini"}
	var rows []Declaration
	for _, table := range []string{"tool.pytest.ini_options", "tool.pytest"} {
		rows = append(rows,
			Declaration{File: "pyproject.toml", Table: table, Key: "testpaths", Means: TestFolders, Tool: "pytest", Shadowed: shadowed},
			Declaration{File: "pyproject.toml", Table: table, Key: "pythonpath", Means: Roots, Tool: "pytest", Shadowed: shadowed},
		)
	}
	return rows
}
