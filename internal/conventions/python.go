package conventions

// The Python entry: what the shared rules cannot know for Python files.

// pythonLanguage is the entry of Python.
func pythonLanguage() Language {
	return Language{
		Name: "python", Extensions: []string{".py", ".pyi"},
		Imports: pythonImports(), Layout: pytestLayout(), Grammar: "python", Query: pythonQuery,
	}
}

// pythonQuery reads `import` and `from` statements with the names they import, `import_module()` and `__import__()`
// with a plain string first (any other first argument is an import built at run time), the imports under a top-level
// `if TYPE_CHECKING:`, what a file that only passes imports on may hold, settings reads through a computed name, and
// pydantic's env_prefix.
const pythonQuery = `
(import_from_statement module_name: (_) @spec) @import
(import_from_statement name: (dotted_name) @name)
(import_from_statement name: (aliased_import name: (dotted_name) @name))
(future_import_statement "__future__" @spec) @import
(future_import_statement name: (dotted_name) @name)
(import_statement name: [(dotted_name) @spec (aliased_import name: (dotted_name) @spec)]) @import
((call
   function: [(identifier) @f (attribute attribute: (identifier) @f)]
   arguments: (argument_list . (string . (string_start) @q . (string_content) @spec . (string_end) .))) @import
  (#match? @f "^(import_module|__import__)$") (#match? @q "^[\"']") (#match? @spec "^[A-Za-z_][A-Za-z0-9_.]*$"))
((call function: [(identifier) @f (attribute attribute: (identifier) @f)] arguments: (argument_list . (_) @computed))
  (#match? @f "^(import_module|__import__)$"))
((module (if_statement condition: [(identifier) (attribute)] @c consequence: (block) @type_block))
  (#match? @c "^([A-Za-z0-9_]+[.])?TYPE_CHECKING$"))
(module [(comment) (import_from_statement) (future_import_statement)] @pass)
(module (expression_statement (string)) @pass)
((module (expression_statement (assignment left: (identifier) @n)) @pass) (#eq? @n "__all__"))
((subscript value: (attribute object: (identifier) @o attribute: (identifier) @a) subscript: (_) @dynamic)
  (#eq? @o "os") (#eq? @a "environ") (#not-match? @dynamic "^[\"']"))
((call function: (attribute object: (identifier) @o attribute: (identifier) @a) arguments: (argument_list . (_) @dynamic))
  (#eq? @o "os") (#eq? @a "getenv") (#not-match? @dynamic "^[\"']"))
((call
   function: (attribute object: (attribute object: (identifier) @o attribute: (identifier) @e) attribute: (identifier) @a)
   arguments: (argument_list . (_) @dynamic))
  (#eq? @o "os") (#eq? @e "environ") (#eq? @a "get") (#not-match? @dynamic "^[\"']"))
((keyword_argument name: (identifier) @k value: (string (string_content) @prefix))
  (#match? @k "env_prefix$") (#match? @prefix "^[A-Za-z0-9_]+$"))
((assignment left: (identifier) @k right: (string (string_content) @prefix))
  (#match? @k "env_prefix$") (#match? @prefix "^[A-Za-z0-9_]+$"))
`

func pythonImports() Imports {
	return Imports{
		Relative:   LeadingDots,
		Prefix:     Prefix{Roots: []string{".", "src"}, Format: OwnPackages, Required: true},
		Target:     FileTarget,
		Candidates: Candidates{Add: []string{".py"}, InFolder: []string{"__init__.py"}},
		Implicit:   []string{"__init__.py", "__main__.py"},
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
