package conventions

import "regexp"

// rustLanguage is the entry for Rust. Its mod lines and use paths are followed to files through the module tree that
// the mod lines declare.
func rustLanguage() Language {
	return Language{
		Name: "rust", Extensions: []string{".rs"}, Grammar: "rust", Query: rustQuery, Declarations: rustDeclarations,
		Imports: Imports{
			Statements: []ImportForm{{Pattern: rustMod}, {Pattern: rustUse}, {Pattern: rustExtern}},
			Grouped:    true,
			Relative:   NoRelative,
			Prefix:     Prefix{File: "Cargo.toml", Format: CargoCrates, Members: true},
			Target:     FileTarget,
			NoRings:    "a crate's modules use each other, and Rust compiles them as one unit",
			Implicit:   []string{"main.rs", "lib.rs", "build.rs"},
			NotSeen:    "a path written in code without a `use` line, and a module named by a `path` attribute",
		},
	}
}

// rustQuery reads mod lines, use paths and extern crates (a use may group paths in braces, which own code expands),
// the modules a file holds in braces, test code under #[cfg(test)] (the item after the attributes and comments),
// macro bodies, which the grammar keeps as tokens and the text forms below read, comments and strings, blanked inside
// targets and macro bodies, and what a file that only passes imports on may hold.
const rustQuery = `
(mod_item !body name: (identifier) @spec.mod) @import
(use_declaration argument: (_) @spec) @import
(extern_crate_declaration name: (identifier) @spec) @import
(mod_item name: (identifier) @module.name body: (declaration_list)) @module
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
	// rustExtern is an extern crate declaration.
	rustExtern = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `extern[ \t]+crate[ \t]+(?P<spec>[A-Za-z_]\w*)`)
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
`
