package conventions

import "regexp"

// The TypeScript and JavaScript entry: what the shared rules cannot know for these files.

// typescriptLanguage is the entry of TypeScript, and of JavaScript, whose files it also speaks for. .ts files are read
// by the TypeScript grammar; .tsx and the JavaScript suffixes by the TSX grammar, which also reads JSX.
func typescriptLanguage() Language {
	return Language{
		Name: "typescript", Extensions: []string{".ts", ".tsx", ".js", ".mjs", ".cjs"},
		Imports: typescriptImports(), Grammar: "tsx", GrammarBySuffix: map[string]string{".ts": "typescript"},
		Query: typescriptQuery, Declarations: typescriptDeclarations,
		// trigger.dev finds tasks in folders named trigger when its config sets no dirs
		// (https://trigger.dev/docs/config/config-file, read 2026-11-17).
		// NOTE: a dirs entry of trigger.config that names another folder is not read. Upgrade: read dirs from the
		// config.
		Loaded: []Convention{{
			Beside: []string{"trigger.config.ts", "trigger.config.js", "trigger.config.mjs", "trigger.config.cjs", "trigger.config.mts"},
			Folder: "trigger",
		}},
		Library: &Library{Manifest: "package.json", Is: regexp.MustCompile(`"exports"\s*:`), Not: regexp.MustCompile(`"private"\s*:\s*true`)},
	}
}

// typescriptQuery reads import and export statements with a target, `import x = require()`, calls of `import()` and
// `require()` with a string first, every such call as one built at run time unless a target lies inside it, the
// and the statements a file that only passes imports on may hold.
const typescriptQuery = `
(import_statement "type"? @type source: (string (string_fragment)) @spec) @import
(export_statement "type"? @type source: (string (string_fragment)) @spec) @import
(import_require_clause source: (string (string_fragment)) @spec) @import
((call_expression function: [(import) (identifier) @f] arguments: (arguments . (string (string_fragment)) @spec)) @import
  (#eq? @f "require"))
((call_expression function: [(import) (identifier) @f] arguments: (arguments . (_) @computed)) (#eq? @f "require"))
(program (export_statement source: (string)) @pass)
(program (comment) @pass)
`

func typescriptImports() Imports {
	extensions := []string{".ts", ".tsx", ".d.ts", ".js", ".jsx"}
	return Imports{
		Relative: DotSlash,
		Prefix:   Prefix{File: "tsconfig.json", Format: TSConfigPaths},
		Target:   FileTarget,
		Candidates: Candidates{
			Swap: []Swap{
				{From: ".js", To: []string{".ts", ".tsx", ".d.ts"}}, {From: ".jsx", To: []string{".tsx"}},
				{From: ".mjs", To: []string{".mts", ".d.mts"}}, {From: ".cjs", To: []string{".cts", ".d.cts"}},
			},
			Add:      extensions,
			InFolder: []string{"index.ts", "index.tsx", "index.d.ts", "index.js", "index.jsx"},
		},
	}
}

// typescriptDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const typescriptDeclarations = `
(program (function_declaration name: (identifier) @declared.function))
(program (generator_function_declaration name: (identifier) @declared.function))
(program (export_statement declaration: [(function_declaration name: (identifier) @declared.function)
  (generator_function_declaration name: (identifier) @declared.function)]))
(program (export_statement "default" declaration: [(function_declaration name: (identifier) @implicit)
  (generator_function_declaration name: (identifier) @implicit) (class_declaration name: (type_identifier) @implicit)]))
(program [(class_declaration name: (type_identifier) @declared.type) (abstract_class_declaration name: (type_identifier) @declared.type)
  (interface_declaration name: (type_identifier) @declared.type) (type_alias_declaration name: (type_identifier) @declared.type)
  (enum_declaration name: (identifier) @declared.type)])
(program (export_statement declaration: [(class_declaration name: (type_identifier) @declared.type)
  (abstract_class_declaration name: (type_identifier) @declared.type) (interface_declaration name: (type_identifier) @declared.type)
  (type_alias_declaration name: (type_identifier) @declared.type) (enum_declaration name: (identifier) @declared.type)]))
(class_declaration decorator: (decorator) name: (type_identifier) @implicit)
(export_statement decorator: (decorator) declaration: (class_declaration name: (type_identifier) @implicit))
(program [(lexical_declaration (variable_declarator name: (identifier) @declared.function value: [(arrow_function) (function_expression)]))
  (variable_declaration (variable_declarator name: (identifier) @declared.function value: [(arrow_function) (function_expression)]))])
(program (export_statement declaration: [(lexical_declaration (variable_declarator name: (identifier) @declared.function value: [(arrow_function) (function_expression)]))
  (variable_declaration (variable_declarator name: (identifier) @declared.function value: [(arrow_function) (function_expression)]))]))
(program (export_statement declaration: [(lexical_declaration (variable_declarator name: (identifier) @declared.value))
  (variable_declaration (variable_declarator name: (identifier) @declared.value))]))
(program (export_statement declaration: [(function_declaration name: (identifier) @exported)
  (generator_function_declaration name: (identifier) @exported) (class_declaration name: (type_identifier) @exported)
  (abstract_class_declaration name: (type_identifier) @exported) (interface_declaration name: (type_identifier) @exported)
  (type_alias_declaration name: (type_identifier) @exported) (enum_declaration name: (identifier) @exported)
  (lexical_declaration (variable_declarator name: (identifier) @exported))
  (variable_declaration (variable_declarator name: (identifier) @exported))]))
`
