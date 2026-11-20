package conventions

import "regexp"

// rustLanguage is the entry for Rust. Its mod lines and use paths are followed to files through the module tree that
// the mod lines declare.
func rustLanguage() Language {
	return Language{
		Name: "rust", Functions: rustFunctions(), Tokens: cLikeTokens(rustLiterals, RustRawStrings), Extensions: []string{".rs"}, Grammar: "rust", Query: rustQuery, Declarations: rustDeclarations,
		Units: rustUnits,
		Imports: Imports{
			Statements: []ImportForm{{Pattern: rustMod}, {Pattern: rustUse}, {Pattern: rustExtern}},
			Grouped:    true,
			Relative:   NoRelative,
			Prefix:     Prefix{File: "Cargo.toml", Format: CargoCrates, Members: true},
			Target:     FileTarget,
			NoRings:    "a crate's modules use each other, and Rust compiles them as one unit",
			Implicit:   []string{"main.rs", "lib.rs", "build.rs"},
			NotSeen:    "a path written in code without a `use` line, and a module or name a macro creates",
			NoImporter: "a parent file that only declares a module (`mod x;`)",
		},
	}
}

// rustItems are the items that bind a name in the module they stand in, and the item each name belongs to.
const rustItems = `[(function_item name: (identifier) @bind) (function_signature_item name: (identifier) @bind)
  (struct_item name: (type_identifier) @bind) (enum_item name: (type_identifier) @bind)
  (trait_item name: (type_identifier) @bind) (type_item name: (type_identifier) @bind)
  (union_item name: (type_identifier) @bind) (const_item name: (identifier) @bind) (static_item name: (identifier) @bind)
  (mod_item name: (identifier) @bind) (macro_definition name: (identifier) @bind)
  (extern_crate_declaration alias: (identifier) @bind)] @bind.at`

// rustOpaque is what may bring names into a module that no item shows: a macro call among its items, and an attribute
// other than the inert built-in ones.
const rustOpaque = `[(macro_invocation) (expression_statement (macro_invocation))
  (attribute_item (attribute [(identifier) (scoped_identifier)] @_a (#not-match? @_a "^(cfg|path|doc|allow|warn|deny|forbid|expect|test|inline|must_use|repr)$")))
  (inner_attribute_item (attribute [(identifier) (scoped_identifier)] @_i (#not-match? @_i "^(cfg|path|doc|allow|warn|deny|forbid|expect|test|inline|must_use|repr)$")))] @opaque`

// rustQuery reads mod lines, use paths and extern crates (a use may group paths in braces, which own code expands),
// the file a path attribute names for a module, the modules a file holds in braces, the names each module binds and
// what may bind others, the macros a crate exports, test code under #[cfg(test)] (the item after the attributes and
// comments), macro bodies, which the grammar keeps as tokens and the text forms below read, comments and strings,
// blanked inside targets and macro bodies, and what a file that only passes imports on may hold.
const rustQuery = `
(mod_item !body name: (identifier) @spec.mod) @import
(use_declaration argument: (_) @spec) @import
(extern_crate_declaration name: (identifier) @spec.extern) @import
((attribute_item (attribute (identifier) @_p value: (string_literal) @path)) . [(attribute_item) (line_comment) (block_comment)]* .
  (mod_item name: (identifier) @path.of) (#eq? @_p "path"))
(mod_item name: (identifier) @module.name body: (declaration_list)) @module
(source_file ` + rustItems + `)
(mod_item body: (declaration_list ` + rustItems + `))
(source_file ` + rustOpaque + `)
(mod_item body: (declaration_list ` + rustOpaque + `))
((attribute_item (attribute (identifier) @_e)) . [(attribute_item) (line_comment) (block_comment)]* .
  (macro_definition name: (identifier) @export) (#eq? @_e "macro_export"))
((attribute_item (attribute (identifier) @a arguments: (token_tree) @t)) @test . [(attribute_item) (line_comment) (block_comment)]* .
  [(function_item) (mod_item) (impl_item) (struct_item) (enum_item) (union_item) (trait_item) (use_declaration)
   (const_item) (static_item) (type_item) (macro_definition) (macro_invocation) (extern_crate_declaration)
   (field_declaration) (expression_statement) (let_declaration) (enum_variant) (foreign_mod_item)
   (associated_type) (function_signature_item)] @test.end
  (#eq? @a "cfg") (#eq? @t "(test)"))
(macro_invocation (token_tree) @text)
(macro_rule right: (token_tree) @text)
[(line_comment) (block_comment) (string_literal) (raw_string_literal)] @blank
(source_file [(inner_attribute_item) (attribute_item) (line_comment) (block_comment)] @pass)
(source_file (mod_item !body) @pass)
(source_file (use_declaration (visibility_modifier)) @pass)
(source_file (extern_crate_declaration (visibility_modifier)) @pass)
`

