package conventions

// kotlinLanguage is the entry for Kotlin; it resolves imports and packages as Java does, and its folders share Java's.
func kotlinLanguage() Language {
	return Language{
		Name: "kotlin", Extensions: []string{".kt", ".kts"}, Grammar: "kotlin", Query: kotlinQuery,
		Imports: jvmImports([]string{".java"}, nil),
	}
}

// kotlinQuery reads an import (with its wildcard; an alias is not part of the target).
const kotlinQuery = `
(import_header (identifier) @spec (wildcard_import)? @spec) @import
`
