package conventions

// dartLanguage is the entry for Dart and Flutter.
func dartLanguage() Language {
	return Language{Name: "dart", Extensions: []string{".dart"}, Imports: dartImports(), Grammar: "dart", Query: dartQuery, Declarations: dartDeclarations}
}

// dartQuery reads import, export, part and part of with a quoted target (a conditional import's first target is the
// one taken, and part of with a library name has no target), and what a file that only passes imports on may hold.
const dartQuery = `
(configurable_uri . (uri (string_literal) @spec)) @import
(import_specification . (uri (string_literal) @spec)) @import
(part_directive (uri (string_literal) @spec)) @import
(part_of_directive (uri (string_literal) @spec)) @import
(program (import_or_export (library_export)) @pass)
(program [(library_name) (comment) (documentation_comment)] @pass)
`

func dartImports() Imports {
	return Imports{
		Relative: NoScheme,
		Prefix:   Prefix{File: "pubspec.yaml", Format: PubspecName, Members: true},
		Target:   FileTarget,
		Implicit: []string{"main.dart"},
	}
}

// dartDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const dartDeclarations = `
(program [(class_definition name: (identifier) @declared.type) (mixin_declaration (identifier) @declared.type)
  (enum_declaration name: (identifier) @declared.type) (type_alias (type_identifier) @declared.type)])
(program (function_signature name: (identifier) @declared.function))
(program (annotation) . (function_signature name: (identifier) @implicit))
`
