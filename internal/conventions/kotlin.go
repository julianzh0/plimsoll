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
(source_file [(class_declaration (type_identifier) @declared.type) (object_declaration (type_identifier) @declared.type)])
(source_file (function_declaration (simple_identifier) @declared.function))
(source_file (property_declaration (binding_pattern_kind "val") (variable_declaration (simple_identifier) @declared.value)))
(source_file [(function_declaration (modifiers (annotation)) (simple_identifier) @implicit)
  (class_declaration (modifiers (annotation)) (type_identifier) @implicit)])
`
