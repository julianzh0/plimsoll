package conventions

import "regexp"

// swiftLanguage is the entry for Swift. An import names a module: a target of Package.swift, whose folder holds every
// file of it, at any depth.
func swiftLanguage() Language {
	return Language{
		Name: "swift", Extensions: []string{".swift"}, DynamicRead: swiftDynamicRead, Literals: swiftLiterals,
		Imports: Imports{
			Statements: []ImportForm{{Pattern: swiftImport}},
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

var (
	// swiftDynamicRead is a settings read whose name is not a quoted literal.
	swiftDynamicRead = regexp.MustCompile(`ProcessInfo\.processInfo\.environment\[[ \t]*[^" \t\]]`)
	// swiftLiterals are a line comment, a block comment, a multi-line string, a raw string and a string.
	// NOTE: a block comment that holds another ends at the inner end. Upgrade: a lexer.
	swiftLiterals = regexp.MustCompile(`//[^\n]*|/\*(?:[^*]|\*+[^*/])*\*+/|#*"""(?s:.*?)"""#*|#+"[^\n]*?"#+|"(?:[^"\\\n]|\\.)*"`)
	// swiftImport is an import of a module, with its attributes, an access level and a kind of the one symbol it
	// imports: the target is the module, the first name.
	// NOTE: an import under #if is read whatever its condition, so a platform's or a trait's imports count on every
	// side. Upgrade: evaluate the conditions of the platforms and traits the package declares.
	swiftImport = regexp.MustCompile(`(?m)^[ \t]*(?:@\w+(?:\([^)\n]*\))?[ \t]+)*(?:(?:public|package|internal|fileprivate|private)[ \t]+)?` +
		`import[ \t]+(?:(?:typealias|struct|class|enum|protocol|let|var|func)[ \t]+)?(?P<spec>[A-Za-z_]\w*)`)
)