// rustAttributes are the attributes before an item on its line, rustVisibility an item's visibility.
const (
	rustAttributes = `(?:#\[[^\]\n]*\][ \t]*)*`
	rustVisibility = `(?:pub(?:\([^)\n]*\))?[ \t]+)?`
)

var (
	// rustMod is a module declared in its own file; the target keeps the word mod, so a declaration is told from a path.
	rustMod = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `(?P<spec>mod[ \t]+[A-Za-z_]\w*)[ \t]*;`)
	// rustUse is a use declaration; its target may group paths in braces.
	rustUse = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `use[ \t\r\n]+(?P<spec>[^;]+);`)
	// rustExtern is an extern crate declaration; the target keeps the word crate, as the grammar's reading does.
	rustExtern = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `extern[ \t]+(?P<spec>crate[ \t]+[A-Za-z_]\w*)`)
)

// rustDeclarations reads the top-level names a file declares, for the list of code a change added that nothing uses: run
// only on the files a change names.
const rustDeclarations = `
(source_file (function_item name: (identifier) @declared.function))
(source_file [(struct_item name: (type_identifier) @declared.type) (enum_item name: (type_identifier) @declared.type)
  (trait_item name: (type_identifier) @declared.type) (type_item name: (type_identifier) @declared.type)
  (union_item name: (type_identifier) @declared.type)])
(source_file [(const_item (visibility_modifier) name: (identifier) @declared.value)
  (static_item (visibility_modifier) name: (identifier) @declared.value)])
((source_file (attribute_item (attribute (identifier) @_frb)) . [(struct_item name: (type_identifier) @implicit)
  (enum_item name: (type_identifier) @implicit)]) (#eq? @_frb "frb"))
((source_file (attribute_item (attribute [(identifier) @_t (scoped_identifier name: (identifier) @_t)])) .
  (function_item name: (identifier) @implicit)) (#match? @_t "^(test|bench)$"))
((source_file (attribute_item (attribute [(identifier) @_t (scoped_identifier name: (identifier) @_t)])) . (attribute_item) .
  (function_item name: (identifier) @implicit)) (#match? @_t "^(test|bench)$"))
`

// rustFunctions names what a function is and what counts in it, in the grammar's node kinds.
func rustFunctions() FunctionKinds {
	return FunctionKinds{
		Declared:    []string{"function_item"},
		Bodies:      []string{"block"},
		Closures:    []string{"closure_expression"},
		Ifs:         []string{"if_expression"},
		ElseClauses: []string{"else_clause"},
		Nesting:     []string{"for_expression", "while_expression", "loop_expression", "match_expression"},
		Logic:       []string{"&&", "||"},
		Parentheses: []string{"parenthesized_expression"},
		DepthIfs:    []string{"if_expression"},
		Depth:       []string{"for_expression", "while_expression", "loop_expression", "match_expression"},
		Blocks:      []string{"block"},
	}
}
