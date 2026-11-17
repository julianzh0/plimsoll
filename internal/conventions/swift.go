package conventions

// swiftLanguage is the entry for Swift. An import names a module: a target of Package.swift, whose folder holds every
// file of it, at any depth.
func swiftLanguage() Language {
	return Language{
		Name: "swift", Extensions: []string{".swift"}, Grammar: "swift", Query: swiftQuery,
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
