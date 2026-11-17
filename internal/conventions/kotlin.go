package conventions

// kotlinLanguage is the entry for Kotlin; it resolves imports and packages as Java does, and its folders share Java's.
func kotlinLanguage() Language {
	return Language{
		Name: "kotlin", Extensions: []string{".kt", ".kts"}, Grammar: "kotlin", Query: kotlinQuery, Declarations: kotlinDeclarations,
		Imports: jvmImports([]string{".java"}, nil),
	}
}

// kotlinQuery reads an import (with its wildcard; an alias is not part of the target).
const kotlinQuery = `
(import_header (identifier) @spec (wildcard_import)? @spec) @import
`

// kotlinDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const kotlinDeclarations = `
(source_file [(class_declaration (type_identifier) @declared.type) (object_declaration (type_identifier) @declared.type)])
(source_file (function_declaration (simple_identifier) @declared.function))
(source_file (property_declaration (binding_pattern_kind "val") (variable_declaration (simple_identifier) @declared.value)))
(source_file [(function_declaration (modifiers (annotation)) (simple_identifier) @implicit)
  (class_declaration (modifiers (annotation)) (type_identifier) @implicit)])
`
