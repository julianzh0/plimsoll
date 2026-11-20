package conventions

import "regexp"

// The Go entry: what the shared rules cannot know for Go files.

// goLanguage is the entry of Go.
func goLanguage() Language {
	return Language{Name: "go", Functions: goFunctions(), Tokens: cLikeTokens(doubleQuoted+"|"+character+"|"+backQuoted, PlainStrings), Extensions: []string{".go"}, Imports: goImports(), Grammar: "go", Query: goQuery, Declarations: goDeclarations,
		Units: goUnits, LayoutCode: LayoutCode{LineBreaks: true}, Peers: FolderPeers}
}

// goQuery reads the import specs.
const goQuery = `
(import_spec path: (_) @spec) @import
`

var goFolderExcept = regexp.MustCompile(`_test\.go$`)

func goImports() Imports {
	return Imports{
		Relative:     NoRelative,
		Prefix:       Prefix{File: "go.mod", Format: GoModule, Required: true},
		Target:       FolderTarget,
		FolderExcept: goFolderExcept,
		NoRings:      "the Go compiler rejects import cycles",
	}
}

// goDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const goDeclarations = `
(source_file (function_declaration name: (identifier) @declared.function))
(source_file (type_declaration [(type_spec name: (type_identifier) @declared.type) (type_alias name: (type_identifier) @declared.type)]))
((source_file (var_declaration [(var_spec name: (identifier) @declared.value) (var_spec_list (var_spec name: (identifier) @declared.value))]))
  (#match? @declared.value "^[A-Z]"))
((source_file (const_declaration (const_spec name: (identifier) @declared.value))) (#match? @declared.value "^[A-Z]"))
`

// goFunctions names what a function is and what counts in it, in the grammar's node kinds.
func goFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:    []string{"function_declaration", "method_declaration"},
		Bodies:      []string{"block"},
		Closures:    []string{"func_literal"},
		Ifs:         []string{"if_statement"},
		Nesting:     []string{"for_statement", "expression_switch_statement", "type_switch_statement", "select_statement"},
		Logic:       []string{"&&", "||"},
		Parentheses: []string{"parenthesized_expression"},
		DepthIfs:    []string{"if_statement"},
		Depth:       []string{"for_statement", "expression_switch_statement", "type_switch_statement", "select_statement"},
		Blocks:      []string{"statement_list"},
	}
}
