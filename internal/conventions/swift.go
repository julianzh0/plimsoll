package conventions

import "regexp"

// swiftLanguage is the entry for Swift. An import names a module: a target of Package.swift, whose folder holds every
// file of it, at any depth.
func swiftLanguage() Language {
	return Language{
		Name: "swift", Functions: swiftFunctions(), Tokens: cLikeTokens(`#*` + tripleQuoted + `#*|` + doubleQuoted), Extensions: []string{".swift"}, Grammar: "swift", Query: swiftQuery, Declarations: swiftDeclarations,
		Library: &Library{Manifest: "Package.swift", Is: regexp.MustCompile(`\.library\(`)},
		Imports: Imports{
			Relative:   NoRelative,
			Prefix:     Prefix{File: "Package.swift", Format: SwiftTargets, Required: true},
			Target:     FolderTarget,
			FolderTree: true,
			NoRings:    "an import names a module, and the compiler rejects modules that import each other",
			Implicit:   []string{"main.swift", "Package.swift"},
			NotSeen:    "use between the files of one module, which needs no `import`",
		},
	}
}

// swiftQuery reads an import of a module (its attributes, access level and the kind of one imported symbol are no
// part of the target, which is the module, the first name).
// NOTE: an import under #if is read whatever its condition, so a platform's or a trait's imports count on every side.
// Upgrade: evaluate the conditions of the platforms and traits the package declares.
const swiftQuery = `
(import_declaration (identifier . (simple_identifier) @spec)) @import
`

// swiftDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const swiftDeclarations = `
(source_file (function_declaration name: (simple_identifier) @declared.function))
(source_file (class_declaration declaration_kind: ["class" "struct" "enum" "actor"] name: (type_identifier) @declared.type))
(source_file [(protocol_declaration name: (type_identifier) @declared.type) (typealias_declaration name: (type_identifier) @declared.type)])
(source_file (class_declaration (modifiers (attribute)) name: (type_identifier) @implicit))
((source_file [(function_declaration (modifiers (visibility_modifier) @_p) name: (simple_identifier) @exported)
  (class_declaration (modifiers (visibility_modifier) @_p) name: (type_identifier) @exported)
  (protocol_declaration (modifiers (visibility_modifier) @_p) name: (type_identifier) @exported)
  (typealias_declaration (modifiers (visibility_modifier) @_p) name: (type_identifier) @exported)])
  (#match? @_p "^(public|open)$"))
`

// swiftFunctions names what a function is and what counts in it, in the grammar's node kinds.
func swiftFunctions() FunctionKinds {
	return FunctionKinds{
		Declared: []string{"function_declaration", "init_declaration", "deinit_declaration"},
		Bodies:   []string{"function_body"},
		Closures: []string{"lambda_literal"},
		Ifs:      []string{"if_statement"},
		Nesting:  []string{"guard_statement", "for_statement", "while_statement", "repeat_while_statement", "switch_statement", "catch_block", "ternary_expression"},
		Logic:    []string{"&&", "||"},
		DepthIfs: []string{"if_statement"},
		Depth:    []string{"guard_statement", "for_statement", "while_statement", "repeat_while_statement", "switch_statement", "do_statement"},
		Blocks:   []string{"statements"},
	}
}
