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
