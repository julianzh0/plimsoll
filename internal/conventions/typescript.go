package conventions

// The TypeScript and JavaScript entry: what the shared rules cannot know for these files.

// typescriptLanguage is the entry of TypeScript, and of JavaScript, whose files it also speaks for. .ts files are read
// by the TypeScript grammar; .tsx and the JavaScript suffixes by the TSX grammar, which also reads JSX.
func typescriptLanguage() Language {
	return Language{
		Name: "typescript", Extensions: []string{".ts", ".tsx", ".js", ".mjs", ".cjs"},
		Imports: typescriptImports(), Grammar: "tsx", GrammarBySuffix: map[string]string{".ts": "typescript"},
		Query: typescriptQuery,
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
