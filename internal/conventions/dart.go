package conventions

// dartLanguage is the entry for Dart and Flutter.
func dartLanguage() Language {
	return Language{Name: "dart", Extensions: []string{".dart"}, Imports: dartImports(), Grammar: "dart", Query: dartQuery}
}

// dartQuery reads import, export, part and part of with a quoted target (a conditional import's first target is the
// one taken, and part of with a library name has no target), what a file that only passes imports on may hold, and a
// settings read whose name is not a quoted literal: Platform.environment[…], String/bool/int.fromEnvironment(…) and
// flutter_dotenv's dotenv.env[…].
const dartQuery = `
(configurable_uri . (uri (string_literal) @spec)) @import
(import_specification . (uri (string_literal) @spec)) @import
(part_directive (uri (string_literal) @spec)) @import
(part_of_directive (uri (string_literal) @spec)) @import
(program (import_or_export (library_export)) @pass)
(program [(library_name) (comment) (documentation_comment)] @pass)
((identifier) @o . (selector (unconditional_assignable_selector (identifier) @p)) .
  (selector (unconditional_assignable_selector (index_selector . (_) @dynamic)))
  (#eq? @o "Platform") (#eq? @p "environment") (#not-match? @dynamic "^[\"']"))
((identifier) @o . (selector (unconditional_assignable_selector (identifier) @p)) .
  (selector (unconditional_assignable_selector (index_selector . (_) @dynamic)))
  (#eq? @o "dotenv") (#eq? @p "env") (#not-match? @dynamic "^[\"']"))
((identifier) @o . (selector (unconditional_assignable_selector (identifier) @p)) .
  (selector (argument_part (arguments . (argument) @dynamic)))
  (#match? @o "^(String|bool|int)$") (#eq? @p "fromEnvironment") (#not-match? @dynamic "^[\"']"))
((const_object_expression (type_identifier) @o (identifier) @p (arguments . (argument) @dynamic))
  (#match? @o "^(String|bool|int)$") (#eq? @p "fromEnvironment") (#not-match? @dynamic "^[\"']"))
`

func dartImports() Imports {
	return Imports{
		Relative: NoScheme,
		Prefix:   Prefix{File: "pubspec.yaml", Format: PubspecName, Members: true},
		Target:   FileTarget,
		Implicit: []string{"main.dart"},
	}
}
