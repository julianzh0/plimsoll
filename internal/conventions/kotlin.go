package conventions

// kotlinLanguage is the entry for Kotlin; it resolves imports and packages as Java does, and its folders share Java's.
func kotlinLanguage() Language {
	return Language{
		Name: "kotlin", Functions: kotlinFunctions(), Tokens: cLikeTokens(character, KotlinStrings), Extensions: []string{".kt", ".kts"}, Grammar: "kotlin", Query: kotlinQuery, Declarations: kotlinDeclarations,
		Units: kotlinUnits, LayoutCode: LayoutCode{LineBreaks: true}, Peers: PackagePeers,
		Imports: jvmImports([]string{".java"}, nil),
	}
}

// kotlinQuery reads an import (with its wildcard; an alias is not part of the target), and the top-level names a
// file declares, which an import narrows by.
const kotlinQuery = `
(import_header (identifier) @spec (wildcard_import)? @spec) @import
(source_file [(class_declaration (type_identifier) @bind) (object_declaration (type_identifier) @bind)
  (function_declaration (simple_identifier) @bind) (property_declaration (variable_declaration (simple_identifier) @bind))
  (type_alias (type_identifier) @bind)] @bind.at)
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

// kotlinFunctions names what a function is and what counts in it, in the grammar's node kinds.
func kotlinFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:    []string{"function_declaration", "secondary_constructor", "getter", "setter"},
		Bodies:      []string{"function_body", "statements"},
		Closures:    []string{"lambda_literal", "anonymous_function"},
		Ifs:         []string{"if_expression"},
		Nesting:     []string{"for_statement", "while_statement", "do_while_statement", "when_expression", "catch_block"},
		Logic:       []string{"&&", "||"},
		Parentheses: []string{"parenthesized_expression"},
		DepthIfs:    []string{"if_expression"},
		Depth:       []string{"for_statement", "while_statement", "do_while_statement", "when_expression", "try_expression"},
		Blocks:      []string{"statements"},
	}
}
