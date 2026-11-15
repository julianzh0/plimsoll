package conventions

import "regexp"

// rustLanguage is the entry for Rust. Its mod lines and use paths are followed to files through the module tree that
// the mod lines declare.
func rustLanguage() Language {
	return Language{
		Name: "rust", Extensions: []string{".rs"}, DynamicRead: rustDynamicRead, Literals: rustLiterals, TestBlock: rustTestBlock,
		Imports: Imports{
			Statements: []ImportForm{{Pattern: rustMod}, {Pattern: rustUse}, {Pattern: rustExtern}},
			Grouped:    true,
			Module:     rustModule,
			PassesOn:   rustPassesOn,
			Relative:   NoRelative,
			Prefix:     Prefix{File: "Cargo.toml", Format: CargoCrates, Members: true},
			Target:     FileTarget,
			NoRings:    "a crate's modules use each other, and Rust compiles them as one unit",
			Implicit:   []string{"main.rs", "lib.rs", "build.rs"},
			NotSeen:    "a path written in code without a `use` line, and a module named by a `path` attribute",
		},
	}
}

// rustAttributes are the attributes before an item on its line, rustVisibility an item's visibility.
const (
	rustAttributes = `(?:#\[[^\]\n]*\][ \t]*)*`
	rustVisibility = `(?:pub(?:\([^)\n]*\))?[ \t]+)?`
)

var (
	// rustDynamicRead is a settings read whose name is not a quoted literal.
	rustDynamicRead = regexp.MustCompile(`\benv::var(?:_os)?\([ \t\r\n]*[^" \t\r\n)]`)
	// rustLiterals are a line comment, a block comment that may hold block comments one level deep, a raw string, a
	// string over lines and a character literal; a lifetime ('a) is none.
	// NOTE: block comments nested deeper than one level, and raw strings with more than three "#", end early.
	// Upgrade: a lexer.
	rustLiterals = regexp.MustCompile(`//[^\n]*|/\*(?:/+\*(?:[^*]|\*+[^*/])*\*+/|[^*/]|\*+[^*/]|/+[^*/])*\*+/|r"[^"]*"|(?s:r#".*?"#|r##".*?"##|r###".*?"###)|"(?:[^"\\]|\\[\s\S])*"|'(?:\\(?:[^xu\n]|x[0-9a-fA-F]{2}|u\{[0-9a-fA-F_]+\})|[^\\'\n])'`)
	// rustTestBlock is #[cfg(test)] and what follows it up to the first ";" or "{" without a "}" between: a "}"
	// first means a field or a variant under the attribute, which is no block.
	rustTestBlock = regexp.MustCompile(`#\[cfg\(test\)\][^;{}]*[;{]`)
	// rustMod is a module declared in its own file; the target keeps the word mod, so a declaration is told from a path.
	rustMod = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `(?P<spec>mod[ \t]+[A-Za-z_]\w*)[ \t]*;`)
	// rustUse is a use declaration; its target may group paths in braces.
	rustUse = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `use[ \t\r\n]+(?P<spec>[^;]+);`)
	// rustExtern is an extern crate declaration.
	rustExtern = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `extern[ \t]+crate[ \t]+(?P<spec>[A-Za-z_]\w*)`)
	// rustModule is the first line of a module a file holds in braces.
	rustModule = regexp.MustCompile(`(?m)^[ \t]*` + rustAttributes + rustVisibility + `mod[ \t]+(?P<name>[A-Za-z_]\w*)[ \t\r\n]*\{`)
	// rustPassesOn is a file of attributes, mod lines and public use and extern crate lines only; it is tried on text
	// whose comments are blanked.
	rustPassesOn = regexp.MustCompile(`\A(?:[ \t\r\n]*(?:#!?\[[^\]\n]*\]|` + rustVisibility + `mod[ \t]+\w+[ \t]*;|pub(?:\([^)\n]*\))?[ \t]+(?:use[ \t\r\n]+[^;]+|extern[ \t]+crate[ \t]+[^;]+);))*[ \t\r\n]*\z`)
)
