package conventions

import "regexp"

// The TypeScript and JavaScript entry: what the shared rules cannot know for these files.

// typescriptLanguage is the entry of TypeScript, and of JavaScript, whose files it also speaks for.
func typescriptLanguage() Language {
	return Language{
		Name: "typescript", Extensions: []string{".ts", ".tsx", ".js", ".mjs", ".cjs"},
		DynamicRead: typescriptDynamicRead, Imports: typescriptImports(),
	}
}

// A settings read whose name is computed at run time: not a quoted name after the opening.
var typescriptDynamicRead = regexp.MustCompile(`process\.env\[[ \t]*[^"' \t\]]`)

// The parts of the TypeScript forms. Go's regexp has no backreference and no lookahead, so a quote is a character
// class, and the clause between import and from holds only what an import clause can: names, braces, commas, `*`
// and comments. A match therefore never runs into the next statement.
const (
	tsComment = `//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/`
	tsClause  = `(?:[\w\s{},*$]|` + tsComment + `)*?`
	tsNames   = `\{(?:[\w\s,$]|` + tsComment + `)*\}`
	tsTarget  = `["'](?P<spec>[^"'\n]+)["']`
	// tsReexport is what follows `export` (and `type`) in a statement that passes imports on, up to the target.
	tsReexport = `[ \t\r\n]*(?:\*(?:[ \t\r\n]*as[ \t\r\n]+[\w$]+)?|` + tsNames + `)[ \t\r\n]*from[ \t\r\n]*`
)

var (
	tsImportFrom = regexp.MustCompile(`(?m)^[ \t]*import\b(?P<type>[ \t]+type\b)?` + tsClause + `\bfrom[ \t\r\n]*` + tsTarget)
	tsExportFrom = regexp.MustCompile(`(?m)^[ \t]*export\b(?P<type>[ \t]+type\b)?` + tsReexport + tsTarget)
	tsSideEffect = regexp.MustCompile(`(?m)^[ \t]*import[ \t]*` + tsTarget)
	tsCall       = regexp.MustCompile(`(?m)(?:^|[^.\w$])(?:import|require)\([ \t\r\n]*` + tsTarget + `[ \t\r\n]*[,)]`)
	tsComputed   = regexp.MustCompile(`(?m)(?:^|[^.\w$])(?:import|require)\([ \t\r\n]*(?:[^"'\s)]|["'][^"'\n]*["'][ \t\r\n]*[^\s,)])`)
	tsPassesOn   = regexp.MustCompile(`\A(?:[ \t\r\n]*(?:export\b(?:[ \t]+type\b)?` + tsReexport + `["'][^"'\n]+["'][ \t]*;?|` + tsComment + `))*[ \t\r\n]*\z`)
)

// tsCallWords are the words a call import needs; a file without them is not searched for one.
func tsCallWords() []string { return []string{"import(", "require("} }

func typescriptImports() Imports {
	extensions := []string{".ts", ".tsx", ".d.ts", ".js", ".jsx"}
	return Imports{
		Statements: []ImportForm{
			{Pattern: tsImportFrom}, {Pattern: tsExportFrom}, {Pattern: tsSideEffect}, {Pattern: tsCall, Words: tsCallWords()},
		},
		Computed: ImportForm{Pattern: tsComputed, Words: tsCallWords()},
		PassesOn: tsPassesOn,
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
