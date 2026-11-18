package conventions

// The Python entry: what the shared rules cannot know for Python files.

// pythonLanguage is the entry of Python.
func pythonLanguage() Language {
	return Language{
		Name: "python", Functions: pythonFunctions(), Tokens: pythonTokens, Extensions: []string{".py", ".pyi"},
		Imports: pythonImports(), Layout: pytestLayout(), Grammar: "python", Query: pythonQuery, Declarations: pythonDeclarations,
		// MkDocs hooks: scripts named under hooks: in mkdocs.yml hold event handlers named on_… (MkDocs 1.4.0).
		Loaded: []Convention{{Beside: []string{"mkdocs.yml", "mkdocs.yaml"}, Names: "on_*"}},
	}
}

// pythonQuery reads `import` and `from` statements with the names they import, `import_module()` and `__import__()`
// with a plain string first (any other first argument is an import built at run time), the imports under a top-level
// `if TYPE_CHECKING:`, and what a file that only passes imports on may hold.
const pythonQuery = `
(import_from_statement module_name: (_) @spec) @import
(import_from_statement name: [(dotted_name) @name (aliased_import name: (dotted_name) @name)])
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

// pythonDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const pythonDeclarations = `
(module (function_definition name: (identifier) @declared.function))
(module (class_definition name: (identifier) @declared.type))
`

// pythonFunctions names what a function is and what counts in it, in the grammar's node kinds.
func pythonFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:     []string{"function_definition"},
		Bodies:       []string{"block"},
		Closures:     []string{"lambda"},
		Ifs:          []string{"if_statement"},
		ElseClauses:  []string{"else_clause"},
		FlatBranches: []string{"elif_clause"},
		Nesting:      []string{"for_statement", "while_statement", "except_clause", "conditional_expression", "match_statement"},
		Logic:        []string{"and", "or"},
		Parentheses:  []string{"parenthesized_expression"},
		DepthIfs:     []string{"if_statement"},
		Depth:        []string{"for_statement", "while_statement", "try_statement", "match_statement"},
		Blocks:       []string{"block"},
	}
}
