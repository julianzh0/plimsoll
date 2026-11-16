package conventions

// kotlinLanguage is the entry for Kotlin; it resolves imports and packages as Java does, and its folders share Java's.
func kotlinLanguage() Language {
	return Language{
		Name: "kotlin", Extensions: []string{".kt", ".kts"}, Grammar: "kotlin", Query: kotlinQuery,
		Imports: jvmImports([]string{".java"}, nil),
	}
}

// kotlinQuery reads an import (with its wildcard; an alias is not part of the target) and a settings read whose name
// is not a quoted literal.
const kotlinQuery = `
(import_header (identifier) @spec (wildcard_import)? @spec) @import
((call_expression
   (navigation_expression
     [(simple_identifier) @o (navigation_expression (navigation_suffix (simple_identifier) @o) .)]
     (navigation_suffix (simple_identifier) @m))
   (call_suffix (value_arguments . (value_argument) @dynamic)))
  (#eq? @o "System") (#eq? @m "getenv") (#not-match? @dynamic "^\""))
`
