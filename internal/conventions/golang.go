package conventions

import "regexp"

// The Go entry: what the shared rules cannot know for Go files.

// goLanguage is the entry of Go.
func goLanguage() Language {
	return Language{Name: "go", Extensions: []string{".go"}, Imports: goImports(), Grammar: "go", Query: goQuery}
}

// goQuery reads the import specs.
const goQuery = `
(import_spec path: (_) @spec) @import
(source_file (function_declaration name: (identifier) @declared.function))
(source_file (type_declaration [(type_spec name: (type_identifier) @declared.type) (type_alias name: (type_identifier) @declared.type)]))
((source_file (var_declaration [(var_spec name: (identifier) @declared.value) (var_spec_list (var_spec name: (identifier) @declared.value))]))
  (#match? @declared.value "^[A-Z]"))
((source_file (const_declaration (const_spec name: (identifier) @declared.value))) (#match? @declared.value "^[A-Z]"))
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
