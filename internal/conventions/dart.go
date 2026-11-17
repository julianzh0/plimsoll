package conventions

// dartLanguage is the entry for Dart and Flutter.
func dartLanguage() Language {
	return Language{Name: "dart", Functions: dartFunctions(), Tokens: cLikeTokens(`r?'''[\s\S]*?'''|r?` + tripleQuoted + "|" + doubleQuoted + "|" + singleQuoted), Extensions: []string{".dart"}, Imports: dartImports(), Grammar: "dart", Query: dartQuery, Declarations: dartDeclarations}
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

// dartFunctions names what a function is and what counts in it, in the grammar's node kinds.
func dartFunctions() *FunctionKinds {
	return &FunctionKinds{
		Declared:     []string{"function_signature", "method_signature", "getter_signature", "setter_signature", "constructor_signature", "factory_constructor_signature"},
		Bodies:       []string{"function_body"},
		BodyAfter:    true,
		Closures:     []string{"function_expression"},
		Ifs:          []string{"if_statement", "if_element"},
		Nesting:      []string{"for_statement", "for_element", "while_statement", "do_statement", "switch_statement", "switch_expression", "catch_clause", "conditional_expression"},
		Logic:        []string{"&&", "||"},
		Parentheses:  []string{"parenthesized_expression"},
		DepthIfs:     []string{"if_statement"},
		Depth:        []string{"for_statement", "while_statement", "do_statement", "switch_statement", "switch_expression", "try_statement"},
		Blocks:       []string{"block", "switch_statement_case", "switch_statement_default"},
		SwitchBlocks: []string{"switch_statement_case", "switch_statement_default"},
	}
}
